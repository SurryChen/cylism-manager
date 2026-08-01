package api

import (
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

type persistentVolumeBackupRequest struct {
	EnvironmentID      uint   `json:"environment_id"`
	BackupServerID     uint   `json:"backup_server_id"`
	BackupRoot         string `json:"backup_root"`
	ConfirmDataReplace bool   `json:"confirm_data_replace"`
}

func (h *K8sHandler) ListPersistentVolumeBackups(c *gin.Context) {
	environment, ok := h.pvcEnvironment(c)
	if !ok {
		return
	}
	backups, err := h.store.ListPersistentVolumeBackups(environment.ID, c.Param("name"))
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取存储卷备份失败")
		return
	}
	model.Success(c, backups)
}

func (h *K8sHandler) CreatePersistentVolumeBackup(c *gin.Context) {
	if K8s == nil || h.store == nil {
		k8sUnavailable(c)
		return
	}
	var request persistentVolumeBackupRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.EnvironmentID == 0 || request.BackupServerID == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "环境和备份服务器必填")
		return
	}
	environment, err := h.store.GetEnvironmentByID(request.EnvironmentID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "环境不存在")
		return
	}
	claim, err := K8s.GetManagedPVC(environment.Namespace, c.Param("name"), environment.ID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, err.Error())
		return
	}
	if !claim.IsLocal || claim.BoundNode == "" || claim.LocalPath == "" {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "当前仅支持已绑定的 local-path 或 hostPath PVC 备份")
		return
	}
	root := path.Clean(strings.TrimSpace(request.BackupRoot))
	if !strings.HasPrefix(root, "/") || root == "/" {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "备份根目录必须是非根目录的绝对路径")
		return
	}
	if _, err := h.store.GetServer(request.BackupServerID); err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "备份服务器不存在")
		return
	}
	backup := &model.PersistentVolumeBackup{EnvironmentID: environment.ID, PVCName: claim.Name, SourceNodeName: claim.BoundNode, BackupServerID: request.BackupServerID, BackupPath: path.Join(root, environment.Namespace, claim.Name, time.Now().UTC().Format("20060102T150405Z")), Status: "accepted", CreatedBy: getUserID(c)}
	if err := h.store.CreatePersistentVolumeBackup(backup); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "创建存储卷备份记录失败")
		return
	}
	go h.executePersistentVolumeBackup(backup.ID)
	model.SuccessWithMessage(c, backup, "存储卷备份已创建")
}

func (h *K8sHandler) RestorePersistentVolumeBackup(c *gin.Context) {
	if K8s == nil || h.store == nil {
		k8sUnavailable(c)
		return
	}
	var request persistentVolumeBackupRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.EnvironmentID == 0 || !request.ConfirmDataReplace {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "恢复需要环境并确认覆盖当前 PVC 数据")
		return
	}
	backupID, err := parseID(c.Param("backupID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "备份 ID 无效")
		return
	}
	backup, err := h.store.GetPersistentVolumeBackup(backupID)
	if err != nil || backup.EnvironmentID != request.EnvironmentID || backup.PVCName != c.Param("name") {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "备份不存在")
		return
	}
	if backup.Status != "succeeded" {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "只有成功备份可以恢复")
		return
	}
	if backup.RestoreStatus == "running" {
		model.Error(c, http.StatusConflict, model.CodeConflict, "该备份正在恢复")
		return
	}
	now := time.Now().UTC()
	backup.RestoreStatus = "running"
	backup.RestoreDetail = ""
	backup.RestoreStartedAt = &now
	backup.RestoreCompletedAt = nil
	if err := h.store.UpdatePersistentVolumeBackup(backup); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "更新存储卷恢复状态失败")
		return
	}
	go h.executePersistentVolumeRestore(backup.ID)
	model.SuccessWithMessage(c, backup, "存储卷恢复已开始")
}

func (h *K8sHandler) executePersistentVolumeBackup(backupID uint) {
	backup, err := h.store.GetPersistentVolumeBackup(backupID)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	backup.Status, backup.StartedAt = "running", &now
	_ = h.store.UpdatePersistentVolumeBackup(backup)
	environment, err := h.store.GetEnvironmentByID(backup.EnvironmentID)
	if err != nil {
		h.failPersistentVolumeBackup(backup, err)
		return
	}
	claim, err := K8s.GetManagedPVC(environment.Namespace, backup.PVCName, environment.ID)
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
	bytesCopied, err := h.streamPVCData(source, target, claim.LocalPath, backup.BackupPath)
	if err != nil {
		h.failPersistentVolumeBackup(backup, err)
		return
	}
	backup.Status, backup.Bytes, backup.Detail = "succeeded", bytesCopied, "PVC 数据已复制到备份服务器"
	completedAt := time.Now().UTC()
	backup.CompletedAt = &completedAt
	_ = h.store.UpdatePersistentVolumeBackup(backup)
}

func (h *K8sHandler) executePersistentVolumeRestore(backupID uint) {
	backup, err := h.store.GetPersistentVolumeBackup(backupID)
	if err != nil {
		return
	}
	environment, err := h.store.GetEnvironmentByID(backup.EnvironmentID)
	if err != nil {
		h.failPersistentVolumeRestore(backup, err)
		return
	}
	claim, err := K8s.GetManagedPVC(environment.Namespace, backup.PVCName, environment.ID)
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
	deployments, err := K8s.DeploymentUsingPVC(environment.Namespace, backup.PVCName)
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
		if err := K8s.ScaleDeployment(environment.Namespace, deployment.Name, 0); err != nil {
			h.failPersistentVolumeRestore(backup, err)
			return
		}
		replicas[deployment.Name] = count
		if err := h.waitForDeploymentPods(environment.Namespace, deployment.Name, false, 90*time.Second); err != nil {
			h.failPersistentVolumeRestore(backup, err)
			return
		}
	}
	clear := "sudo -n find " + shellQuote(claim.LocalPath) + " -mindepth 1 -maxdepth 1 -exec rm -rf {} +"
	if out, err := sshExec(30*time.Second, append(buildSSHArgs(source, h.encKey, source.Host), clear)); err != nil {
		h.restoreBackupReplicas(environment.Namespace, replicas)
		h.failPersistentVolumeRestore(backup, fmt.Errorf("清空目标 PVC 失败: %s", strings.TrimSpace(string(out))))
		return
	}
	if _, err := h.streamPVCData(archive, source, backup.BackupPath, claim.LocalPath); err != nil {
		h.restoreBackupReplicas(environment.Namespace, replicas)
		h.failPersistentVolumeRestore(backup, err)
		return
	}
	h.restoreBackupReplicas(environment.Namespace, replicas)
	backup.RestoreStatus = "succeeded"
	backup.RestoreDetail = "PVC 数据已从备份恢复"
	completedAt := time.Now().UTC()
	backup.RestoreCompletedAt = &completedAt
	_ = h.store.UpdatePersistentVolumeBackup(backup)
}

func (h *K8sHandler) restoreBackupReplicas(namespace string, replicas map[string]int32) {
	for name, count := range replicas {
		_ = K8s.ScaleDeployment(namespace, name, count)
	}
}

func (h *K8sHandler) serverForK8sNode(nodeName string) (*model.Server, error) {
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

func (h *K8sHandler) failPersistentVolumeBackup(backup *model.PersistentVolumeBackup, err error) {
	backup.Status, backup.Detail = "failed", err.Error()
	completedAt := time.Now().UTC()
	backup.CompletedAt = &completedAt
	_ = h.store.UpdatePersistentVolumeBackup(backup)
}

func (h *K8sHandler) failPersistentVolumeRestore(backup *model.PersistentVolumeBackup, err error) {
	backup.RestoreStatus = "failed"
	backup.RestoreDetail = err.Error()
	completedAt := time.Now().UTC()
	backup.RestoreCompletedAt = &completedAt
	_ = h.store.UpdatePersistentVolumeBackup(backup)
}
