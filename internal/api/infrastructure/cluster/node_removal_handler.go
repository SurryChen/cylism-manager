package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	auditservice "github.com/cylism/cylism-manager/internal/service/audit"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// NodeRemovalClient uses the existing cluster Agent connection for Kubernetes operations.
type NodeRemovalClient interface {
	GetRemovalNodeContext(context.Context, string) (*k8s.RemovalNode, error)
	DrainPlanContext(context.Context, string) (*k8s.DrainPlan, error)
	DrainRemovalNodeContext(context.Context, string, string, k8s.DrainOptions) (*k8s.DrainResult, error)
	DeleteRemovalNodeContext(context.Context, string, string) error
}

type NodeRemovalHost interface {
	Check(context.Context, *model.Server, *model.ClusterDeployment) error
	Stop(context.Context, *model.Server, *model.ClusterDeployment) error
	Uninstall(context.Context, *model.Server, *model.ClusterDeployment) error
}

type nodeRemovalPlan struct {
	NodeName        string         `json:"node_name"`
	NodeUID         string         `json:"node_uid"`
	NodeMissing     bool           `json:"node_missing"`
	NodeUninstalled bool           `json:"node_uninstalled"`
	CanStart        bool           `json:"can_start"`
	Drain           *k8s.DrainPlan `json:"drain"`
}

func (h *DeploymentHandler) WithNodeRemovalClientFactory(factory func(context.Context, uint) (NodeRemovalClient, error)) *DeploymentHandler {
	h.removalClient = factory
	return h
}

// removalScope checks ownership before contacting a cluster or a host.
func (h *DeploymentHandler) removalScope(c *gin.Context) (*model.Cluster, *model.Server, bool) {
	clusterID, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "集群 ID 无效")
		return nil, nil, false
	}
	serverID, err := apiShared.ParsePositiveID(c.Param("serverID"))
	if err != nil {
		apiShared.BadRequest(c, "服务器 ID 无效")
		return nil, nil, false
	}
	cluster, err := h.store.GetCluster(clusterID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "集群不存在")
		return nil, nil, false
	}
	if err != nil {
		apiShared.InternalError(c, "读取集群失败")
		return nil, nil, false
	}
	server, err := h.store.GetServer(serverID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "服务器不存在")
		return nil, nil, false
	}
	if err != nil {
		apiShared.InternalError(c, "读取服务器失败")
		return nil, nil, false
	}
	if cluster.Distribution != "k3s" || cluster.Status != "ready" || cluster.ControlServerID == nil || *cluster.ControlServerID == serverID || server.ClusterID == nil || *server.ClusterID != clusterID || server.ClusterRole != "worker" || server.K8sNodeName == "" {
		apiShared.Conflict(c, "仅支持移出当前 K3s 集群中已绑定的工作节点")
		return nil, nil, false
	}
	if server.Host == "" || (server.SSHAuthType == "key" && server.SSHKey == "") || (server.SSHAuthType != "key" && server.SSHPassword == "") {
		apiShared.BadRequest(c, "请先补全服务器 SSH 信息")
		return nil, nil, false
	}
	running, err := h.store.ClusterHasRunningDeployment(clusterID)
	if err != nil {
		apiShared.InternalError(c, "读取集群任务失败")
		return nil, nil, false
	}
	if running {
		apiShared.Conflict(c, "请等待当前集群任务结束")
		return nil, nil, false
	}
	return cluster, server, true
}

func (h *DeploymentHandler) prepareRemoval(ctx context.Context, clusterID uint, server *model.Server) (*nodeRemovalPlan, *model.ClusterDeployment, error) {
	if h.removalClient == nil {
		return nil, nil, fmt.Errorf("节点移出功能尚未配置集群连接")
	}
	client, err := h.removalClient(ctx, clusterID)
	if err != nil {
		return nil, nil, fmt.Errorf("无法连接集群 Agent，请检查 Agent 状态")
	}
	node, err := client.GetRemovalNodeContext(ctx, server.K8sNodeName)
	previous, previousErr := h.store.LatestClusterNodeRemoval(clusterID, server.ID)
	if previousErr != nil {
		return nil, nil, fmt.Errorf("读取历史移出任务失败")
	}
	if apierrors.IsNotFound(err) {
		if previous == nil || previous.Status != "failed" || !previous.NodeStopped || previous.NodeName != server.K8sNodeName || previous.NodeUID == "" || previous.NodeMachineID == "" {
			return nil, nil, fmt.Errorf("集群中未找到节点，且没有可恢复的移出任务；请先核对服务器绑定")
		}
		return &nodeRemovalPlan{NodeName: previous.NodeName, NodeUID: previous.NodeUID, NodeMissing: true, NodeUninstalled: previous.NodeUninstalled, CanStart: true}, previous, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("读取 Kubernetes 节点失败")
	}
	if node.ControlPlane || node.UID == "" || node.MachineID == "" {
		return nil, nil, fmt.Errorf("节点不是工作节点或缺少可验证的身份信息")
	}
	if previous != nil && previous.Status == "failed" && previous.NodeName == node.Name && previous.NodeUID != node.UID {
		return nil, nil, fmt.Errorf("同名节点已重新注册，请核对服务器绑定后再操作")
	}
	drain, err := client.DrainPlanContext(ctx, node.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("读取节点排空计划失败")
	}
	cidrs, err := json.Marshal(node.PodCIDRs)
	if err != nil {
		return nil, nil, err
	}
	identity := &model.ClusterDeployment{NodeName: node.Name, NodeUID: node.UID, NodeMachineID: node.MachineID, NodePodCIDRs: string(cidrs)}
	return &nodeRemovalPlan{NodeName: node.Name, NodeUID: node.UID, CanStart: len(drain.Blocked) == 0, Drain: drain}, identity, nil
}

func (h *DeploymentHandler) NodeRemovalPlan(c *gin.Context) {
	cluster, server, ok := h.removalScope(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	plan, _, err := h.prepareRemoval(ctx, cluster.ID, server)
	if err != nil {
		apiShared.Conflict(c, err.Error())
		return
	}
	apiShared.Success(c, plan)
}

func (h *DeploymentHandler) RemoveNode(c *gin.Context) {
	cluster, server, ok := h.removalScope(c)
	if !ok {
		return
	}
	var request struct {
		ConfirmNodeName    string `json:"confirm_node_name"`
		NodeUID            string `json:"node_uid"`
		DeleteEmptyDirData bool   `json:"delete_empty_dir_data"`
		UninstallK3s       bool   `json:"uninstall_k3s"`
	}
	if c.ShouldBindJSON(&request) != nil || request.ConfirmNodeName != server.K8sNodeName || request.NodeUID == "" {
		apiShared.BadRequest(c, "请先检查节点并输入完整节点名确认")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	plan, identity, err := h.prepareRemoval(ctx, cluster.ID, server)
	if err != nil {
		apiShared.Conflict(c, err.Error())
		return
	}
	if request.NodeUID != plan.NodeUID {
		apiShared.Conflict(c, "节点身份已变化，请重新检查")
		return
	}
	if !plan.CanStart {
		apiShared.Conflict(c, "存在无法自动重建的 Pod，请先处理阻塞项")
		return
	}
	if plan.Drain != nil && len(plan.Drain.RequiresEmptyDirConfirmation) > 0 && !request.DeleteEmptyDirData {
		apiShared.BadRequest(c, "请确认允许丢弃 emptyDir 临时数据")
		return
	}
	if identity.NodeUninstalled && !request.UninstallK3s {
		apiShared.Conflict(c, "上次任务已卸载 K3s，请保留清理选项以完成移出")
		return
	}
	identity.UninstallK3s = request.UninstallK3s
	task, err := h.store.StartClusterNodeRemoval(cluster.ID, server.ID, identity)
	if errors.Is(err, store.ErrClusterDeploymentConflict) || errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.Conflict(c, "无法创建移出任务，请确认节点归属并等待其他任务结束")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "创建移出任务失败")
		return
	}
	err = auditservice.NewService(h.store).Record(auditservice.AuditEventInput{
		Action: "cluster.node_remove", ResourceType: "cluster", ResourceID: cluster.ID,
		Actor: apiShared.ActorFromContext(c), Source: model.AuditSourceAPI, Outcome: model.AuditOutcomeAccepted,
		Summary: "工作节点移出任务已创建", RequestID: apiShared.EnsureRequestID(c), OperationID: fmt.Sprintf("cluster-deployment-%d", task.ID),
		Metadata: map[string]any{"server_id": server.ID, "node_name": task.NodeName, "delete_empty_dir_data": request.DeleteEmptyDirData, "uninstall_k3s": request.UninstallK3s},
	})
	if err != nil {
		_ = h.store.FinishClusterNodeRemoval(task, false)
		apiShared.InternalError(c, "记录移出操作审计失败，任务未执行")
		return
	}
	// The background task updates its checkpoint in memory; keep the response
	// snapshot independent from those updates while Gin marshals it.
	backgroundTask := *task
	go h.runNodeRemoval(&backgroundTask, k8s.DrainOptions{DeleteEmptyDirData: request.DeleteEmptyDirData})
	apiShared.Success(c, task)
}

func (h *DeploymentHandler) runNodeRemoval(task *model.ClusterDeployment, options k8s.DrainOptions) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	log := func(message string) { _ = h.store.AppendClusterDeploymentLog(task.ID, security.Redact(message)) }
	err := h.removeWorker(ctx, task, options, log)
	if err == nil && task.UninstallK3s {
		err = h.uninstallRemovedWorker(ctx, task, log)
	}
	if err == nil {
		err = h.store.FinishClusterNodeRemoval(task, true)
	}
	if err != nil {
		log("移出失败：" + err.Error() + "。服务器绑定保留；请处理原因后重新检查并重试。节点可能已不可调度或服务已停用，不会自动恢复。")
		_ = h.store.FinishClusterNodeRemoval(task, false)
		_ = h.recordAudit(task, model.AuditOutcomeFailed, "工作节点移出失败", "")
		return
	}
	if task.UninstallK3s {
		log("工作节点已安全移出并卸载 K3s；本地集群数据已清理，服务器 SSH 记录保留")
	} else {
		log("工作节点已移出集群；本地 K3s 数据保留，可从服务器管理继续卸载")
	}
	_ = h.recordAudit(task, model.AuditOutcomeSucceeded, "工作节点移出成功", "")
}

func (h *DeploymentHandler) removeWorker(ctx context.Context, task *model.ClusterDeployment, options k8s.DrainOptions, log func(string)) error {
	server, err := h.store.GetServer(task.ServerID)
	if err != nil {
		return fmt.Errorf("读取服务器失败")
	}
	if server.ClusterID == nil || *server.ClusterID != task.ClusterID || server.ClusterRole != "worker" || server.K8sNodeName != task.NodeName {
		return fmt.Errorf("服务器归属已变化")
	}
	client, err := h.removalClient(ctx, task.ClusterID)
	if err != nil {
		return fmt.Errorf("连接集群 Agent 失败")
	}
	log("核对节点 UID、主机身份及 K3s 工作节点角色")
	if err = h.removalHost.Check(ctx, server, task); err != nil {
		return err
	}
	get := func(ctx context.Context) (*k8s.RemovalNode, error) {
		node, err := client.GetRemovalNodeContext(ctx, task.NodeName)
		if err != nil {
			return nil, err
		}
		if node.UID != task.NodeUID || node.MachineID != task.NodeMachineID || node.ControlPlane {
			return nil, fmt.Errorf("节点身份或角色已变化，操作已停止")
		}
		return node, nil
	}
	node, err := get(ctx)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if apierrors.IsNotFound(err) && !task.NodeStopped {
		return fmt.Errorf("节点已消失且任务尚未确认停用，请核对节点与服务器绑定")
	}
	if node != nil {
		log("标记节点不可调度并等待 Pod 排空；遵守 PodDisruptionBudget，不强制删除 Pod")
		err = h.waitRemoval(ctx, 10*time.Minute, func(ctx context.Context) (bool, error) {
			if _, err := get(ctx); err != nil {
				return false, err
			}
			result, err := client.DrainRemovalNodeContext(ctx, task.NodeName, task.NodeUID, options)
			if err != nil {
				return false, err
			}
			if len(result.Pending) > 0 {
				log("等待驱逐：" + result.Pending[0].Namespace + "/" + result.Pending[0].Name + " · " + result.Pending[0].Reason)
			}
			plan, err := client.DrainPlanContext(ctx, task.NodeName)
			if err != nil {
				return false, err
			}
			if len(plan.Blocked) > 0 {
				return false, fmt.Errorf("排空期间出现无法自动重建的 Pod")
			}
			return len(plan.Evictable)+len(plan.RequiresEmptyDirConfirmation) == 0, nil
		})
		if err != nil {
			return fmt.Errorf("等待 Pod 排空失败（请检查任务日志、PDB 及 Pod 终止状态）：%w", err)
		}
		if _, err = get(ctx); err != nil {
			return err
		}
	}
	log("停止并禁用 k3s-agent，停止节点残留容器，保留本地数据")
	if err = h.removalHost.Stop(ctx, server, task); err != nil {
		return err
	}
	if err = h.store.MarkClusterNodeRemovalStopped(task.ID); err != nil {
		return fmt.Errorf("保存停用检查结果失败：%w", err)
	}
	task.NodeStopped = true
	log("等待 Kubernetes 确认节点不再 Ready")
	if err = h.waitRemoval(ctx, 3*time.Minute, func(ctx context.Context) (bool, error) {
		node, err := get(ctx)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return !node.Ready, nil
	}); err != nil {
		return fmt.Errorf("节点离线检查失败：%w", err)
	}
	if _, err = get(ctx); err == nil {
		log("按预期 UID 删除 Kubernetes Node")
		if err = client.DeleteRemovalNodeContext(ctx, task.NodeName, task.NodeUID); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	if err = h.waitRemoval(ctx, time.Minute, func(ctx context.Context) (bool, error) {
		_, err := get(ctx)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}); err != nil {
		return fmt.Errorf("确认节点删除失败：%w", err)
	}
	// Recheck the stopped host before committing the platform binding change.
	if err = h.removalHost.Stop(ctx, server, task); err != nil {
		return err
	}
	log("Kubernetes 节点已删除，正在解除平台关联")
	return nil
}

func (h *DeploymentHandler) waitRemoval(ctx context.Context, timeout time.Duration, check func(ctx context.Context) (bool, error)) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	interval := h.removalPollInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	for {
		if err := waitCtx.Err(); err != nil {
			return err
		}
		done, err := check(waitCtx)
		if err != nil || done {
			return err
		}
		timer := time.NewTimer(interval)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			return waitCtx.Err()
		case <-timer.C:
		}
	}
}
