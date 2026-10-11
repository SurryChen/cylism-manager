package cluster

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/cylism/cylism-manager/internal/transport"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *DeploymentHandler) RestartNode(c *gin.Context) {
	clusterID, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "集群 ID 无效")
		return
	}
	serverID, err := apiShared.ParsePositiveID(c.Param("serverID"))
	if err != nil {
		apiShared.BadRequest(c, "服务器 ID 无效")
		return
	}
	server, err := h.store.GetServer(serverID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "服务器不存在")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "读取服务器失败")
		return
	}
	if server.Host == "" || (server.SSHAuthType == "key" && server.SSHKey == "") || (server.SSHAuthType != "key" && server.SSHPassword == "") {
		apiShared.BadRequest(c, "请先补全服务器 SSH 信息")
		return
	}
	task, err := h.store.StartClusterNodeRestart(clusterID, serverID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "集群不存在")
		return
	}
	if errors.Is(err, store.ErrClusterDeploymentConflict) {
		apiShared.Conflict(c, "仅可重启当前集群的 K3s 节点；请确认节点名已获取且没有其他集群任务运行")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "创建节点重启任务失败")
		return
	}
	_ = h.recordAudit(task, model.AuditOutcomeAccepted, "K3s 节点重启任务已创建", apiShared.EnsureRequestID(c))
	go h.runNodeRestart(task)
	apiShared.Success(c, task)
}

func (h *DeploymentHandler) runNodeRestart(task *model.ClusterDeployment) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	log := func(message string) { _ = h.store.AppendClusterDeploymentLog(task.ID, message) }
	service := "k3s-agent"
	if task.Role == "control-plane" {
		service = "k3s"
	}
	log("开始重启 " + service + " 服务")
	err := h.restartNodeService(ctx, task, service, log)
	if err != nil {
		log("节点重启失败：" + safeDeploymentError(err))
		_ = h.store.FinishClusterNodeRestart(task, false)
		_ = h.recordAudit(task, model.AuditOutcomeFailed, "K3s 节点重启失败", "")
		return
	}
	if err := h.store.FinishClusterNodeRestart(task, true); err != nil {
		log("保存节点重启结果失败：" + safeDeploymentError(err))
		_ = h.recordAudit(task, model.AuditOutcomeFailed, "保存 K3s 节点重启结果失败", "")
		return
	}
	log("K3s 服务已重启，节点已恢复 Ready")
	_ = h.recordAudit(task, model.AuditOutcomeSucceeded, "K3s 节点重启成功", "")
}

func (h *DeploymentHandler) restartNodeService(ctx context.Context, task *model.ClusterDeployment, service string, log func(string)) error {
	server, err := h.store.GetServer(task.ServerID)
	if err != nil {
		return fmt.Errorf("读取节点服务器失败: %w", err)
	}
	cluster, err := h.store.GetCluster(task.ClusterID)
	if err != nil || cluster.ControlServerID == nil {
		return fmt.Errorf("读取控制平面失败")
	}
	control, err := h.store.GetServer(*cluster.ControlServerID)
	if err != nil {
		return fmt.Errorf("读取控制平面服务器失败: %w", err)
	}
	output, err := transport.SSHExecServerContext(ctx, 90*time.Second, server, h.key, privilegedCommand(server, "systemctl restart "+service))
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if len(detail) > 1000 {
			detail = detail[:1000]
		}
		if detail != "" {
			return fmt.Errorf("重启 %s 失败: %s (%w)", service, detail, err)
		}
		return fmt.Errorf("重启 %s 失败: %w", service, err)
	}
	log(service + " 重启命令已完成，等待节点恢复 Ready")
	deadline := time.NewTimer(3 * time.Minute)
	defer deadline.Stop()
	for checks := 0; ; checks++ {
		active, activeErr := transport.SSHExecServerContext(ctx, 15*time.Second, server, h.key, privilegedCommand(server, "systemctl is-active "+service))
		if activeErr == nil && strings.TrimSpace(string(active)) == "active" {
			output, listErr := transport.SSHExecServerContext(ctx, 20*time.Second, control, h.key, privilegedCommand(control, "k3s kubectl get nodes -o json"))
			if listErr == nil {
				states, parseErr := parseMigrationKubeNodes(output)
				if parseErr == nil && states[strings.ToLower(server.K8sNodeName)].Ready {
					return nil
				}
			}
		}
		if checks > 0 && checks%6 == 0 {
			log("仍在等待 " + server.K8sNodeName + " 恢复 Ready")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("等待节点 %s 恢复 Ready 超时，请检查 %s 服务与 Kubernetes 节点状态", server.K8sNodeName, service)
		case <-time.After(5 * time.Second):
		}
	}
}
