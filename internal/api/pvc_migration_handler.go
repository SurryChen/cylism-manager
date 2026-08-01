package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cylism/cylism-manager/internal/application"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const defaultPVCMigrationHelperImage = "registry.k8s.io/pause:3.10"

type pvcMigrationRequest struct {
	EnvironmentID uint   `json:"environment_id"`
	TargetNode    string `json:"target_node_name"`
	HelperImage   string `json:"helper_image"`
}

func (h *K8sHandler) ListPersistentVolumeMigrations(c *gin.Context) {
	if h.store == nil {
		model.Error(c, 500, model.CodeInternalError, "数据存储未初始化")
		return
	}
	environment, ok := h.pvcEnvironment(c)
	if !ok {
		return
	}
	migrations, err := h.store.ListPersistentVolumeMigrations(environment.ID)
	if err != nil {
		model.Error(c, 500, model.CodeDBError, "读取存储卷迁移失败")
		return
	}
	model.Success(c, migrations)
}

func (h *K8sHandler) GetPersistentVolumeMigration(c *gin.Context) {
	if h.store == nil {
		model.Error(c, 500, model.CodeInternalError, "数据存储未初始化")
		return
	}
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, 400, model.CodeBadRequest, "迁移 ID 无效")
		return
	}
	migration, err := h.store.GetPersistentVolumeMigration(id)
	if err != nil {
		model.Error(c, 404, model.CodeNotFound, "迁移任务不存在")
		return
	}
	model.Success(c, migration)
}

func (h *K8sHandler) CreatePersistentVolumeMigration(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	if h.store == nil {
		model.Error(c, 500, model.CodeInternalError, "数据存储未初始化")
		return
	}
	var request pvcMigrationRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.EnvironmentID == 0 || strings.TrimSpace(request.TargetNode) == "" {
		model.Error(c, 400, model.CodeBadRequest, "环境和目标节点必填")
		return
	}
	environment, err := h.store.GetEnvironmentByID(request.EnvironmentID)
	if err != nil {
		model.Error(c, 404, model.CodeNotFound, "环境不存在")
		return
	}
	sourceName := c.Param("name")
	if active, err := h.store.FindActivePVCMigration(environment.ID, sourceName); err == nil {
		model.ErrorWithData(c, 409, model.CodeConflict, "该存储卷已有迁移任务", active)
		return
	} else if !errorsIsNotFound(err) {
		model.Error(c, 500, model.CodeDBError, "检查存储卷迁移状态失败")
		return
	}
	claim, err := K8s.GetManagedPVC(environment.Namespace, sourceName, environment.ID)
	if err != nil {
		model.Error(c, 400, model.CodeValidationFail, err.Error())
		return
	}
	if claim.Phase != string(corev1.ClaimBound) || !claim.IsLocal || claim.BoundNode == "" {
		model.Error(c, 400, model.CodeValidationFail, "仅支持迁移已绑定的 local-path/hostPath 存储卷")
		return
	}
	if claim.BoundNode == request.TargetNode {
		model.Error(c, 400, model.CodeValidationFail, "目标节点与当前绑定节点相同")
		return
	}
	deployment, app, replicas, err := h.migrationWorkload(environment.ID, environment.Namespace, sourceName)
	if err != nil {
		model.Error(c, 409, model.CodeConflict, err.Error())
		return
	}
	templateSnapshot, err := h.migrationTemplateSnapshot(app.ID)
	if err != nil {
		model.Error(c, 500, model.CodeDBError, "保存迁移前模板快照失败")
		return
	}
	migration := &model.PersistentVolumeMigration{EnvironmentID: environment.ID, ApplicationID: app.ID, SourcePVCName: sourceName, SourceNodeName: claim.BoundNode, TargetNodeName: request.TargetNode, SourceDeployment: deployment, SourceReplicas: replicas, SourceTemplateSpec: templateSnapshot, Status: model.PVCMigrationStatusPending}
	if err := h.store.CreatePersistentVolumeMigration(migration); err != nil {
		model.Error(c, 500, model.CodeDBError, "创建存储卷迁移失败")
		return
	}
	migration.TargetPVCName = migrationPVCName(sourceName, migration.ID)
	if err := h.store.UpdatePersistentVolumeMigration(migration, model.PVCMigrationStatusPending, "等待预检"); err != nil {
		model.Error(c, 500, model.CodeDBError, "初始化迁移任务失败")
		return
	}
	helperImage := strings.TrimSpace(request.HelperImage)
	if helperImage == "" {
		helperImage = defaultPVCMigrationHelperImage
	}
	go h.runPersistentVolumeMigration(migration.ID, helperImage)
	c.Status(202)
	model.Success(c, migration)
}

func (h *K8sHandler) CleanupPersistentVolumeMigration(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, 400, model.CodeBadRequest, "迁移 ID 无效")
		return
	}
	migration, err := h.store.GetPersistentVolumeMigration(id)
	if err != nil || migration.Status != model.PVCMigrationStatusCleanupPending {
		model.Error(c, 409, model.CodeConflict, "迁移任务当前不能清理源卷")
		return
	}
	environment, err := h.store.GetEnvironmentByID(migration.EnvironmentID)
	if err != nil {
		model.Error(c, 404, model.CodeNotFound, "环境不存在")
		return
	}
	if deployments, err := K8s.DeploymentUsingPVC(environment.Namespace, migration.SourcePVCName); err != nil || len(deployments) != 0 {
		model.Error(c, 409, model.CodeConflict, "源存储卷仍被工作负载引用，不能清理")
		return
	}
	if err := K8s.DeleteManagedPVC(environment.Namespace, migration.SourcePVCName, environment.ID); err != nil {
		model.Error(c, 400, model.CodeK8sAPIError, err.Error())
		return
	}
	_ = h.store.UpdatePersistentVolumeMigration(migration, model.PVCMigrationStatusCleaned, "源卷已按回收策略清理")
	model.Success(c, migration)
}

func (h *K8sHandler) migrationWorkload(environmentID uint, namespace, claimName string) (string, *model.Application, int32, error) {
	deployments, err := K8s.DeploymentUsingPVC(namespace, claimName)
	if err != nil {
		return "", nil, 0, err
	}
	if len(deployments) != 1 {
		return "", nil, 0, fmt.Errorf("存储卷必须恰好被一个平台 Deployment 引用")
	}
	deployment := deployments[0]
	if deployment.Labels[k8sclient.ManagedByLabel] != k8sclient.ManagedByValue {
		return "", nil, 0, fmt.Errorf("引用存储卷的 Deployment 未由平台管理")
	}
	applications, err := h.store.ListApplications(0, environmentID)
	if err != nil {
		return "", nil, 0, err
	}
	for index := range applications {
		if applications[index].Name == deployment.Name {
			replicas := int32(1)
			if deployment.Spec.Replicas != nil {
				replicas = *deployment.Spec.Replicas
			}
			return deployment.Name, &applications[index], replicas, nil
		}
	}
	return "", nil, 0, fmt.Errorf("找不到引用存储卷的平台应用")
}

func (h *K8sHandler) runPersistentVolumeMigration(id uint, helperImage string) {
	migration, err := h.store.GetPersistentVolumeMigration(id)
	if err != nil {
		return
	}
	environment, err := h.store.GetEnvironmentByID(migration.EnvironmentID)
	if err != nil {
		h.failMigration(migration, err, false)
		return
	}
	if err := h.store.UpdatePersistentVolumeMigration(migration, model.PVCMigrationStatusPreflight, "正在检查源节点、目标节点和工作负载"); err != nil {
		return
	}
	source, target, claim, err := h.preflightMigration(environment, migration)
	if err != nil {
		h.failMigration(migration, err, false)
		return
	}
	if err := h.store.UpdatePersistentVolumeMigration(migration, model.PVCMigrationStatusProvisioningTarget, "正在预配目标本地卷"); err != nil {
		return
	}
	if _, err := K8s.CreateManagedPVC(environment.Namespace, environment.ID, k8sclient.PersistentVolumeClaimRequest{Name: migration.TargetPVCName, Storage: claim.Storage, StorageClassName: claim.StorageClassName}); err != nil {
		h.failMigration(migration, err, false)
		return
	}
	if _, err := K8s.CreatePVCBindingPod(environment.Namespace, strconv.FormatUint(uint64(migration.ID), 10), migration.TargetPVCName, migration.TargetNodeName, helperImage); err != nil {
		h.cleanupMigrationTarget(environment, migration)
		h.failMigration(migration, err, false)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	targetClaim, err := K8s.WaitForManagedPVCBound(ctx, environment.Namespace, migration.TargetPVCName, environment.ID)
	cancel()
	if err != nil || !targetClaim.IsLocal || targetClaim.BoundNode != migration.TargetNodeName {
		h.cleanupMigrationTarget(environment, migration)
		if err == nil {
			err = fmt.Errorf("目标 PVC 未绑定到所选节点")
		}
		h.failMigration(migration, err, false)
		return
	}
	if err := h.checkMigrationTargetPath(target, targetClaim.LocalPath); err != nil {
		h.cleanupMigrationTarget(environment, migration)
		h.failMigration(migration, err, false)
		return
	}
	if err := h.store.UpdatePersistentVolumeMigration(migration, model.PVCMigrationStatusStoppingSource, "正在停止源工作负载"); err != nil {
		return
	}
	if err := K8s.ScaleDeployment(environment.Namespace, migration.SourceDeployment, 0); err != nil {
		h.cleanupMigrationTarget(environment, migration)
		h.failMigration(migration, err, false)
		return
	}
	if err := h.waitForDeploymentPods(environment.Namespace, migration.SourceDeployment, false, 90*time.Second); err != nil {
		h.cleanupMigrationTarget(environment, migration)
		h.failMigration(migration, err, true)
		return
	}
	if err := h.store.UpdatePersistentVolumeMigration(migration, model.PVCMigrationStatusCopying, "正在复制本地卷数据"); err != nil {
		return
	}
	bytesCopied, err := h.streamPVCData(source, target, claim.LocalPath, targetClaim.LocalPath)
	migration.BytesCopied = bytesCopied
	if err != nil {
		h.cleanupMigrationTarget(environment, migration)
		h.failMigration(migration, err, true)
		return
	}
	if err := h.store.UpdatePersistentVolumeMigration(migration, model.PVCMigrationStatusCutover, "正在切换应用到目标存储卷"); err != nil {
		return
	}
	if err := K8s.DeletePVCBindingPod(environment.Namespace, strconv.FormatUint(uint64(migration.ID), 10)); err != nil {
		h.failMigration(migration, err, true)
		return
	}
	if _, err := K8s.ReplaceDeploymentPVCNode(environment.Namespace, migration.SourceDeployment, migration.SourcePVCName, migration.TargetPVCName, migration.TargetNodeName); err != nil {
		h.failMigration(migration, err, true)
		return
	}
	if err := h.replaceMigrationTemplateClaims(migration.ApplicationID, migration.SourcePVCName, migration.TargetPVCName, migration.TargetNodeName); err != nil {
		h.failMigration(migration, err, true)
		return
	}
	if err := K8s.ScaleDeployment(environment.Namespace, migration.SourceDeployment, migration.SourceReplicas); err != nil {
		h.failMigration(migration, err, true)
		return
	}
	if err := h.store.UpdatePersistentVolumeMigration(migration, model.PVCMigrationStatusWaitingReady, "正在等待目标工作负载就绪"); err != nil {
		return
	}
	if err := h.waitForDeploymentPods(environment.Namespace, migration.SourceDeployment, true, 2*time.Minute); err != nil {
		h.failMigration(migration, err, true)
		return
	}
	_ = h.store.UpdatePersistentVolumeMigration(migration, model.PVCMigrationStatusCleanupPending, "目标工作负载已就绪，可确认清理源卷")
}

func (h *K8sHandler) preflightMigration(environment *model.Environment, migration *model.PersistentVolumeMigration) (*model.Server, *model.Server, *k8sclient.PersistentVolumeClaimInfo, error) {
	claim, err := K8s.GetManagedPVC(environment.Namespace, migration.SourcePVCName, environment.ID)
	if err != nil || !claim.IsLocal || claim.LocalPath == "" || claim.BoundNode != migration.SourceNodeName {
		if err == nil {
			err = fmt.Errorf("源 PVC 状态已变化，无法迁移")
		}
		return nil, nil, nil, err
	}
	node, err := K8s.Clientset.CoreV1().Nodes().Get(K8s.Ctx(), migration.TargetNodeName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("目标节点不可用: %w", err)
	}
	if !nodeReady(node) {
		return nil, nil, nil, fmt.Errorf("目标节点未就绪")
	}
	servers, err := h.store.ListServers()
	if err != nil {
		return nil, nil, nil, err
	}
	var source, target *model.Server
	for index := range servers {
		if servers[index].K8sNodeName == migration.SourceNodeName {
			source = &servers[index]
		}
		if servers[index].K8sNodeName == migration.TargetNodeName {
			target = &servers[index]
		}
	}
	if source == nil || target == nil || source.SSHAuthType != "key" || target.SSHAuthType != "key" {
		return nil, nil, nil, fmt.Errorf("源节点和目标节点必须绑定使用 SSH 密钥认证的平台注册服务器")
	}
	for _, server := range []*model.Server{source, target} {
		out, err := sshExec(20*time.Second, append(buildSSHArgs(server, h.encKey, server.Host), "sudo -n true && command -v tar >/dev/null"))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("服务器 %q 的 sudo 或 tar 预检失败: %s", server.Name, strings.TrimSpace(string(out)))
		}
	}
	return source, target, claim, nil
}

func (h *K8sHandler) checkMigrationTargetPath(server *model.Server, path string) error {
	command := "sudo -n test -d " + shellQuote(path) + " && df -Pk " + shellQuote(path) + " >/dev/null"
	out, err := sshExec(20*time.Second, append(buildSSHArgs(server, h.encKey, server.Host), command))
	if err != nil {
		return fmt.Errorf("目标节点目录或磁盘预检失败: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (h *K8sHandler) streamPVCData(source, target *model.Server, sourcePath, targetPath string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	sourceArgs := append(buildSSHArgs(source, h.encKey, source.Host), "sudo -n tar --numeric-owner -C "+shellQuote(sourcePath)+" -cpf - .")
	targetArgs := append(buildSSHArgs(target, h.encKey, target.Host), "sudo -n mkdir -p "+shellQuote(targetPath)+" && sudo -n tar --numeric-owner -C "+shellQuote(targetPath)+" -xpf -")
	sourceCommand := exec.CommandContext(ctx, "ssh", sourceArgs...)
	targetCommand := exec.CommandContext(ctx, "ssh", targetArgs...)
	var sourceErr, targetErr bytes.Buffer
	sourceCommand.Stderr = &sourceErr
	targetCommand.Stderr = &targetErr
	reader, err := sourceCommand.StdoutPipe()
	if err != nil {
		return 0, err
	}
	var copied atomic.Int64
	targetCommand.Stdin = io.TeeReader(reader, countWriter{count: &copied})
	if err := targetCommand.Start(); err != nil {
		return 0, err
	}
	if err := sourceCommand.Start(); err != nil {
		_ = targetCommand.Process.Kill()
		return 0, err
	}
	sourceRunErr := sourceCommand.Wait()
	targetRunErr := targetCommand.Wait()
	if sourceRunErr != nil || targetRunErr != nil {
		return copied.Load(), fmt.Errorf("存储卷数据复制失败: 源=%s 目标=%s", strings.TrimSpace(sourceErr.String()), strings.TrimSpace(targetErr.String()))
	}
	return copied.Load(), nil
}

type countWriter struct{ count *atomic.Int64 }

func (w countWriter) Write(value []byte) (int, error) {
	w.count.Add(int64(len(value)))
	return len(value), nil
}

func (h *K8sHandler) waitForDeploymentPods(namespace, name string, ready bool, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		deployment, err := K8s.Clientset.AppsV1().Deployments(namespace).Get(K8s.Ctx(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if ready {
			if deployment.Status.AvailableReplicas >= 1 {
				return nil
			}
		} else {
			pods, err := K8s.ListDeploymentPods(namespace, name)
			if err != nil {
				return err
			}
			if len(pods) == 0 {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	if ready {
		return fmt.Errorf("等待目标工作负载就绪超时")
	}
	return fmt.Errorf("等待源工作负载停止超时")
}

func (h *K8sHandler) replaceMigrationTemplateClaims(applicationID uint, sourceClaim, targetClaim, targetNode string) error {
	templates, err := h.store.ListApplicationDeploymentTemplates(applicationID)
	if err != nil {
		return err
	}
	for index := range templates {
		var spec application.ReleaseSpec
		if err := json.Unmarshal([]byte(templates[index].Spec), &spec); err != nil {
			return err
		}
		changed := false
		for volume := range spec.Volumes {
			if spec.Volumes[volume].ClaimName == sourceClaim {
				spec.Volumes[volume].ClaimName = targetClaim
				changed = true
			}
		}
		if !changed {
			continue
		}
		spec.NodeName = targetNode
		value, err := json.Marshal(spec)
		if err != nil {
			return err
		}
		templates[index].Spec = string(value)
		if err := h.store.UpdateApplicationDeploymentTemplate(&templates[index]); err != nil {
			return err
		}
	}
	return nil
}

func (h *K8sHandler) migrationTemplateSnapshot(applicationID uint) (string, error) {
	templates, err := h.store.ListApplicationDeploymentTemplates(applicationID)
	if err != nil {
		return "", err
	}
	snapshot := make(map[uint]string, len(templates))
	for _, template := range templates {
		snapshot[template.ID] = template.Spec
	}
	value, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return string(value), nil
}

func (h *K8sHandler) restoreMigrationTemplates(migration *model.PersistentVolumeMigration) error {
	if migration.SourceTemplateSpec == "" {
		return nil
	}
	var snapshot map[uint]string
	if err := json.Unmarshal([]byte(migration.SourceTemplateSpec), &snapshot); err != nil {
		return err
	}
	templates, err := h.store.ListApplicationDeploymentTemplates(migration.ApplicationID)
	if err != nil {
		return err
	}
	for index := range templates {
		if spec, ok := snapshot[templates[index].ID]; ok && templates[index].Spec != spec {
			templates[index].Spec = spec
			if err := h.store.UpdateApplicationDeploymentTemplate(&templates[index]); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *K8sHandler) cleanupMigrationTarget(environment *model.Environment, migration *model.PersistentVolumeMigration) {
	_ = K8s.DeletePVCBindingPod(environment.Namespace, strconv.FormatUint(uint64(migration.ID), 10))
	if migration.TargetPVCName != "" {
		_ = K8s.DeleteManagedPVC(environment.Namespace, migration.TargetPVCName, environment.ID)
	}
}

func (h *K8sHandler) failMigration(migration *model.PersistentVolumeMigration, cause error, restoreSource bool) {
	if restoreSource && K8s != nil {
		if environment, err := h.store.GetEnvironmentByID(migration.EnvironmentID); err == nil {
			if deployments, _ := K8s.DeploymentUsingPVC(environment.Namespace, migration.TargetPVCName); len(deployments) == 1 {
				_, _ = K8s.ReplaceDeploymentPVCNode(environment.Namespace, migration.SourceDeployment, migration.TargetPVCName, migration.SourcePVCName, migration.SourceNodeName)
			}
			_ = K8s.ScaleDeployment(environment.Namespace, migration.SourceDeployment, migration.SourceReplicas)
			_ = h.restoreMigrationTemplates(migration)
		}
	}
	_ = h.store.UpdatePersistentVolumeMigration(migration, model.PVCMigrationStatusFailed, cause.Error())
}

func migrationPVCName(source string, id uint) string {
	suffix := "-migrate-" + strconv.FormatUint(uint64(id), 10)
	base := strings.TrimSuffix(strings.TrimSpace(source), "-")
	if len(base)+len(suffix) > 63 {
		base = strings.TrimRight(base[:63-len(suffix)], "-")
	}
	return base + suffix
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func errorsIsNotFound(err error) bool { return err == gorm.ErrRecordNotFound }

func nodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
