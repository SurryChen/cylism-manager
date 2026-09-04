package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	storageservice "github.com/cylism/cylism-manager/internal/service/storage"
	"github.com/cylism/cylism-manager/internal/transport"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
)

const hostDirectoryImportBackupRoot = "/data/cylism-import-backups"

type hostDirectoryPVCImportRequest struct {
	EnvironmentID      uint   `json:"environment_id"`
	SourceServerID     uint   `json:"source_server_id"`
	SourcePath         string `json:"source_path"`
	ConfirmDataReplace bool   `json:"confirm_data_replace"`
}

func (h *StorageHandler) ListHostDirectoryPVCImports(c *gin.Context) {
	environment, ok := h.pvcEnvironment(c)
	if !ok {
		return
	}
	tasks, err := h.Service.ListImports(environment.ID, c.Param("name"))
	if err != nil {
		apiShared.DBError(c, "读取目录导入记录失败")
		return
	}
	apiShared.Success(c, apiShared.HostDirectoryPVCImportsDTO(tasks))
}

func (h *StorageHandler) CreateHostDirectoryPVCImport(c *gin.Context) {
	if h.pvc == nil || h.migration == nil || h.store == nil {
		storageK8sUnavailable(c)
		return
	}
	var request hostDirectoryPVCImportRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.EnvironmentID == 0 || request.SourceServerID == 0 {
		apiShared.BadRequest(c, "环境、源服务器和源目录必填")
		return
	}
	sourcePath, valid := storageservice.ValidateHostDirectoryImportPath(request.SourcePath)
	if !valid {
		apiShared.ValidationError(c, "源目录必须是非系统目录的绝对路径")
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
	if claim.Phase != string(corev1.ClaimBound) || !claim.IsLocal || claim.BoundNode == "" || claim.LocalPath == "" {
		apiShared.ValidationError(c, "仅支持已绑定的 local-path 或 hostPath PVC 导入")
		return
	}
	if active, err := h.Service.FindActiveImport(environment.ID, claim.Name); err == nil {
		apiShared.ErrorWithData(c, http.StatusConflict, apiShared.CodeConflict, "该存储卷已有目录导入任务", apiShared.HostDirectoryPVCImportDTO(active))
		return
	} else if !storageErrorsIsNotFound(err) {
		apiShared.DBError(c, "检查目录导入状态失败")
		return
	}
	if active, err := h.Service.FindActiveMigration(environment.ID, claim.Name); err == nil {
		apiShared.ErrorWithData(c, http.StatusConflict, apiShared.CodeConflict, "该存储卷已有迁移任务，暂不能导入目录", apiShared.PersistentVolumeMigrationDTO(active))
		return
	} else if !storageErrorsIsNotFound(err) {
		apiShared.DBError(c, "检查存储卷迁移状态失败")
		return
	}
	source, err := h.store.GetServer(request.SourceServerID)
	if err != nil {
		apiShared.NotFound(c, "源服务器不存在")
		return
	}
	if source.SSHAuthType != "key" {
		apiShared.ValidationError(c, "源服务器必须使用 SSH 密钥认证")
		return
	}
	target, err := h.serverForK8sNode(claim.BoundNode)
	if err != nil || target.SSHAuthType != "key" {
		apiShared.ValidationError(c, "PVC 绑定节点必须关联使用 SSH 密钥认证的服务器")
		return
	}
	if source.ID == target.ID && storageservice.HostDirectoryImportPathsOverlap(sourcePath, claim.LocalPath) {
		apiShared.ValidationError(c, "源目录不能与目标 PVC 目录重叠")
		return
	}
	task := &model.HostDirectoryPVCImport{
		EnvironmentID:  environment.ID,
		PVCName:        claim.Name,
		SourceServerID: source.ID,
		SourceNodeName: source.K8sNodeName,
		SourcePath:     sourcePath,
		TargetNodeName: claim.BoundNode,
		TargetPath:     claim.LocalPath,
		Status:         model.PVCImportStatusPending,
		CreatedBy:      storageRequestUserID(c),
	}
	if err := h.Service.CreateImport(task); err != nil {
		apiShared.DBError(c, "创建目录导入记录失败")
		return
	}
	task.BackupPath = hostDirectoryImportBackupPath(claim.Name, task.ID, false)
	task.TargetBackupPath = hostDirectoryImportBackupPath(claim.Name, task.ID, true)
	if request.ConfirmDataReplace {
		task.Detail = "已确认可覆盖目标 PVC 数据"
	}
	if err := h.Service.UpdateImport(task, model.PVCImportStatusPending, task.Detail); err != nil {
		apiShared.DBError(c, "初始化目录导入记录失败")
		return
	}
	if err := h.Service.StartImport(c.Request.Context(), task.ID, request.ConfirmDataReplace); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	c.Status(http.StatusAccepted)
	apiShared.Success(c, apiShared.HostDirectoryPVCImportDTO(task))
}

func (h *StorageHandler) DeleteHostDirectoryPVCImportBackup(c *gin.Context) {
	if h.store == nil {
		apiShared.InternalError(c, "数据存储未初始化")
		return
	}
	environmentID, err := apiShared.OptionalID(strings.TrimSpace(c.Query("environment_id")))
	if err != nil {
		apiShared.BadRequest(c, "环境 ID 无效")
		return
	}
	if environmentID == 0 {
		var request struct {
			EnvironmentID uint `json:"environment_id"`
		}
		if err := c.ShouldBindJSON(&request); err != nil || request.EnvironmentID == 0 {
			apiShared.BadRequest(c, "环境 ID 无效")
			return
		}
		environmentID = request.EnvironmentID
	}
	environment, err := h.store.GetEnvironmentByID(environmentID)
	if err != nil {
		apiShared.NotFound(c, "环境不存在")
		return
	}
	id, err := apiShared.ParsePositiveID(strings.TrimSpace(c.Param("id")))
	if err != nil {
		apiShared.BadRequest(c, "导入任务 ID 无效")
		return
	}
	task, err := h.Service.GetImport(id)
	if err != nil || task.EnvironmentID != environment.ID || task.PVCName != c.Param("name") {
		apiShared.NotFound(c, "目录导入记录不存在")
		return
	}
	if task.Status != model.PVCImportStatusSucceeded || task.VerifiedAt == nil {
		apiShared.Conflict(c, "仅已完成校验的导入可以删除备份")
		return
	}
	if task.BackupDeletedAt != nil {
		apiShared.Success(c, apiShared.HostDirectoryPVCImportDTO(task))
		return
	}
	source, err := h.store.GetServer(task.SourceServerID)
	if err != nil {
		apiShared.NotFound(c, "源服务器不存在")
		return
	}
	target, err := h.serverForK8sNode(task.TargetNodeName)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.deleteHostDirectoryImportArchive(c.Request.Context(), source, task.BackupPath); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if task.TargetBackupChecksum != "" && target.ID != source.ID {
		if err := h.deleteHostDirectoryImportArchive(c.Request.Context(), target, task.TargetBackupPath); err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
	} else if task.TargetBackupChecksum != "" {
		if err := h.deleteHostDirectoryImportArchive(c.Request.Context(), source, task.TargetBackupPath); err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
	}
	now := time.Now().UTC()
	task.BackupDeletedAt = &now
	if err := h.Service.UpdateImport(task, task.Status, "已删除已验证的本地导入备份"); err != nil {
		apiShared.DBError(c, "更新备份删除状态失败")
		return
	}
	apiShared.Success(c, apiShared.HostDirectoryPVCImportDTO(task))
}

func (h *StorageHandler) runHostDirectoryPVCImport(parent context.Context, id uint, replaceTarget bool) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Hour)
	defer cancel()
	task, err := h.Service.GetImport(id)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	task.StartedAt = &now
	if err := h.Service.UpdateImport(task, model.PVCImportStatusPreflight, "正在检查源目录、目标 PVC 和工作负载"); err != nil {
		return
	}
	environment, source, target, claim, targetHasData, err := h.preflightHostDirectoryPVCImport(ctx, task, replaceTarget)
	if err != nil {
		h.failHostDirectoryPVCImport(ctx, task, err, nil)
		return
	}
	if err := h.Service.UpdateImport(task, model.PVCImportStatusStoppingWorkload, "正在停止引用 PVC 的工作负载"); err != nil {
		return
	}
	replicas, err := h.stopPVCImportWorkloads(ctx, environment.Namespace, claim.Name)
	if err != nil {
		h.failHostDirectoryPVCImport(ctx, task, err, nil)
		return
	}
	value, _ := json.Marshal(replicas)
	task.ApplicationReplicas = string(value)
	_ = h.Service.UpdateImport(task, model.PVCImportStatusBackingUp, "正在创建本地归档备份")
	backupChecksum, err := h.createHostDirectoryImportArchive(ctx, source, task.SourcePath, task.BackupPath)
	if err != nil {
		h.failHostDirectoryPVCImport(ctx, task, err, replicas)
		return
	}
	task.BackupChecksum = backupChecksum
	if targetHasData {
		targetChecksum, err := h.createHostDirectoryImportArchive(ctx, target, task.TargetPath, task.TargetBackupPath)
		if err != nil {
			h.failHostDirectoryPVCImport(ctx, task, err, replicas)
			return
		}
		task.TargetBackupChecksum = targetChecksum
	}
	if err := h.clearHostDirectoryImportTarget(ctx, target, task.TargetPath); err != nil {
		h.failHostDirectoryPVCImport(ctx, task, err, replicas)
		return
	}
	if err := h.Service.UpdateImport(task, model.PVCImportStatusCopying, "正在将宿主机目录复制到 PVC"); err != nil {
		return
	}
	bytesCopied, err := streamPVCData(ctx, source, target, task.SourcePath, task.TargetPath, h.encKey)
	task.BytesCopied = bytesCopied
	if err != nil {
		h.rollbackHostDirectoryImportTarget(ctx, task, target)
		h.failHostDirectoryPVCImport(ctx, task, err, replicas)
		return
	}
	if err := h.Service.UpdateImport(task, model.PVCImportStatusVerifying, "正在校验源目录与 PVC 内容"); err != nil {
		return
	}
	sourceChecksum, err := h.hostDirectoryContentChecksum(ctx, source, task.SourcePath)
	if err != nil {
		h.rollbackHostDirectoryImportTarget(ctx, task, target)
		h.failHostDirectoryPVCImport(ctx, task, err, replicas)
		return
	}
	targetChecksum, err := h.hostDirectoryContentChecksum(ctx, target, task.TargetPath)
	if err != nil || sourceChecksum != targetChecksum {
		if err == nil {
			err = fmt.Errorf("目录校验失败：源与 PVC 内容摘要不一致")
		}
		h.rollbackHostDirectoryImportTarget(ctx, task, target)
		h.failHostDirectoryPVCImport(ctx, task, err, replicas)
		return
	}
	task.SourceChecksum, task.TargetChecksum = sourceChecksum, targetChecksum
	verifiedAt := time.Now().UTC()
	task.VerifiedAt = &verifiedAt
	if err := h.Service.UpdateImport(task, model.PVCImportStatusRestoringWorkload, "数据校验完成，正在恢复工作负载"); err != nil {
		return
	}
	if err := h.restorePVCImportWorkloads(ctx, environment.Namespace, replicas); err != nil {
		h.failHostDirectoryPVCImport(ctx, task, err, replicas)
		return
	}
	_ = h.Service.UpdateImport(task, model.PVCImportStatusSucceeded, "目录已导入 PVC 并完成校验，备份可按需删除")
}

func (h *StorageHandler) preflightHostDirectoryPVCImport(ctx context.Context, task *model.HostDirectoryPVCImport, replaceTarget bool) (*model.Environment, *model.Server, *model.Server, *k8sclient.PersistentVolumeClaimInfo, bool, error) {
	environment, err := h.store.GetEnvironmentByID(task.EnvironmentID)
	if err != nil {
		return nil, nil, nil, nil, false, err
	}
	claim, err := h.pvc.GetManagedPVCContext(ctx, environment.Namespace, task.PVCName, environment.ID)
	if err != nil || claim.Phase != string(corev1.ClaimBound) || !claim.IsLocal || claim.BoundNode != task.TargetNodeName || claim.LocalPath != task.TargetPath {
		if err == nil {
			err = fmt.Errorf("目标 PVC 绑定状态已变化")
		}
		return nil, nil, nil, nil, false, err
	}
	source, err := h.store.GetServer(task.SourceServerID)
	if err != nil {
		return nil, nil, nil, nil, false, err
	}
	target, err := h.serverForK8sNode(task.TargetNodeName)
	if err != nil {
		return nil, nil, nil, nil, false, err
	}
	for _, server := range []*model.Server{source, target} {
		if server.SSHAuthType != "key" {
			return nil, nil, nil, nil, false, fmt.Errorf("服务器 %q 必须使用 SSH 密钥认证", server.Name)
		}
	}
	if out, err := transport.SSHExecContext(ctx, 20*time.Second, append(transport.BuildSSHArgs(source, h.encKey, source.Host), "sudo -n test -d "+storageShellQuote(task.SourcePath)+" && command -v tar >/dev/null && command -v sha256sum >/dev/null")); err != nil {
		return nil, nil, nil, nil, false, fmt.Errorf("源目录或工具预检失败: %s", strings.TrimSpace(string(out)))
	}
	command := "sudo -n test -d " + storageShellQuote(task.TargetPath) + " && command -v tar >/dev/null && command -v sha256sum >/dev/null && if sudo -n find " + storageShellQuote(task.TargetPath) + " -mindepth 1 -maxdepth 1 -print -quit | grep -q .; then echo nonempty; else echo empty; fi"
	out, err := transport.SSHExecContext(ctx, 20*time.Second, append(transport.BuildSSHArgs(target, h.encKey, target.Host), command))
	if err != nil {
		return nil, nil, nil, nil, false, fmt.Errorf("目标 PVC 或工具预检失败: %s", strings.TrimSpace(string(out)))
	}
	targetHasData := strings.TrimSpace(string(out)) == "nonempty"
	if targetHasData && !replaceTarget {
		return nil, nil, nil, nil, false, fmt.Errorf("目标 PVC 已有数据；请确认覆盖后再导入")
	}
	return environment, source, target, claim, targetHasData, nil
}

func (h *StorageHandler) stopPVCImportWorkloads(ctx context.Context, namespace, claimName string) (map[string]int32, error) {
	deployments, err := h.migration.DeploymentUsingPVC(ctx, namespace, claimName)
	if err != nil {
		return nil, err
	}
	replicas := make(map[string]int32, len(deployments))
	for _, deployment := range deployments {
		if deployment.Labels[k8sclient.ManagedByLabel] != k8sclient.ManagedByValue {
			return nil, fmt.Errorf("PVC 正被非平台托管的 Deployment %q 使用，拒绝自动导入", deployment.Name)
		}
	}
	for _, deployment := range deployments {
		count := int32(1)
		if deployment.Spec.Replicas != nil {
			count = *deployment.Spec.Replicas
		}
		if err := h.migration.ScaleDeployment(ctx, namespace, deployment.Name, 0); err != nil {
			return replicas, err
		}
		replicas[deployment.Name] = count
		if err := h.waitForStorageDeploymentPods(ctx, namespace, deployment.Name, false, 90*time.Second); err != nil {
			return replicas, err
		}
	}
	return replicas, nil
}

func (h *StorageHandler) restorePVCImportWorkloads(ctx context.Context, namespace string, replicas map[string]int32) error {
	for name, count := range replicas {
		if err := h.migration.ScaleDeployment(ctx, namespace, name, count); err != nil {
			return err
		}
		if count > 0 {
			if err := h.waitForStorageDeploymentPods(ctx, namespace, name, true, 2*time.Minute); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *StorageHandler) createHostDirectoryImportArchive(ctx context.Context, server *model.Server, sourcePath, archivePath string) (string, error) {
	command := "sudo -n mkdir -p " + storageShellQuote(path.Dir(archivePath)) + " && sudo -n rm -f " + storageShellQuote(archivePath) + " && sudo -n tar --numeric-owner -C " + storageShellQuote(sourcePath) + " -czf " + storageShellQuote(archivePath) + " . && sudo -n sha256sum " + storageShellQuote(archivePath) + " | awk '{print $1}'"
	out, err := transport.SSHExecContext(ctx, 30*time.Minute, append(transport.BuildSSHArgs(server, h.encKey, server.Host), command))
	if err != nil {
		return "", fmt.Errorf("创建本地备份失败: %s", strings.TrimSpace(string(out)))
	}
	checksum := hostDirectoryImportChecksumFromSSHOutput(string(out))
	if checksum == "" {
		return "", fmt.Errorf("创建本地备份失败：未得到有效校验摘要")
	}
	return checksum, nil
}

func (h *StorageHandler) hostDirectoryContentChecksum(ctx context.Context, server *model.Server, directory string) (string, error) {
	command := "LC_ALL=C sudo -n tar --sort=name --numeric-owner --mtime='UTC 1970-01-01' -C " + storageShellQuote(directory) + " -cf - . | sha256sum | awk '{print $1}'"
	out, err := transport.SSHExecContext(ctx, 30*time.Minute, append(transport.BuildSSHArgs(server, h.encKey, server.Host), command))
	if err != nil {
		return "", fmt.Errorf("计算目录校验摘要失败: %s", strings.TrimSpace(string(out)))
	}
	checksum := hostDirectoryImportChecksumFromSSHOutput(string(out))
	if checksum == "" {
		return "", fmt.Errorf("计算目录校验摘要失败：未得到有效摘要")
	}
	return checksum, nil
}

func (h *StorageHandler) clearHostDirectoryImportTarget(ctx context.Context, server *model.Server, targetPath string) error {
	command := "sudo -n find " + storageShellQuote(targetPath) + " -mindepth 1 -maxdepth 1 -exec rm -rf {} +"
	out, err := transport.SSHExecContext(ctx, 30*time.Second, append(transport.BuildSSHArgs(server, h.encKey, server.Host), command))
	if err != nil {
		return fmt.Errorf("清空目标 PVC 失败: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (h *StorageHandler) rollbackHostDirectoryImportTarget(ctx context.Context, task *model.HostDirectoryPVCImport, target *model.Server) {
	if task.TargetBackupChecksum == "" || task.TargetBackupPath == "" {
		return
	}
	command := "sudo -n find " + storageShellQuote(task.TargetPath) + " -mindepth 1 -maxdepth 1 -exec rm -rf {} + && sudo -n tar -xzf " + storageShellQuote(task.TargetBackupPath) + " -C " + storageShellQuote(task.TargetPath)
	_, _ = transport.SSHExecContext(ctx, 30*time.Minute, append(transport.BuildSSHArgs(target, h.encKey, target.Host), command))
}

func (h *StorageHandler) failHostDirectoryPVCImport(ctx context.Context, task *model.HostDirectoryPVCImport, cause error, replicas map[string]int32) {
	if len(replicas) > 0 && h.migration != nil {
		if environment, err := h.store.GetEnvironmentByID(task.EnvironmentID); err == nil {
			_ = h.restorePVCImportWorkloads(ctx, environment.Namespace, replicas)
		}
	}
	_ = h.Service.UpdateImport(task, model.PVCImportStatusFailed, cause.Error())
}

func (h *StorageHandler) deleteHostDirectoryImportArchive(ctx context.Context, server *model.Server, archivePath string) error {
	cleaned := path.Clean(archivePath)
	if !strings.HasPrefix(cleaned, hostDirectoryImportBackupRoot+"/") || !strings.HasSuffix(cleaned, ".tar.gz") {
		return fmt.Errorf("备份路径不属于平台管理目录")
	}
	out, err := transport.SSHExecContext(ctx, 30*time.Second, append(transport.BuildSSHArgs(server, h.encKey, server.Host), "sudo -n rm -f "+storageShellQuote(cleaned)))
	if err != nil {
		return fmt.Errorf("删除本地备份失败: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func safeHostDirectoryImportPath(value string) (string, bool) {
	return storageservice.ValidateHostDirectoryImportPath(value)
}

func hostDirectoryImportBackupPath(pvcName string, id uint, target bool) string {
	role := "source"
	if target {
		role = "target"
	}
	return path.Join(hostDirectoryImportBackupRoot, fmt.Sprintf("%s-import-%d-%s.tar.gz", pvcName, id, role))
}

func hostDirectoryImportPathsOverlap(left, right string) bool {
	return storageservice.HostDirectoryImportPathsOverlap(left, right)
}

// hostDirectoryImportChecksumFromSSHOutput extracts the final valid digest
// while ignoring SSH host-key warnings mixed into command output.
func hostDirectoryImportChecksumFromSSHOutput(output string) string {
	fields := strings.Fields(output)
	for index := len(fields) - 1; index >= 0; index-- {
		candidate := fields[index]
		if len(candidate) != 64 {
			continue
		}
		valid := true
		for _, char := range candidate {
			if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
				valid = false
				break
			}
		}
		if valid {
			return candidate
		}
	}
	return ""
}

func SafeHostDirectoryImportPath(value string) (string, bool) {
	return safeHostDirectoryImportPath(value)
}
func HostDirectoryImportBackupPath(pvcName string, id uint, target bool) string {
	return hostDirectoryImportBackupPath(pvcName, id, target)
}
func HostDirectoryImportPathsOverlap(left, right string) bool {
	return hostDirectoryImportPathsOverlap(left, right)
}
func HostDirectoryImportChecksumFromSSHOutput(output string) string {
	return hostDirectoryImportChecksumFromSSHOutput(output)
}
func StreamCommandDiagnostic(output string) string { return streamCommandDiagnostic(output) }
