package database

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const memberRolesScript = `import json, sys
import mysqlsh
try:
    request = json.load(sys.stdin)
    session = mysqlsh.globals.shell.open_session({"scheme": "mysql", "host": request["host"], "port": 3306, "user": request["user"], "password": request["password"]})
    rows = session.run_sql("SELECT MEMBER_HOST, MEMBER_ROLE, MEMBER_STATE FROM performance_schema.replication_group_members").fetch_all()
    print("CYLISM_MEMBER_ROLES=" + json.dumps([{"host": row[0], "role": row[1], "state": row[2]} for row in rows]))
    session.close()
except Exception:
    print("CYLISM_MEMBER_ROLES_FAILED", file=sys.stderr)
    sys.exit(1)
`

type MemberRole struct {
	Name         string `json:"name"`
	DatabaseRole string `json:"database_role"`
	MemberState  string `json:"member_state"`
}

type memberObservation struct {
	Host  string `json:"host"`
	Role  string `json:"role"`
	State string `json:"state"`
}

type operatorQuery func(context.Context, *k8s.Client, string, []byte, string) ([]byte, error)

// MemberRoles is deliberately independent of Snapshot: a slow database query
// must not hold the cluster task lock or delay resource and task polling.
func MemberRoles(ctx context.Context, client *k8s.Client, item *model.MySQLCluster) ([]MemberRole, error) {
	return memberRoles(ctx, client, item, executeOperatorScript)
}

func memberRoles(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, execute operatorQuery) ([]MemberRole, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	pods, err := client.Clientset.CoreV1().Pods(item.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "mysql.oracle.com/cluster=" + item.ResourceName})
	if err != nil {
		return nil, fmt.Errorf("读取数据库实例失败")
	}
	var members []corev1.Pod
	for _, pod := range pods.Items {
		if pod.Labels["component"] == "mysqld" && pod.DeletionTimestamp == nil {
			members = append(members, pod)
		}
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Name < members[j].Name })
	operator, credentials, err := operatorAccess(ctx, client, item)
	if err != nil {
		return nil, err
	}
	for _, pod := range members {
		ready := false
		for _, condition := range pod.Status.Conditions {
			ready = ready || (condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue)
		}
		if !ready || pod.Status.PodIP == "" {
			continue
		}
		input, _ := json.Marshal(map[string]string{"host": pod.Status.PodIP, "user": credentials["user"], "password": credentials["password"]})
		attempt, stop := context.WithTimeout(ctx, 10*time.Second)
		output, queryErr := execute(attempt, client, operator, input, memberRolesScript)
		stop()
		if queryErr == nil {
			if roles, parseErr := parseMemberRoles(output, item, members); parseErr == nil {
				return roles, nil
			}
		}
		if ctx.Err() != nil {
			break
		}
	}
	// Never return raw MySQL Shell output: it can contain credentials.
	return nil, fmt.Errorf("无法查询 MySQL 实时角色，请检查 Operator 到数据库的连接")
}

func parseMemberRoles(output []byte, item *model.MySQLCluster, pods []corev1.Pod) ([]MemberRole, error) {
	const marker = "CYLISM_MEMBER_ROLES="
	var observations []memberObservation
	found := false
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, marker) {
			if found || json.Unmarshal([]byte(strings.TrimPrefix(line, marker)), &observations) != nil {
				return nil, fmt.Errorf("数据库角色响应无效")
			}
			found = true
		}
	}
	if !found || len(observations) == 0 {
		return nil, fmt.Errorf("未查询到数据库成员角色")
	}
	hosts := map[string]string{}
	for _, pod := range pods {
		base := pod.Name + "." + item.ResourceName + "-instances"
		for _, host := range []string{pod.Name, pod.Status.PodIP, base, base + "." + item.Namespace, base + "." + item.Namespace + ".svc", base + "." + item.Namespace + ".svc.cluster.local"} {
			if host != "" {
				hosts[host] = pod.Name
			}
		}
	}
	roles := []MemberRole{}
	seen := map[string]bool{}
	primaryCount := 0
	for _, member := range observations {
		name := hosts[strings.TrimSuffix(member.Host, ".")]
		if name == "" {
			continue
		}
		if seen[name] {
			return nil, fmt.Errorf("数据库成员角色重复")
		}
		seen[name] = true
		role := ""
		if member.State == "ONLINE" && (member.Role == "PRIMARY" || member.Role == "SECONDARY") {
			role = member.Role
			if role == "PRIMARY" {
				primaryCount++
			}
		}
		roles = append(roles, MemberRole{Name: name, DatabaseRole: role, MemberState: member.State})
	}
	if len(roles) == 0 || primaryCount != 1 {
		return nil, fmt.Errorf("无法确认当前主实例")
	}
	return roles, nil
}
