package cloud

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/service/database"
	"github.com/cylism/cylism-manager/internal/service/database/gateway"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type databaseTerminalSession struct {
	ID         string    `json:"id"`
	InstanceID uint      `json:"instance_id"`
	ClusterID  uint      `json:"cluster_id"`
	NodeName   string    `json:"node_name"`
	Account    string    `json:"account"`
	Username   string    `json:"username"`
	ExpiresAt  time.Time `json:"expires_at"`
	owner      uint
	jobName    string
	host       string
	port       int
	database   string
	connected  atomic.Bool
}

func databaseTerminalCredentials(item *model.DatabaseInstance, account string, key []byte) (string, []byte, error) {
	var username, encrypted string
	switch account {
	case "admin":
		username, encrypted = item.AdminUsername, item.EncryptedAdmin
		if item.Mode == "managed" {
			username, encrypted = "root", item.EncryptedRoot
		}
	case "business":
		username, encrypted = item.Username, item.EncryptedPassword
	default:
		return "", nil, fmt.Errorf("请选择已保存的管理账号或业务账号")
	}
	if username == "" || encrypted == "" {
		return "", nil, fmt.Errorf("该账号尚未配置凭据")
	}
	password, err := security.Decrypt(key, encrypted)
	if err != nil || password == "" || strings.ContainsAny(password, "\r\n") {
		return "", nil, fmt.Errorf("读取数据库凭据失败")
	}
	return username, []byte(password), nil
}

func shellArgument(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
func databaseClientCommand(session *databaseTerminalSession) string {
	args := []string{"mysql", "--protocol=tcp", "--ssl-mode=REQUIRED", "--host=" + session.host, "--port=" + strconv.Itoa(session.port), "--user=" + session.Username}
	if session.database != "" {
		args = append(args, "--database="+session.database)
	}
	for i := range args {
		args[i] = shellArgument(args[i])
	}
	return "stty -echo || exit 1; printf '\\036CYLISM_MYSQL_READY\\036'; IFS= read -r MYSQL_PWD || exit 1; stty echo; export MYSQL_PWD MYSQL_HISTFILE=/dev/null; exec " + strings.Join(args, " ")
}

func databaseClientJob(session *databaseTerminalSession) *batchv1.Job {
	ttl, deadline, retries := int32(300), int64(1800), int32(0)
	no, yes, uid := false, true, int64(999)
	labels := map[string]string{k8s.ManagedByLabel: k8s.ManagedByValue, "cylism.io/database-session": session.ID}
	return &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: session.jobName, Namespace: database.Namespace, Labels: labels}, Spec: batchv1.JobSpec{BackoffLimit: &retries, TTLSecondsAfterFinished: &ttl, ActiveDeadlineSeconds: &deadline, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyNever, AutomountServiceAccountToken: &no, NodeSelector: map[string]string{corev1.LabelHostname: session.NodeName},
		DNSConfig:   &corev1.PodDNSConfig{Options: []corev1.PodDNSConfigOption{{Name: "ndots", Value: func() *string { v := "1"; return &v }()}}},
		Tolerations: []corev1.Toleration{{Key: "cylism.io/default-no-schedule", Operator: corev1.TolerationOpEqual, Value: "true", Effect: corev1.TaintEffectNoSchedule}},
		Containers:  []corev1.Container{{Name: "mysql", Image: "mysql:8.0.44", Command: []string{"/bin/sh", "-c", "exec sleep 1800"}, SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: &no, ReadOnlyRootFilesystem: &yes, RunAsNonRoot: &yes, RunAsUser: &uid, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}, SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}}, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("25m"), corev1.ResourceMemory: resource.MustParse("32Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m"), corev1.ResourceMemory: resource.MustParse("128Mi")}}}},
	}}}}
}

func (h *DatabaseHandler) CreateDatabaseSession(c *gin.Context) {
	item, ok := h.get(c)
	if !ok {
		return
	}
	var req struct {
		ClusterID uint   `json:"cluster_id"`
		NodeName  string `json:"node_name"`
		Account   string `json:"account"`
	}
	if c.ShouldBindJSON(&req) != nil || req.ClusterID == 0 || req.NodeName == "" {
		apiShared.ValidationError(c, "请选择执行集群、节点和账号")
		return
	}
	if apiShared.UserID(c) == 0 {
		c.AbortWithStatus(401)
		return
	}
	username, password, err := databaseTerminalCredentials(item, req.Account, h.key)
	for i := range password {
		password[i] = 0
	}
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if h.client == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	client, err := h.client(c.Request.Context(), req.ClusterID)
	if err != nil || client == nil || client.Clientset == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	node, err := client.Clientset.CoreV1().Nodes().Get(c.Request.Context(), req.NodeName, metav1.GetOptions{})
	ready := false
	if err == nil && !node.Spec.Unschedulable {
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				ready = true
			}
		}
	}
	if !ready {
		apiShared.ValidationError(c, "执行节点尚未就绪")
		return
	}
	host, port := item.Host, item.Port
	if item.Mode == "managed" && item.ClusterID != req.ClusterID {
		apiShared.ValidationError(c, "请选择实例所在集群")
		return
	}
	var gw model.DatabaseGateway
	err = h.store.DB().Where("instance_id = ?", item.ID).First(&gw).Error
	if err == nil {
		// A failed aggregate probe may concern another node. The terminal tests
		// the chosen node's existing Service path without changing gateway ACLs.
		if gw.ClusterID != req.ClusterID || (gw.Status != "ready" && gw.Status != "degraded" && gw.Status != "error") {
			apiShared.ValidationError(c, "私网网关尚未部署或已禁用，请先检查网关")
			return
		}
		host, port = gateway.ServiceHost(gw.ID), 3306
	} else if !errors.Is(err, gorm.ErrRecordNotFound) || item.ConnectionMode == "gateway" {
		apiShared.ValidationError(c, "请先配置私网网关")
		return
	}
	raw := make([]byte, 16)
	if _, err = rand.Read(raw); err != nil {
		apiShared.ValidationError(c, "创建终端会话失败")
		return
	}
	id := hex.EncodeToString(raw)
	session := &databaseTerminalSession{ID: id, InstanceID: item.ID, ClusterID: req.ClusterID, NodeName: req.NodeName, Account: req.Account, Username: username, ExpiresAt: time.Now().Add(30 * time.Minute), owner: apiShared.UserID(c), jobName: "cylism-db-cli-" + id, host: host, port: port}
	if req.Account == "business" {
		session.database = item.DatabaseName
	}
	if _, err = client.Clientset.CoreV1().Namespaces().Get(c.Request.Context(), database.Namespace, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		_, err = client.Clientset.CoreV1().Namespaces().Create(c.Request.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: database.Namespace}}, metav1.CreateOptions{})
	}
	if err != nil {
		apiShared.ValidationError(c, "创建终端命名空间失败")
		return
	}
	if _, err = client.Clientset.BatchV1().Jobs(database.Namespace).Create(c.Request.Context(), databaseClientJob(session), metav1.CreateOptions{}); err != nil {
		apiShared.ValidationError(c, "创建临时 MySQL 客户端失败")
		return
	}
	h.databaseSessions.Store(id, session)
	time.AfterFunc(time.Until(session.ExpiresAt), func() { h.cleanupDatabaseSession(session) })
	apiShared.Success(c, session)
}

func (h *DatabaseHandler) getDatabaseSession(c *gin.Context) (*databaseTerminalSession, bool) {
	value, ok := h.databaseSessions.Load(c.Param("sessionID"))
	if !ok {
		apiShared.NotFound(c, "终端会话不存在或已过期，请重新打开")
		return nil, false
	}
	session := value.(*databaseTerminalSession)
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil || id != session.InstanceID || session.owner != apiShared.UserID(c) || !time.Now().Before(session.ExpiresAt) {
		apiShared.NotFound(c, "终端会话不存在或已过期")
		return nil, false
	}
	return session, true
}

func (h *DatabaseHandler) cleanupDatabaseSession(session *databaseTerminalSession) {
	if _, loaded := h.databaseSessions.LoadAndDelete(session.ID); !loaded {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := h.client(ctx, session.ClusterID)
	if err != nil || client == nil || client.Clientset == nil {
		return
	}
	policy := metav1.DeletePropagationBackground
	_ = client.Clientset.BatchV1().Jobs(database.Namespace).Delete(ctx, session.jobName, metav1.DeleteOptions{PropagationPolicy: &policy})
}
func (h *DatabaseHandler) DeleteDatabaseSession(c *gin.Context) {
	session, ok := h.getDatabaseSession(c)
	if !ok {
		return
	}
	h.cleanupDatabaseSession(session)
	apiShared.Success(c, gin.H{"closed": true})
}
func (h *DatabaseHandler) DatabaseTerminal(c *gin.Context) {
	session, ok := h.getDatabaseSession(c)
	if !ok {
		return
	}
	if !session.connected.CompareAndSwap(false, true) {
		apiShared.ValidationError(c, "此会话已连接，请重新创建终端")
		return
	}
	defer h.cleanupDatabaseSession(session)
	item, ok := h.get(c)
	if !ok {
		return
	}
	username, password, err := databaseTerminalCredentials(item, session.Account, h.key)
	if err != nil || username != session.Username {
		apiShared.ValidationError(c, "账号凭据已变更，请重新打开终端")
		return
	}
	defer func() {
		for i := range password {
			password[i] = 0
		}
	}()
	client, err := h.client(c.Request.Context(), session.ClusterID)
	if err != nil || client == nil || client.Config == nil || client.Clientset == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		pods, e := client.Clientset.CoreV1().Pods(database.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "cylism.io/database-session=" + session.ID})
		if e != nil {
			apiShared.ValidationError(c, "读取临时客户端状态失败")
			return
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
				apiShared.ValidationError(c, "临时客户端已退出，请重新打开")
				return
			}
			if pod.Status.Phase == corev1.PodRunning && pod.DeletionTimestamp == nil {
				for _, state := range pod.Status.ContainerStatuses {
					if state.Name == "mysql" && state.Ready {
						if bytes.ContainsAny(password, "\r\n") {
							apiShared.ValidationError(c, "账号凭据格式无效")
							return
						}
						serveMySQLTerminal(c, client, database.Namespace, pod.Name, "mysql", databaseClientCommand(session), password)
						return
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			apiShared.ValidationError(c, "等待临时客户端就绪超时，请检查所选节点的镜像拉取情况")
			return
		case <-ticker.C:
		}
	}
}
