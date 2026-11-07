package operations

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/service/database"
	"gorm.io/gorm"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//go:embed job.py
var jobScript string

func taskName(id uint) string { return fmt.Sprintf("cylism-db-task-%d", id) }
func jobName(task *model.DatabaseOperation) string {
	return fmt.Sprintf("%s-%d", taskName(task.ID), task.Phase)
}
func taskLabels(id uint) map[string]string {
	return map[string]string{k8s.ManagedByLabel: k8s.ManagedByValue, "cylism.io/database-task": strconv.FormatUint(uint64(id), 10)}
}
func owned(labels map[string]string, id uint) bool {
	return labels[k8s.ManagedByLabel] == k8s.ManagedByValue && labels["cylism.io/database-task"] == fmt.Sprint(id)
}

func (s *Service) executePhase(ctx context.Context, task *model.DatabaseOperation, req Request) (bool, string, error) {
	if s.client == nil {
		return false, "", fmt.Errorf("集群客户端未配置")
	}
	client, err := s.client(ctx, task.ClusterID)
	if err != nil {
		return false, "", err
	}
	if task.Kind == "deploy" {
		_, id, err := ParseReference(task.Target)
		if err != nil {
			return false, "", err
		}
		var item model.DatabaseInstance
		if err = s.store.DB().First(&item, id).Error; err != nil {
			return false, "", err
		}
		if task.Phase == 0 {
			root, e := security.Decrypt(s.key, item.EncryptedRoot)
			if e != nil {
				return false, "", fmt.Errorf("读取管理密码失败")
			}
			password := ""
			if item.EncryptedPassword != "" {
				password, e = security.Decrypt(s.key, item.EncryptedPassword)
				if e != nil {
					return false, "", fmt.Errorf("读取业务密码失败")
				}
			}
			if e = database.Apply(ctx, client, &item, root, password); e != nil {
				return false, "", e
			}
			if e = s.store.DB().Model(&item).Updates(map[string]any{"status": "starting", "last_error": ""}).Error; e != nil {
				return false, "", e
			}
			return true, "", nil
		}
		status, e := database.Status(ctx, client, &item)
		if e != nil {
			return false, "", e
		}
		if e = s.store.DB().Model(&item).Update("status", status).Error; e != nil {
			return false, "", e
		}
		return status == "ready", "等待就绪；可在实例详情查看 Pod/PVC", nil
	}
	if task.Kind == "switch" && task.Phase == 1 {
		return true, "上线模板已更新；请到应用页面发布所选模板，使运行中的应用生效", s.switchConnections(task)
	}
	jobs := client.Clientset.BatchV1().Jobs(database.Namespace)
	job, err := jobs.Get(ctx, jobName(task), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if err = s.ensureInputs(ctx, client, task, req); err != nil {
			return false, "", err
		}
		job = buildJob(task, req)
		if _, err = jobs.Create(ctx, job, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return false, "", fmt.Errorf("提交数据库 Job 失败：%w", err)
		}
		return false, "已提交 Job，等待调度与执行", nil
	}
	if err != nil {
		return false, "", err
	}
	if !owned(job.Labels, task.ID) {
		return false, "", fmt.Errorf("同名 Job 不属于当前任务，拒绝接管")
	}
	if job.DeletionTimestamp != nil {
		return false, "等待执行 Job 退出", nil
	}
	data, e := s.jobOutput(ctx, client, task, job)
	if e != nil {
		return false, "读取阶段日志中；Job 状态将在下次检查", nil
	}
	summary, result, reason, completed := parseEvents(data)
	failed := false
	success := false
	for _, condition := range job.Status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		failed = failed || condition.Type == batchv1.JobFailed
		success = success || condition.Type == batchv1.JobComplete
	}
	// Failed pods alone are not final until the Job controller records failure.
	if failed {
		return false, "", fmt.Errorf("%s：%s", task.PhaseName, errorMessage(reason))
	}
	if success {
		if !completed {
			return false, "", fmt.Errorf("阶段结束但未返回完成标记，保留目标数据与资源锁")
		}
		return true, result, nil
	}
	if summary == "" {
		summary = "Job 执行中；可请求在当前阶段结束后暂停"
	}
	return false, summary, nil
}

func buildJob(task *model.DatabaseOperation, req Request) *batchv1.Job {
	zero := int32(0)
	no := false
	grace := int64(30)
	volumes := []corev1.Volume{{Name: "input", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: taskName(task.ID), DefaultMode: ptr(int32(0440))}}}}
	mounts := []corev1.VolumeMount{{Name: "input", MountPath: "/run/task", ReadOnly: true}}
	if task.Kind == "migrate" || task.Kind == "replicate" {
		volumes = append(volumes, corev1.Volume{Name: "work", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: taskName(task.ID)}}})
	} else {
		volumes = append(volumes, corev1.Volume{Name: "work", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}})
	}
	mounts = append(mounts, corev1.VolumeMount{Name: "work", MountPath: "/work"})
	return &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: jobName(task), Namespace: database.Namespace, Labels: taskLabels(task.ID)}, Spec: batchv1.JobSpec{
		BackoffLimit: &zero, // No automatic replay of partial database mutations.
		// No ActiveDeadlineSeconds: long data copies are controlled by the user.
		Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: taskLabels(task.ID)}, Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever, AutomountServiceAccountToken: &no, TerminationGracePeriodSeconds: &grace,
			NodeSelector:    map[string]string{corev1.LabelHostname: task.NodeName},
			Tolerations:     []corev1.Toleration{{Key: "cylism.io/default-no-schedule", Value: "true", Operator: corev1.TolerationOpEqual, Effect: corev1.TaintEffectNoSchedule}},
			DNSConfig:       &corev1.PodDNSConfig{Options: []corev1.PodDNSConfigOption{{Name: "ndots", Value: ptr("1")}}},
			SecurityContext: &corev1.PodSecurityContext{FSGroup: ptr(int64(27))}, Volumes: volumes,
			Containers: []corev1.Container{{Name: "mysql-shell", Image: req.Image, Command: []string{"mysqlsh", "--quiet-start=2", "--log-level=none", "--py", "--file", "/run/task/job.py"}, Env: []corev1.EnvVar{{Name: "CYLISM_PHASE", Value: fmt.Sprint(task.Phase)}, {Name: "MYSQLSH_USER_CONFIG_HOME", Value: "/tmp/mysqlsh"}}, VolumeMounts: mounts,
				Resources:       corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("256Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("1Gi")}},
				SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: &no, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}},
			}},
		}},
	}}
}
func ptr[T any](v T) *T { return &v }

func (s *Service) ensureInputs(ctx context.Context, client *k8s.Client, task *model.DatabaseOperation, req Request) error {
	namespaces := client.Clientset.CoreV1().Namespaces()
	if _, err := namespaces.Get(ctx, database.Namespace, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		if _, err = namespaces.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: database.Namespace}}, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	} else if err != nil {
		return err
	}
	plain, err := security.Decrypt(s.key, task.EncryptedInput)
	if err != nil {
		return fmt.Errorf("读取任务加密参数失败")
	}
	name := taskName(task.ID)
	secrets := client.Clientset.CoreV1().Secrets(database.Namespace)
	secret, err := secrets.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = secrets.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: database.Namespace, Labels: taskLabels(task.ID)}, Type: corev1.SecretTypeOpaque, Data: map[string][]byte{"input.json": []byte(plain), "job.py": []byte(jobScript)}}, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if !owned(secret.Labels, task.ID) || !bytes.Equal(secret.Data["input.json"], []byte(plain)) || !bytes.Equal(secret.Data["job.py"], []byte(jobScript)) {
		return fmt.Errorf("任务 Secret 已变化，拒绝覆盖")
	}
	if task.Kind != "migrate" && task.Kind != "replicate" {
		return nil
	}
	claims := client.Clientset.CoreV1().PersistentVolumeClaims(database.Namespace)
	claim, err := claims.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		capacity, e := resource.ParseQuantity(req.StorageSize)
		if e != nil {
			return e
		}
		claim = &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: database.Namespace, Labels: taskLabels(task.ID)}, Spec: corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, StorageClassName: ptr(req.StorageClass), Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: capacity}}}}
		_, err = claims.Create(ctx, claim, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if !owned(claim.Labels, task.ID) {
		return fmt.Errorf("快照 PVC 不属于当前任务")
	}
	return nil
}

func (s *Service) jobOutput(ctx context.Context, client *k8s.Client, task *model.DatabaseOperation, job *batchv1.Job) ([]byte, error) {
	pods, err := client.Clientset.CoreV1().Pods(database.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + jobName(task)})
	if err != nil {
		return nil, err
	}
	for _, pod := range pods.Items {
		if !owned(pod.Labels, task.ID) {
			continue
		}
		if job.UID != "" && pod.Labels["controller-uid"] != string(job.UID) && pod.Labels["batch.kubernetes.io/controller-uid"] != string(job.UID) {
			continue
		}
		limit := int64(128 * 1024)
		return client.Clientset.CoreV1().Pods(database.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{Container: "mysql-shell", LimitBytes: &limit}).DoRaw(ctx)
	}
	return nil, nil
}

type event struct {
	Done          bool            `json:"done"`
	Error         string          `json:"error"`
	ProgressBytes *int64          `json:"progress_bytes"`
	Waiting       string          `json:"waiting"`
	Result        json.RawMessage `json:"result"`
}

func parseEvents(output []byte) (summary, result, reason string, done bool) {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 4096), 128*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "CYLISM_EVENT:") {
			continue
		}
		var e event
		if json.Unmarshal([]byte(strings.TrimPrefix(line, "CYLISM_EVENT:")), &e) != nil {
			continue
		}
		if e.ProgressBytes != nil && *e.ProgressBytes >= 0 {
			summary = fmt.Sprintf("快照已写入 %.1f MiB（阶段进度，非完成百分比）", float64(*e.ProgressBytes)/(1024*1024))
		}
		if e.Waiting == "replication_connecting" {
			summary = "等待复制线程连接；可终止任务检查源库白名单与复制权限"
		}
		if e.Error != "" {
			reason = e.Error
		}
		if len(e.Result) > 0 && len(e.Result) <= 32*1024 {
			result = string(e.Result)
		}
		done = done || e.Done
	}
	return
}
func errorMessage(code string) string {
	if code == "replication_source_mismatch" {
		return "现有通道的源实例 UUID 与所选源库不一致，未登记；请核对源实例"
	}
	if code == "replication_multiple_channels" {
		return "目标存在多个复制通道；当前仅支持单源复制，未登记也未修改任何通道"
	}
	if code == "local_infile_required" {
		return "目标须开启 local_infile，MySQL Shell 才能导入快照；请在目标参数中启用后重新创建任务"
	}
	if code == "target_read_only" {
		return "目标处于只读状态，不能导入快照"
	}
	if code == "load_objects_mismatch" {
		return "导入后的表、视图、存储过程、触发器或事件清单不一致，请检查目标数据"
	}
	values := map[string]string{"same_server": "源和目标实际为同一数据库服务器", "version_mismatch": "仅支持相同 MySQL 8.0 或 8.4 系列，请检查源与目标版本", "schema_missing": "源库清单变化或无可迁移数据库", "target_schema_exists": "目标已有同名数据库，拒绝覆盖", "target_has_replication": "目标已有复制通道，拒绝改写", "target_not_empty": "复制初始化要求全新空目标", "gtid_required": "源与目标须启用 GTID", "binlog_required": "源库未开启 binlog", "target_gtid_not_empty": "目标已有 GTID 历史，请使用全新实例", "inconsistent_gtid": "源快照 GTID 不一致，禁止建立复制", "snapshot_gtid_missing": "快照缺少 GTID 位点", "replication_missing": "复制通道不存在", "replication_not_running": "复制线程未全部运行", "replication_thread_error": "复制线程报错，请检查 binlog 保留、复制权限和网络", "load_schema_missing": "导入后缺少所选数据库"}
	if value := values[code]; value != "" {
		return value
	}
	return "MySQL 命令未完成，请检查管理权限、TLS、网络、数据库兼容性或执行 Pod 状态（原始错误已隐藏以免泄露凭据）"
}

func (s *Service) removeSecrets(ctx context.Context, task *model.DatabaseOperation) error {
	if task.Kind == "deploy" {
		return nil
	}
	client, err := s.client(ctx, task.ClusterID)
	if err != nil {
		return err
	}
	secrets := client.Clientset.CoreV1().Secrets(database.Namespace)
	secret, err := secrets.Get(ctx, taskName(task.ID), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !owned(secret.Labels, task.ID) {
		return fmt.Errorf("任务 Secret 不受平台管理")
	}
	return secrets.Delete(ctx, secret.Name, metav1.DeleteOptions{})
}
func (s *Service) cleanupJobs(ctx context.Context, task *model.DatabaseOperation) error {
	if task.Kind == "deploy" {
		client, err := s.client(ctx, task.ClusterID)
		if err != nil {
			return err
		}
		_, id, err := ParseReference(task.Target)
		if err != nil {
			return err
		}
		var item model.DatabaseInstance
		if err = s.store.DB().First(&item, id).Error; err != nil {
			return err
		}
		status, err := database.Status(ctx, client, &item)
		if err != nil {
			return err
		}
		return s.store.DB().Model(&item).Update("status", status).Error
	}
	if err := s.stopJobs(ctx, task, false); err != nil {
		return err
	}
	return s.removeSecrets(ctx, task)
}

// First stop all mutation Jobs, then a separate read-only Job inventories the
// channel. Its identity survives restarts and is excluded from the stop pass.
func (s *Service) settleReplication(ctx context.Context, task *model.DatabaseOperation) (bool, error) {
	if err := s.stopJobs(ctx, task, true); err != nil {
		return false, err
	}
	var req Request
	if err := json.Unmarshal([]byte(task.Spec), &req); err != nil {
		return false, err
	}
	audit := *task
	audit.Phase = -1
	audit.PhaseName = "核对终止后的复制通道"
	done, result, err := s.executePhase(ctx, &audit, req)
	if err != nil {
		// Unlike mutation phases, this Job only reads SHOW REPLICA STATUS.
		// It is safe to retry after transient connection errors during cancel.
		if client, e := s.client(ctx, task.ClusterID); e == nil {
			jobs := client.Clientset.BatchV1().Jobs(database.Namespace)
			if job, e := jobs.Get(ctx, jobName(&audit), metav1.GetOptions{}); e == nil && owned(job.Labels, task.ID) && job.DeletionTimestamp == nil {
				for _, condition := range job.Status.Conditions {
					if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
						foreground := metav1.DeletePropagationForeground
						_ = jobs.Delete(ctx, job.Name, metav1.DeleteOptions{PropagationPolicy: &foreground})
						break
					}
				}
			}
		}
		return false, err
	}
	if !done {
		return false, err
	}
	var observed struct {
		Present    *bool `json:"channel_present"`
		Running    bool  `json:"io_running"`
		SQLRunning bool  `json:"sql_running"`
	}
	if err = json.Unmarshal([]byte(result), &observed); err != nil || observed.Present == nil {
		return false, fmt.Errorf("未收到复制通道状态")
	}
	err = s.store.DB().Transaction(func(tx *gorm.DB) error {
		channel := fmt.Sprintf("cylism_%d", task.ID)
		if task.Kind == "detach" {
			var input jobInput
			plain, e := security.Decrypt(s.key, task.EncryptedInput)
			if e != nil {
				return e
			}
			if e = json.Unmarshal([]byte(plain), &input); e != nil {
				return e
			}
			channel = input.Channel
		}
		var relation model.DatabaseReplication
		e := tx.Where("target = ?", task.Target).First(&relation).Error
		if !*observed.Present {
			return tx.Where("target = ? AND channel = ?", task.Target, channel).Delete(&model.DatabaseReplication{}).Error
		}
		status := "error"
		if observed.Running && observed.SQLRunning {
			status = "running"
		}
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return tx.Create(&model.DatabaseReplication{Source: task.Source, Target: task.Target, TaskID: task.ID, Channel: channel, Status: status}).Error
		}
		if e != nil {
			return e
		}
		if relation.Channel != channel {
			return fmt.Errorf("复制通道记录冲突")
		}
		return tx.Model(&relation).Update("status", status).Error
	})
	return err == nil, err
}

func (s *Service) stopJobs(ctx context.Context, task *model.DatabaseOperation, keepAudit bool) error {
	if task.Kind == "deploy" {
		return nil
	}
	client, err := s.client(ctx, task.ClusterID)
	if err != nil {
		return err
	}
	selector := "cylism.io/database-task=" + fmt.Sprint(task.ID)
	jobs, err := client.Clientset.BatchV1().Jobs(database.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return err
	}
	foreground := metav1.DeletePropagationForeground
	pending := false
	audit := *task
	audit.Phase = -1
	for _, job := range jobs.Items {
		if keepAudit && job.Name == jobName(&audit) {
			continue
		}
		pending = true
		if !owned(job.Labels, task.ID) {
			return fmt.Errorf("Job 不受平台管理")
		}
		if job.DeletionTimestamp == nil {
			if err = client.Clientset.BatchV1().Jobs(database.Namespace).Delete(ctx, job.Name, metav1.DeleteOptions{PropagationPolicy: &foreground}); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	pods, err := client.Clientset.CoreV1().Pods(database.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		if keepAudit && pod.Labels["job-name"] == jobName(&audit) {
			continue
		}
		// A Pod may outlive its Job, e.g. following a controller restart.
		if !owned(pod.Labels, task.ID) {
			return fmt.Errorf("Pod 不受平台管理")
		}
		pending = true
		if pod.DeletionTimestamp == nil {
			if err = client.Clientset.CoreV1().Pods(database.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	if pending {
		return fmt.Errorf("正在等待执行资源退出")
	}
	return nil
}
