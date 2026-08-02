package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
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

func (h *K8sHandler) ListHostDirectoryPVCImports(c *gin.Context) {
	environment, ok := h.pvcEnvironment(c)
	if !ok {
		return
	}
	tasks, err := h.store.ListHostDirectoryPVCImports(environment.ID, c.Param("name"))
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取目录导入记录失败")
		return
	}
	model.Success(c, tasks)
}

func (h *K8sHandler) CreateHostDirectoryPVCImport(c *gin.Context) {
	if K8s == nil || h.store == nil {
		k8sUnavailable(c)
		return
	}
	var request hostDirectoryPVCImportRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.EnvironmentID == 0 || request.SourceServerID == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "环境、源服务器和源目录必填")
		return
	}
	sourcePath, valid := safeHostDirectoryImportPath(request.SourcePath)
	if !valid {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "源目录必须是非系统目录的绝对路径")
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
	if claim.Phase != string(corev1.ClaimBound) || !claim.IsLocal || claim.BoundNode == "" || claim.LocalPath == "" {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "仅支持已绑定的 local-path 或 hostPath PVC 导入")
		return
	}
	if active, err := h.store.FindActiveHostDirectoryPVCImport(environment.ID, claim.Name); err == nil {
		model.ErrorWithData(c, http.StatusConflict, model.CodeConflict, "该存储卷已有目录导入任务", active)
		return
	} else if !errorsIsNotFound(err) {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "检查目录导入状态失败")
		return
	}
	if active, err := h.store.FindActivePVCMigration(environment.ID, claim.Name); err == nil {
		model.ErrorWithData(c, http.StatusConflict, model.CodeConflict, "该存储卷已有迁移任务，暂不能导入目录", active)
		return
	} else if !errorsIsNotFound(err) {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "检查存储卷迁移状态失败")
		return
	}
	source, err := h.store.GetServer(request.SourceServerID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "源服务器不存在")
		return
	}
	if source.SSHAuthType != "key" {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "源服务器必须使用 SSH 密钥认证")
		return
	}
	target, err := h.serverForK8sNode(claim.BoundNode)
	if err != nil || target.SSHAuthType != "key" {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "PVC 绑定节点必须关联使用 SSH 密钥认证的服务器")
		return
	}
	if source.ID == target.ID && hostDirectoryImportPathsOverlap(sourcePath, claim.LocalPath) {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "源目录不能与目标 PVC 目录重叠")
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
		CreatedBy:      getUserID(c),
	}
	if err := h.store.CreateHostDirectoryPVCImport(task); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "创建目录导入记录失败")
		return
	}
	task.BackupPath = hostDirectoryImportBackupPath(claim.Name, task.ID, false)
	task.TargetBackupPath = hostDirectoryImportBackupPath(claim.Name, task.ID, true)
	if request.ConfirmDataReplace {
		task.Detail = "已确认可覆盖目标 PVC 数据"
	}
	if err := h.store.UpdateHostDirectoryPVCImport(task, model.PVCImportStatusPending, task.Detail); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "初始化目录导入记录失败")
		return
	}
	go h.runHostDirectoryPVCImport(task.ID, request.ConfirmDataReplace)
	c.Status(http.StatusAccepted)
	model.Success(c, task)
}

func (h *K8sHandler) DeleteHostDirectoryPVCImportBackup(c *gin.Context) {
	if h.store == nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "数据存储未初始化")
		return
	}
	environment, ok := h.pvcEnvironment(c)
	if !ok {
		return
	}
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "导入任务 ID 无效")
		return
	}
	task, err := h.store.GetHostDirectoryPVCImport(id)
	if err != nil || task.EnvironmentID != environment.ID || task.PVCName != c.Param("name") {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "目录导入记录不存在")
		return
	}
	if task.Status != model.PVCImportStatusSucceeded || task.VerifiedAt == nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "仅已完成校验的导入可以删除备份")
		return
	}
	if task.BackupDeletedAt != nil {
		model.Success(c, task)
		return
	}
	source, err := h.store.GetServer(task.SourceServerID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "源服务器不存在")
		return
	}
	target, err := h.serverForK8sNode(task.TargetNodeName)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.deleteHostDirectoryImportArchive(source, task.BackupPath); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if task.TargetBackupChecksum != "" && target.ID != source.ID {
		if err := h.deleteHostDirectoryImportArchive(target, task.TargetBackupPath); err != nil {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
			return
		}
	} else if task.TargetBackupChecksum != "" {
		if err := h.deleteHostDirectoryImportArchive(source, task.TargetBackupPath); err != nil {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
			return
		}
	}
	now := time.Now().UTC()
	task.BackupDeletedAt = &now
	if err := h.store.UpdateHostDirectoryPVCImport(task, task.Status, "已删除已验证的本地导入备份"); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "更新备份删除状态失败")
		return
	}
	model.Success(c, task)
}

func (h *K8sHandler) runHostDirectoryPVCImport(id uint, replaceTarget bool) {
	task, err := h.store.GetHostDirectoryPVCImport(id)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	task.StartedAt = &now
	if err := h.store.UpdateHostDirectoryPVCImport(task, model.PVCImportStatusPreflight, "正在检查源目录、目标 PVC 和工作负载"); err != nil {
		return
	}
	environment, source, target, claim, targetHasData, err := h.preflightHostDirectoryPVCImport(task, replaceTarget)
	if err != nil {
		h.failHostDirectoryPVCImport(task, err, nil)
		return
	}
	if err := h.store.UpdateHostDirectoryPVCImport(task, model.PVCImportStatusStoppingWorkload, "正在停止引用 PVC 的工作负载"); err != nil {
		return
	}
	replicas, err := h.stopPVCImportWorkloads(environment.Namespace, claim.Name)
	if err != nil {
		h.failHostDirectoryPVCImport(task, err, nil)
		return
	}
	value, _ := json.Marshal(replicas)
	task.ApplicationReplicas = string(value)
	_ = h.store.UpdateHostDirectoryPVCImport(task, model.PVCImportStatusBackingUp, "正在创建本地归档备份")
	backupChecksum, err := h.createHostDirectoryImportArchive(source, task.SourcePath, task.BackupPath)
	if err != nil {
		h.failHostDirectoryPVCImport(task, err, replicas)
		return
	}
	task.BackupChecksum = backupChecksum
	if targetHasData {
		targetChecksum, err := h.createHostDirectoryImportArchive(target, task.TargetPath, task.TargetBackupPath)
		if err != nil {
			h.failHostDirectoryPVCImport(task, err, replicas)
			return
		}
		task.TargetBackupChecksum = targetChecksum
	}
	if err := h.clearHostDirectoryImportTarget(target, task.TargetPath); err != nil {
		h.failHostDirectoryPVCImport(task, err, replicas)
		return
	}
	if err := h.store.UpdateHostDirectoryPVCImport(task, model.PVCImportStatusCopying, "正在将宿主机目录复制到 PVC"); err != nil {
		return
	}
	bytesCopied, err := h.streamPVCData(source, target, task.SourcePath, task.TargetPath)
	task.BytesCopied = bytesCopied
	if err != nil {
		h.rollbackHostDirectoryImportTarget(task, target)
		h.failHostDirectoryPVCImport(task, err, replicas)
		return
	}
	if err := h.store.UpdateHostDirectoryPVCImport(task, model.PVCImportStatusVerifying, "正在校验源目录与 PVC 内容"); err != nil {
		return
	}
	sourceChecksum, err := h.hostDirectoryContentChecksum(source, task.SourcePath)
	if err != nil {
		h.rollbackHostDirectoryImportTarget(task, target)
		h.failHostDirectoryPVCImport(task, err, replicas)
		return
	}
	targetChecksum, err := h.hostDirectoryContentChecksum(target, task.TargetPath)
	if err != nil || sourceChecksum != targetChecksum {
		if err == nil {
			err = fmt.Errorf("目录校验失败：源与 PVC 内容摘要不一致")
		}
		h.rollbackHostDirectoryImportTarget(task, target)
		h.failHostDirectoryPVCImport(task, err, replicas)
		return
	}
	task.SourceChecksum, task.TargetChecksum = sourceChecksum, targetChecksum
	verifiedAt := time.Now().UTC()
	task.VerifiedAt = &verifiedAt
	if err := h.store.UpdateHostDirectoryPVCImport(task, model.PVCImportStatusRestoringWorkload, "数据校验完成，正在恢复工作负载"); err != nil {
		return
	}
	if err := h.restorePVCImportWorkloads(environment.Namespace, replicas); err != nil {
		h.failHostDirectoryPVCImport(task, err, replicas)
		return
	}
	_ = h.store.UpdateHostDirectoryPVCImport(task, model.PVCImportStatusSucceeded, "目录已导入 PVC 并完成校验，备份可按需删除")
}

func (h *K8sHandler) preflightHostDirectoryPVCImport(task *model.HostDirectoryPVCImport, replaceTarget bool) (*model.Environment, *model.Server, *model.Server, *k8sclient.PersistentVolumeClaimInfo, bool, error) {
	environment, err := h.store.GetEnvironmentByID(task.EnvironmentID)
	if err != nil {
		return nil, nil, nil, nil, false, err
	}
	claim, err := K8s.GetManagedPVC(environment.Namespace, task.PVCName, environment.ID)
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
	if out, err := sshExec(20*time.Second, append(buildSSHArgs(source, h.encKey, source.Host), "sudo -n test -d "+shellQuote(task.SourcePath)+" && command -v tar >/dev/null && command -v sha256sum >/dev/null")); err != nil {
		return nil, nil, nil, nil, false, fmt.Errorf("源目录或工具预检失败: %s", strings.TrimSpace(string(out)))
	}
	command := "sudo -n test -d " + shellQuote(task.TargetPath) + " && command -v tar >/dev/null && command -v sha256sum >/dev/null && if sudo -n find " + shellQuote(task.TargetPath) + " -mindepth 1 -maxdepth 1 -print -quit | grep -q .; then echo nonempty; else echo empty; fi"
	out, err := sshExec(20*time.Second, append(buildSSHArgs(target, h.encKey, target.Host), command))
	if err != nil {
		return nil, nil, nil, nil, false, fmt.Errorf("目标 PVC 或工具预检失败: %s", strings.TrimSpace(string(out)))
	}
	targetHasData := strings.TrimSpace(string(out)) == "nonempty"
	if targetHasData && !replaceTarget {
		return nil, nil, nil, nil, false, fmt.Errorf("目标 PVC 已有数据；请确认覆盖后再导入")
	}
	return environment, source, target, claim, targetHasData, nil
}

func (h *K8sHandler) stopPVCImportWorkloads(namespace, claimName string) (map[string]int32, error) {
	deployments, err := K8s.DeploymentUsingPVC(namespace, claimName)
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
		if err := K8s.ScaleDeployment(namespace, deployment.Name, 0); err != nil {
			return replicas, err
		}
		replicas[deployment.Name] = count
		if err := h.waitForDeploymentPods(namespace, deployment.Name, false, 90*time.Second); err != nil {
			return replicas, err
		}
	}
	return replicas, nil
}

func (h *K8sHandler) restorePVCImportWorkloads(namespace string, replicas map[string]int32) error {
	for name, count := range replicas {
		if err := K8s.ScaleDeployment(namespace, name, count); err != nil {
			return err
		}
		if count > 0 {
			if err := h.waitForDeploymentPods(namespace, name, true, 2*time.Minute); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *K8sHandler) createHostDirectoryImportArchive(server *model.Server, sourcePath, archivePath string) (string, error) {
	command := "sudo -n mkdir -p " + shellQuote(path.Dir(archivePath)) + " && sudo -n rm -f " + shellQuote(archivePath) + " && sudo -n tar --numeric-owner -C " + shellQuote(sourcePath) + " -czf " + shellQuote(archivePath) + " . && sudo -n sha256sum " + shellQuote(archivePath) + " | awk '{print $1}'"
	out, err := sshExec(30*time.Minute, append(buildSSHArgs(server, h.encKey, server.Host), command))
	if err != nil {
		return "", fmt.Errorf("创建本地备份失败: %s", strings.TrimSpace(string(out)))
	}
	checksum := strings.TrimSpace(string(out))
	if len(checksum) != 64 {
		return "", fmt.Errorf("创建本地备份失败：未得到有效校验摘要")
	}
	return checksum, nil
}

func (h *K8sHandler) hostDirectoryContentChecksum(server *model.Server, directory string) (string, error) {
	command := "LC_ALL=C sudo -n tar --sort=name --numeric-owner --mtime='UTC 1970-01-01' -C " + shellQuote(directory) + " -cf - . | sha256sum | awk '{print $1}'"
	out, err := sshExec(30*time.Minute, append(buildSSHArgs(server, h.encKey, server.Host), command))
	if err != nil {
		return "", fmt.Errorf("计算目录校验摘要失败: %s", strings.TrimSpace(string(out)))
	}
	checksum := strings.TrimSpace(string(out))
	if len(checksum) != 64 {
		return "", fmt.Errorf("计算目录校验摘要失败：未得到有效摘要")
	}
	return checksum, nil
}

func (h *K8sHandler) clearHostDirectoryImportTarget(server *model.Server, targetPath string) error {
	command := "sudo -n find " + shellQuote(targetPath) + " -mindepth 1 -maxdepth 1 -exec rm -rf {} +"
	out, err := sshExec(30*time.Second, append(buildSSHArgs(server, h.encKey, server.Host), command))
	if err != nil {
		return fmt.Errorf("清空目标 PVC 失败: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (h *K8sHandler) rollbackHostDirectoryImportTarget(task *model.HostDirectoryPVCImport, target *model.Server) {
	if task.TargetBackupChecksum == "" || task.TargetBackupPath == "" {
		return
	}
	command := "sudo -n find " + shellQuote(task.TargetPath) + " -mindepth 1 -maxdepth 1 -exec rm -rf {} + && sudo -n tar -xzf " + shellQuote(task.TargetBackupPath) + " -C " + shellQuote(task.TargetPath)
	_, _ = sshExec(30*time.Minute, append(buildSSHArgs(target, h.encKey, target.Host), command))
}

func (h *K8sHandler) failHostDirectoryPVCImport(task *model.HostDirectoryPVCImport, cause error, replicas map[string]int32) {
	if len(replicas) > 0 && K8s != nil {
		if environment, err := h.store.GetEnvironmentByID(task.EnvironmentID); err == nil {
			_ = h.restorePVCImportWorkloads(environment.Namespace, replicas)
		}
	}
	_ = h.store.UpdateHostDirectoryPVCImport(task, model.PVCImportStatusFailed, cause.Error())
}

func (h *K8sHandler) deleteHostDirectoryImportArchive(server *model.Server, archivePath string) error {
	cleaned := path.Clean(archivePath)
	if !strings.HasPrefix(cleaned, hostDirectoryImportBackupRoot+"/") || !strings.HasSuffix(cleaned, ".tar.gz") {
		return fmt.Errorf("备份路径不属于平台管理目录")
	}
	out, err := sshExec(30*time.Second, append(buildSSHArgs(server, h.encKey, server.Host), "sudo -n rm -f "+shellQuote(cleaned)))
	if err != nil {
		return fmt.Errorf("删除本地备份失败: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func safeHostDirectoryImportPath(value string) (string, bool) {
	cleaned := path.Clean(strings.TrimSpace(value))
	if !strings.HasPrefix(cleaned, "/") || cleaned == "/" {
		return "", false
	}
	for _, protected := range []string{"/boot", "/dev", "/etc", "/proc", "/run", "/sys", "/usr", "/bin", "/sbin", "/lib", "/lib64", "/var/lib/rancher/k3s", "/var/lib/kubelet"} {
		if cleaned == protected || strings.HasPrefix(cleaned, protected+"/") {
			return "", false
		}
	}
	return cleaned, true
}

func hostDirectoryImportBackupPath(pvcName string, id uint, target bool) string {
	role := "source"
	if target {
		role = "target"
	}
	return path.Join(hostDirectoryImportBackupRoot, fmt.Sprintf("%s-import-%d-%s.tar.gz", pvcName, id, role))
}

func hostDirectoryImportPathsOverlap(left, right string) bool {
	left, right = path.Clean(left), path.Clean(right)
	return left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}
