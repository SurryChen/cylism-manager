package database

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// Executed inside the Operator, where MySQL Shell and cluster networking are
// available. Credentials travel over stdin, never in process arguments or logs.
const configurePrimaryScript = `import json, sys
import mysqlsh
try:
    request = json.load(sys.stdin)
    mysqlsh.globals.shell.connect({"scheme": "mysql", "host": request["host"], "port": 3306, "user": request["user"], "password": request["password"]})
    cluster = mysqlsh.globals.dba.get_cluster()
    status = cluster.status()
    topology = status["defaultReplicaSet"]["topology"]
    if len(topology) != request["instances"] or any(member.get("status") != "ONLINE" for member in topology.values()):
        print("CYLISM_PRIMARY_WAITING")
        sys.exit(2)
    session = mysqlsh.globals.shell.open_session({"scheme": "mysql", "host": request["host"], "port": 3306, "user": request["user"], "password": request["password"]})
    role = session.run_sql("SELECT MEMBER_ROLE FROM performance_schema.replication_group_members WHERE MEMBER_ID = @@server_uuid").fetch_one()
    if request["require_secondary"] and (not role or role[0] != "SECONDARY"):
        print("CYLISM_PRIMARY_NOT_SECONDARY")
        sys.exit(3)
    if not role or role[0] != "PRIMARY":
        cluster.set_primary_instance(request["endpoint"])
    # Verify the connected target's real role after the switch.
    role = session.run_sql("SELECT MEMBER_ROLE FROM performance_schema.replication_group_members WHERE MEMBER_ID = @@server_uuid").fetch_one()
    if not role or role[0] != "PRIMARY":
        raise RuntimeError("target instance is not PRIMARY")
    print("CYLISM_PRIMARY_CONFIGURED")
except Exception:
    # MySQL Shell errors may embed connection credentials; do not emit them.
    print("CYLISM_PRIMARY_FAILED", file=sys.stderr)
    sys.exit(1)
`

var ErrPrimaryNotReady = errors.New("数据库成员尚未全部在线")
var ErrPrimaryNotSecondary = errors.New("目标实例已不是副本")

func PreferredPrimaryPod(item *model.MySQLCluster, pods []ClusterPod) (string, error) {
	for _, pod := range pods {
		if pod.Component == "mysqld" && pod.NodeName == item.PreferredPrimaryNode && pod.Ready {
			return pod.Name, nil
		}
	}
	return "", fmt.Errorf("优先主节点 %s 上没有就绪的数据库实例：%w", item.PreferredPrimaryNode, ErrPrimaryNotReady)
}

func ConfigurePreferredPrimary(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, pods []ClusterPod) error {
	return configurePreferredPrimary(ctx, client, item, pods, executeOperatorCommand)
}

func SwitchPrimary(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, podName, nodeName string) error {
	return switchPrimaryPod(ctx, client, item, podName, nodeName, true, executeOperatorCommand)
}

type operatorCommand func(context.Context, *k8s.Client, string, []byte) error

func configurePreferredPrimary(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, pods []ClusterPod, execute operatorCommand) error {
	if item.PreferredPrimaryNode == "" || item.PrimaryConfigured {
		return nil
	}
	name, err := PreferredPrimaryPod(item, pods)
	if err != nil {
		return err
	}
	err = switchPrimaryPod(ctx, client, item, name, item.PreferredPrimaryNode, false, execute)
	if err == nil || errors.Is(err, ErrPrimaryNotReady) {
		return err
	}
	return fmt.Errorf("设置节点 %s 上的初始主实例失败，请检查成员同步状态和 Operator 到数据库的连接后重试部署", item.PreferredPrimaryNode)
}

func switchPrimaryPod(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, name, nodeName string, requireSecondary bool, execute operatorCommand) error {
	target, err := client.Clientset.CoreV1().Pods(item.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if target.Labels["mysql.oracle.com/cluster"] != item.ResourceName || target.Labels["component"] != "mysqld" {
		return fmt.Errorf("目标 Pod 不属于当前 MySQL 集群")
	}
	if target.Spec.NodeName != nodeName || target.DeletionTimestamp != nil {
		return fmt.Errorf("目标实例的节点位置已变化，请刷新运行状态后重试")
	}
	if requireSecondary {
		ready := false
		for _, condition := range target.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		if !ready {
			return fmt.Errorf("目标数据库副本尚未就绪")
		}
	}
	if requireSecondary && target.Labels["mysql.oracle.com/cluster-role"] != "SECONDARY" {
		return ErrPrimaryNotSecondary
	}
	if target.Status.PodIP == "" {
		return fmt.Errorf("目标实例尚未分配 Pod IP")
	}
	secret, err := client.Clientset.CoreV1().Secrets(item.Namespace).Get(ctx, item.ResourceName+"-root", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("读取数据库管理凭据失败")
	}
	if len(secret.Data["rootPassword"]) == 0 {
		return fmt.Errorf("数据库管理凭据缺少密码")
	}
	operators, err := client.Clientset.CoreV1().Pods("mysql-operator").List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=mysql-operator"})
	if err != nil {
		return err
	}
	// The official chart has used different label layouts across releases.
	if len(operators.Items) == 0 {
		operators, err = client.Clientset.CoreV1().Pods("mysql-operator").List(ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}
	}
	operatorName := ""
	for _, pod := range operators.Items {
		if !strings.HasPrefix(pod.Name, "mysql-operator-") && pod.Labels["app.kubernetes.io/name"] != "mysql-operator" {
			continue
		}
		if pod.DeletionTimestamp != nil {
			continue
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				operatorName = pod.Name
				break
			}
		}
		if operatorName != "" {
			break
		}
	}
	if operatorName == "" {
		return fmt.Errorf("没有就绪的 MySQL Operator Pod：%w", ErrPrimaryNotReady)
	}
	input, err := json.Marshal(map[string]any{"host": target.Status.PodIP, "endpoint": net.JoinHostPort(target.Status.PodIP, "3306"), "user": string(secret.Data["rootUser"]), "password": string(secret.Data["rootPassword"]), "instances": DatabaseInstanceCount(item), "require_secondary": requireSecondary})
	if err != nil {
		return err
	}
	return execute(ctx, client, operatorName, input)
}

func executeOperatorCommand(ctx context.Context, client *k8s.Client, operatorName string, input []byte) error {
	if client.Config == nil || client.Clientset.CoreV1().RESTClient() == nil {
		return fmt.Errorf("Kubernetes Pod 命令通道不可用")
	}
	request := client.Clientset.CoreV1().RESTClient().Post().Resource("pods").Namespace("mysql-operator").Name(operatorName).SubResource("exec").VersionedParams(&corev1.PodExecOptions{Container: "mysql-operator", Command: []string{"mysqlsh", "--py", "--execute", configurePrimaryScript}, Stdin: true, Stdout: true, Stderr: true}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(client.Config, http.MethodPost, request.URL())
	if err != nil {
		return fmt.Errorf("建立主实例配置通道失败")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdin: bytes.NewReader(input), Stdout: &stdout, Stderr: &stderr})
	if strings.Contains(stdout.String(), "CYLISM_PRIMARY_WAITING") {
		return ErrPrimaryNotReady
	}
	if strings.Contains(stdout.String(), "CYLISM_PRIMARY_NOT_SECONDARY") {
		return ErrPrimaryNotSecondary
	}
	if err != nil {
		return fmt.Errorf("主实例配置命令未完成")
	}
	if !strings.Contains(stdout.String(), "CYLISM_PRIMARY_CONFIGURED") {
		return fmt.Errorf("未确认初始主实例设置成功")
	}
	return nil
}
