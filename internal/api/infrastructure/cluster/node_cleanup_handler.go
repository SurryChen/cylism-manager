package cluster

import (
	"context"
	"fmt"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	auditservice "github.com/cylism/cylism-manager/internal/service/audit"
	"github.com/gin-gonic/gin"
)

// Server-level cleanup uses the durable identity saved by a successful removal;
// it does not need a live Agent or even the original logical cluster record.
func (h *DeploymentHandler) ServerK3sCleanupStatus(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "服务器 ID 无效")
		return
	}
	server, err := h.store.GetServer(id)
	if err != nil {
		apiShared.NotFound(c, "服务器不存在")
		return
	}
	var source *model.ClusterDeployment
	if server.K3sRemovalTaskID != nil {
		source, err = h.store.ServerK3sCleanupSource(id)
		if err != nil {
			apiShared.Conflict(c, "服务器关联已变化，请刷新后重试")
			return
		}
	}
	task, err := h.store.LatestServerK3sCleanup(id)
	if err != nil {
		apiShared.InternalError(c, "读取清理任务失败")
		return
	}
	// Do not surface an older installation's cleanup as the current one.
	if source != nil && task != nil && task.ID < source.ID {
		task = nil
	}
	logs := []model.ClusterDeploymentLog{}
	if task != nil {
		logs, err = h.store.ListClusterDeploymentLogs(task.ID, 0)
		if err != nil {
			apiShared.InternalError(c, "读取清理日志失败")
			return
		}
	}
	apiShared.Success(c, gin.H{"source": source, "task": task, "logs": logs})
}

func (h *DeploymentHandler) CleanupServerK3s(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "服务器 ID 无效")
		return
	}
	var request struct {
		RemovalTaskID   uint   `json:"removal_task_id"`
		ConfirmNodeName string `json:"confirm_node_name"`
	}
	if c.ShouldBindJSON(&request) != nil || request.RemovalTaskID == 0 {
		apiShared.BadRequest(c, "请检查移出记录并输入完整节点名确认")
		return
	}
	source, err := h.store.ServerK3sCleanupSource(id)
	if err != nil || source.ID != request.RemovalTaskID || source.NodeName == "" || source.NodeName != request.ConfirmNodeName {
		apiShared.Conflict(c, "仅支持清理已安全移出且尚未重新绑定的工作节点，请核对节点名")
		return
	}
	server, err := h.store.GetServer(id)
	if err != nil {
		apiShared.NotFound(c, "服务器不存在")
		return
	}
	if server.Host == "" || (server.SSHAuthType == "key" && server.SSHKey == "") || (server.SSHAuthType != "key" && server.SSHPassword == "") {
		apiShared.BadRequest(c, "请先补全服务器 SSH 信息")
		return
	}
	task, err := h.store.StartServerK3sCleanup(id, source.ID)
	if err != nil {
		apiShared.Conflict(c, "无法创建清理任务，请刷新并等待当前任务结束")
		return
	}
	err = auditservice.NewService(h.store).Record(auditservice.AuditEventInput{
		Action: "cluster.node_cleanup", ResourceType: "server", ResourceID: id,
		Actor: apiShared.ActorFromContext(c), Source: model.AuditSourceAPI, Outcome: model.AuditOutcomeAccepted,
		Summary: "已移出节点的 K3s 清理任务已创建", RequestID: apiShared.EnsureRequestID(c), OperationID: fmt.Sprintf("cluster-deployment-%d", task.ID),
		Metadata: map[string]any{"server_id": id, "removal_task_id": source.ID, "node_name": source.NodeName},
	})
	if err != nil {
		_ = h.store.FinishServerK3sCleanup(task, false)
		apiShared.InternalError(c, "记录审计失败，任务未执行")
		return
	}
	background := *task
	go h.runServerK3sCleanup(&background)
	apiShared.Success(c, task)
}

func (h *DeploymentHandler) uninstallRemovedWorker(ctx context.Context, task *model.ClusterDeployment, log func(string)) error {
	server, err := h.store.GetServer(task.ServerID)
	if err != nil {
		return fmt.Errorf("读取服务器失败")
	}
	if task.Operation == "cleanup" {
		source, err := h.store.ServerK3sCleanupSource(task.ServerID)
		if err != nil || source.NodeMachineID != task.NodeMachineID || source.NodeUID != task.NodeUID {
			return fmt.Errorf("服务器或移出记录已变化，停止卸载")
		}
	} else if server.ClusterID == nil || *server.ClusterID != task.ClusterID || server.ClusterRole != "worker" || server.K8sNodeName != task.NodeName {
		return fmt.Errorf("服务器集群归属已变化，停止卸载")
	}
	log("核对已停用主机身份，通过官方脚本卸载 K3s 并清理本地集群数据")
	if err := h.removalHost.Uninstall(ctx, server, task); err != nil {
		return err
	}
	if err := h.store.MarkClusterNodeUninstalled(task.ID); err != nil {
		return fmt.Errorf("保存卸载检查结果失败：%w", err)
	}
	task.NodeUninstalled = true
	return nil
}

func (h *DeploymentHandler) runServerK3sCleanup(task *model.ClusterDeployment) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	log := func(message string) { _ = h.store.AppendClusterDeploymentLog(task.ID, security.Redact(message)) }
	err := h.uninstallRemovedWorker(ctx, task, log)
	if err == nil {
		err = h.store.FinishServerK3sCleanup(task, true)
	}
	if err != nil {
		log("清理失败：" + err.Error() + "。保留移出记录，可检查主机后重试。")
		_ = h.store.FinishServerK3sCleanup(task, false)
		_ = h.recordAudit(task, model.AuditOutcomeFailed, "已移出节点清理失败", "")
		return
	}
	log("K3s 服务及本地集群数据已清理；保留服务器记录和 SSH 信息")
	_ = h.recordAudit(task, model.AuditOutcomeSucceeded, "已移出节点清理完成", "")
}
