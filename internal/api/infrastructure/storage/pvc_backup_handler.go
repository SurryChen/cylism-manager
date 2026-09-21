package storage

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	storageservice "github.com/cylism/cylism-manager/internal/service/storage"
	"github.com/cylism/cylism-manager/internal/transport"
	"github.com/gin-gonic/gin"
)

type persistentVolumeBackupRequest struct {
	EnvironmentID      uint   `json:"environment_id"`
	BackupServerID     uint   `json:"backup_server_id"`
	BackupRoot         string `json:"backup_root"`
	ConfirmDataReplace bool   `json:"confirm_data_replace"`
}

func (h *StorageHandler) ListPersistentVolumeBackups(c *gin.Context) {
	environment, ok := h.pvcEnvironment(c)
	if !ok {
		return
	}
	backups, err := h.Service.ListBackups(environment.ID, c.Param("name"))
	if err != nil {
		apiShared.DBError(c, "读取存储卷备份失败")
		return
	}
	apiShared.Success(c, apiShared.PersistentVolumeBackupsDTO(backups))
}

func (h *StorageHandler) CreatePersistentVolumeBackup(c *gin.Context) {
	if h.pvc == nil || h.migration == nil || h.store == nil {
		storageK8sUnavailable(c)
		return
	}
	var request persistentVolumeBackupRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.EnvironmentID == 0 || request.BackupServerID == 0 {
		apiShared.BadRequest(c, "环境和备份服务器必填")
		return
	}
	environment, err := h.store.GetEnvironmentByID(request.EnvironmentID)
	if err != nil {
		apiShared.NotFound(c, "环境不存在")
		return
	}
	claim, err := h.pvc.GetManagedPVCContext(c.Request.Context(), environment.Namespace, c.Param("name"), environment.ID)
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	if !claim.IsLocal || claim.BoundNode == "" || claim.LocalPath == "" {
		apiShared.ValidationError(c, "当前仅支持已绑定的 local-path 或 hostPath PVC 备份")
		return
	}
	root, rootErr := storageservice.ValidateBackupRoot(request.BackupRoot)
	if rootErr != nil {
		apiShared.ValidationError(c, rootErr.Error())
		return
	}
	if _, err := h.store.GetServer(request.BackupServerID); err != nil {
		apiShared.NotFound(c, "备份服务器不存在")
		return
	}
	backup := &model.PersistentVolumeBackup{EnvironmentID: environment.ID, PVCName: claim.Name, SourceNodeName: claim.BoundNode, BackupServerID: request.BackupServerID, BackupPath: path.Join(root, environment.Namespace, claim.Name, time.Now().UTC().Format("20060102T150405Z")), Status: "accepted", CreatedBy: storageRequestUserID(c)}
	if err := h.Service.CreateBackup(backup); err != nil {
		apiShared.DBError(c, "创建存储卷备份记录失败")
		return
	}
	if err := h.Service.StartBackup(c.Request.Context(), backup.ID); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, apiShared.PersistentVolumeBackupDTO(backup), "存储卷备份已创建")
}

func (h *StorageHandler) RestorePersistentVolumeBackup(c *gin.Context) {
	if h.pvc == nil || h.migration == nil || h.store == nil {
		storageK8sUnavailable(c)
		return
	}
	var request persistentVolumeBackupRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.EnvironmentID == 0 || !request.ConfirmDataReplace {
		apiShared.BadRequest(c, "恢复需要环境并确认覆盖当前 PVC 数据")
		return
	}
	backupID, err := apiShared.ParsePositiveID(strings.TrimSpace(c.Param("backupID")))
	if err != nil {
		apiShared.BadRequest(c, "备份 ID 无效")
		return
	}
	backup, err := h.Service.GetBackup(backupID)
	if err != nil || backup.EnvironmentID != request.EnvironmentID || backup.PVCName != c.Param("name") {
		apiShared.NotFound(c, "备份不存在")
		return
	}
	if backup.Status != "succeeded" {
		apiShared.ValidationError(c, "只有成功备份可以恢复")
		return
	}
	if backup.RestoreStatus == "running" {
		apiShared.Conflict(c, "该备份正在恢复")
		return
	}
	now := time.Now().UTC()
	backup.RestoreStatus = "running"
	backup.RestoreDetail = ""
	backup.RestoreStartedAt = &now
	backup.RestoreCompletedAt = nil
	if err := h.Service.UpdateBackup(backup); err != nil {
		apiShared.DBError(c, "更新存储卷恢复状态失败")
		return
	}
	if err := h.Service.StartRestore(c.Request.Context(), backup.ID); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, apiShared.PersistentVolumeBackupDTO(backup), "存储卷恢复已开始")
}

func (h *StorageHandler) executePersistentVolumeBackup(parent context.Context, backupID uint) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Hour)
	defer cancel()
	backup, err := h.Service.GetBackup(backupID)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	backup.Status, backup.StartedAt = "running", &now
	_ = h.Service.UpdateBackup(backup)
	environment, err := h.store.GetEnvironmentByID(backup.EnvironmentID)
	if err != nil {
		h.failPersistentVolumeBackup(backup, err)
		return
	}
	claim, err := h.pvc.GetManagedPVCContext(ctx, environment.Namespace, backup.PVCName, environment.ID)
	if err != nil || !claim.IsLocal || claim.LocalPath == "" || claim.BoundNode != backup.SourceNodeName {
		h.failPersistentVolumeBackup(backup, fmt.Errorf("PVC 绑定状态已变化，无法执行备份"))
		return
	}
	source, err := h.serverForK8sNode(backup.SourceNodeName)
	if err != nil {
		h.failPersistentVolumeBackup(backup, err)
		return
	}
	target, err := h.store.GetServer(backup.BackupServerID)
	if err != nil {
		h.failPersistentVolumeBackup(backup, err)
		return
	}
	bytesCopied, err := streamPVCData(ctx, source, target, claim.LocalPath, backup.BackupPath, h.encKey)
	if err != nil {
		h.failPersistentVolumeBackup(backup, err)
		return
	}
	backup.Status, backup.Bytes, backup.Detail = "succeeded", bytesCopied, "PVC 数据已复制到备份服务器"
	completedAt := time.Now().UTC()
	backup.CompletedAt = &completedAt
	_ = h.Service.UpdateBackup(backup)
}

func (h *StorageHandler) executePersistentVolumeRestore(parent context.Context, backupID uint) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Hour)
	defer cancel()
	backup, err := h.Service.GetBackup(backupID)
	if err != nil {
		return
	}
	environment, err := h.store.GetEnvironmentByID(backup.EnvironmentID)
	if err != nil {
		h.failPersistentVolumeRestore(backup, err)
		return
	}
	claim, err := h.pvc.GetManagedPVCContext(ctx, environment.Namespace, backup.PVCName, environment.ID)
	if err != nil || !claim.IsLocal || claim.LocalPath == "" {
		h.failPersistentVolumeRestore(backup, fmt.Errorf("PVC 不再是可恢复的本地卷"))
		return
	}
	source, err := h.serverForK8sNode(claim.BoundNode)
	if err != nil {
		h.failPersistentVolumeRestore(backup, err)
		return
	}
	archive, err := h.store.GetServer(backup.BackupServerID)
	if err != nil {
		h.failPersistentVolumeRestore(backup, err)
		return
	}
	deployments, err := h.migration.DeploymentUsingPVC(ctx, environment.Namespace, backup.PVCName)
	if err != nil {
		h.failPersistentVolumeRestore(backup, err)
		return
	}
	replicas := make(map[string]int32, len(deployments))
	for _, deployment := range deployments {
		if deployment.Labels[k8sclient.ManagedByLabel] != k8sclient.ManagedByValue {
			h.failPersistentVolumeRestore(backup, fmt.Errorf("PVC 正被非平台托管的 Deployment %q 使用，拒绝自动恢复", deployment.Name))
			return
		}
		count := int32(1)
		if deployment.Spec.Replicas != nil {
			count = *deployment.Spec.Replicas
		}
		if err := h.migration.ScaleDeployment(ctx, environment.Namespace, deployment.Name, 0); err != nil {
			h.failPersistentVolumeRestore(backup, err)
			return
		}
		replicas[deployment.Name] = count
		if err := h.waitForStorageDeploymentPods(ctx, environment.Namespace, deployment.Name, false, 90*time.Second); err != nil {
			h.failPersistentVolumeRestore(backup, err)
			return
		}
	}
	clear := "sudo -n find " + storageShellQuote(claim.LocalPath) + " -mindepth 1 -maxdepth 1 -exec rm -rf {} +"
	if out, err := transport.SSHExecServerContext(ctx, 30*time.Second, source, h.encKey, clear); err != nil {
		h.restoreBackupReplicas(ctx, environment.Namespace, replicas)
		h.failPersistentVolumeRestore(backup, fmt.Errorf("清空目标 PVC 失败: %s", strings.TrimSpace(string(out))))
		return
	}
	if _, err := streamPVCData(ctx, archive, source, backup.BackupPath, claim.LocalPath, h.encKey); err != nil {
		h.restoreBackupReplicas(ctx, environment.Namespace, replicas)
		h.failPersistentVolumeRestore(backup, err)
		return
	}
	h.restoreBackupReplicas(ctx, environment.Namespace, replicas)
	backup.RestoreStatus = "succeeded"
	backup.RestoreDetail = "PVC 数据已从备份恢复"
	completedAt := time.Now().UTC()
	backup.RestoreCompletedAt = &completedAt
	_ = h.Service.UpdateBackup(backup)
}

func (h *StorageHandler) restoreBackupReplicas(ctx context.Context, namespace string, replicas map[string]int32) {
	for name, count := range replicas {
		_ = h.migration.ScaleDeployment(ctx, namespace, name, count)
	}
}

func (h *StorageHandler) serverForK8sNode(nodeName string) (*model.Server, error) {
	servers, err := h.store.ListServers()
	if err != nil {
		return nil, err
	}
	for index := range servers {
		if servers[index].K8sNodeName == nodeName {
			return &servers[index], nil
		}
	}
	return nil, fmt.Errorf("节点 %q 未关联受管 SSH 服务器", nodeName)
}

func (h *StorageHandler) failPersistentVolumeBackup(backup *model.PersistentVolumeBackup, err error) {
	backup.Status, backup.Detail = "failed", err.Error()
	completedAt := time.Now().UTC()
	backup.CompletedAt = &completedAt
	_ = h.Service.UpdateBackup(backup)
}

func (h *StorageHandler) failPersistentVolumeRestore(backup *model.PersistentVolumeBackup, err error) {
	backup.RestoreStatus = "failed"
	backup.RestoreDetail = err.Error()
	completedAt := time.Now().UTC()
	backup.RestoreCompletedAt = &completedAt
	_ = h.Service.UpdateBackup(backup)
}
