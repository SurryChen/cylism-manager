package k8s

import (
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const victoriaMetricsMigrationImage = "busybox:1.36.1"

// StartVictoriaMetricsHostPathMigration converts a legacy metrics directory to
// the platform-owned PVC on the same node. The source hostPath is preserved.
func (c *Client) StartVictoriaMetricsHostPathMigration(request VictoriaMetricsMigrationRequest) (*VictoriaMetricsStatus, error) {
	if c == nil || c.Clientset == nil {
		return c.VictoriaMetricsStatus(), fmt.Errorf("Kubernetes 客户端未初始化")
	}
	if err := validateVictoriaMetricsPVCConfig(VictoriaMetricsConfig{NodeName: "migration", Storage: request.Storage}); err != nil {
		return c.VictoriaMetricsStatus(), err
	}
	deployment, err := c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Get(c.Ctx(), victoriaMetricsName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return c.VictoriaMetricsStatus(), fmt.Errorf("VictoriaMetrics 尚未安装")
	}
	if err != nil {
		return c.VictoriaMetricsStatus(), fmt.Errorf("读取 VictoriaMetrics 配置失败: %w", err)
	}
	config, legacy := victoriaMetricsConfigFromDeployment(deployment)
	if !legacy || config.DataPath == "" {
		return c.VictoriaMetricsStatus(), fmt.Errorf("当前 VictoriaMetrics 未使用旧 hostPath 存储，无需迁移")
	}
	node, err := c.Clientset.CoreV1().Nodes().Get(c.Ctx(), config.NodeName, metav1.GetOptions{})
	if err != nil {
		return c.VictoriaMetricsStatus(), fmt.Errorf("读取数据节点失败: %w", err)
	}
	if !nodeReady(node) {
		return c.VictoriaMetricsStatus(), fmt.Errorf("数据节点 %s 未就绪，不能迁移", config.NodeName)
	}
	if job, getErr := c.Clientset.BatchV1().Jobs(victoriaMetricsNamespace).Get(c.Ctx(), victoriaMetricsMigrationJobName, metav1.GetOptions{}); getErr == nil {
		if jobFailed(job) {
			if deleteErr := c.Clientset.BatchV1().Jobs(victoriaMetricsNamespace).Delete(c.Ctx(), job.Name, metav1.DeleteOptions{}); deleteErr != nil {
				return c.VictoriaMetricsStatus(), fmt.Errorf("清理失败的存储迁移任务失败: %w", deleteErr)
			}
		} else {
			return c.VictoriaMetricsStatus(), fmt.Errorf("VictoriaMetrics 存储迁移正在执行，请等待当前任务结束")
		}
	} else if !apierrors.IsNotFound(getErr) {
		return c.VictoriaMetricsStatus(), fmt.Errorf("读取存储迁移任务失败: %w", getErr)
	}
	if err := ensureMonitoringNamespace(c); err != nil {
		return c.VictoriaMetricsStatus(), err
	}
	if err := upsertVictoriaMetricsPVC(c, VictoriaMetricsConfig{Storage: request.Storage, StorageClassName: request.StorageClassName}); err != nil {
		return c.VictoriaMetricsStatus(), err
	}
	if err := scaleVictoriaMetrics(c, 0); err != nil {
		return c.VictoriaMetricsStatus(), err
	}
	if err := createVictoriaMetricsMigrationJob(c, config); err != nil {
		_ = scaleVictoriaMetrics(c, 1)
		return c.VictoriaMetricsStatus(), err
	}
	return c.VictoriaMetricsStatus(), nil
}

func createVictoriaMetricsMigrationJob(c *Client, config VictoriaMetricsConfig) error {
	sourceType := corev1.HostPathDirectory
	backoffLimit := int32(0)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsMigrationJobName, Namespace: victoriaMetricsNamespace, Labels: map[string]string{ManagedByLabel: ManagedByValue, "cylism.io/component": "monitoring", "cylism.io/task": "victoria-metrics-storage-migration"}},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{ManagedByLabel: ManagedByValue, "cylism.io/task": "victoria-metrics-storage-migration"}},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					NodeSelector:  map[string]string{corev1.LabelHostname: config.NodeName},
					Volumes: []corev1.Volume{
						{Name: "source", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: config.DataPath, Type: &sourceType}}},
						{Name: "target", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: victoriaMetricsPVCName}}},
					},
					Containers: []corev1.Container{{
						Name:      "copy-data",
						Image:     victoriaMetricsMigrationImage,
						Command:   []string{"sh", "-ec", "test -d /source && tar -C /source -cf - . | tar -C /target -xf - && test \"$(find /source -type f | wc -l)\" = \"$(find /target -type f | wc -l)\""},
						Resources: alertingResources("100m", "128Mi"),
						VolumeMounts: []corev1.VolumeMount{
							{Name: "source", MountPath: "/source", ReadOnly: true},
							{Name: "target", MountPath: "/target"},
						},
					}},
				},
			},
		},
	}
	if _, err := c.Clientset.BatchV1().Jobs(victoriaMetricsNamespace).Create(c.Ctx(), job, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("创建 VictoriaMetrics 数据迁移任务失败: %w", err)
	}
	return nil
}

// reconcileVictoriaMetricsMigration performs an idempotent cutover or recovery
// from the Kubernetes Job state, so a platform restart does not strand metrics.
func (c *Client) reconcileVictoriaMetricsMigration() error {
	job, err := c.Clientset.BatchV1().Jobs(victoriaMetricsNamespace).Get(c.Ctx(), victoriaMetricsMigrationJobName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if jobFailed(job) {
		if err := scaleVictoriaMetrics(c, 1); err != nil {
			return err
		}
		return nil
	}
	if !jobSucceeded(job) {
		return fmt.Errorf("迁移任务仍在执行")
	}
	deployment, err := c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Get(c.Ctx(), victoriaMetricsName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	config, legacy := victoriaMetricsConfigFromDeployment(deployment)
	if !legacy || config.DataPath == "" {
		return nil
	}
	config.DataPath = ""
	config.Storage = "managed"
	if err := upsertVictoriaMetricsDeployment(c, config); err != nil {
		return err
	}
	return c.Clientset.BatchV1().Jobs(victoriaMetricsNamespace).Delete(c.Ctx(), victoriaMetricsMigrationJobName, metav1.DeleteOptions{})
}

func (c *Client) victoriaMetricsMigrationStatus() *VictoriaMetricsStorageMigration {
	if c == nil || c.Clientset == nil {
		return nil
	}
	job, err := c.Clientset.BatchV1().Jobs(victoriaMetricsNamespace).Get(c.Ctx(), victoriaMetricsMigrationJobName, metav1.GetOptions{})
	if err != nil {
		return nil
	}
	if jobFailed(job) {
		return &VictoriaMetricsStorageMigration{Stage: VictoriaMetricsMigrationFailed, Message: jobStatusMessage(job)}
	}
	if jobSucceeded(job) {
		return &VictoriaMetricsStorageMigration{Stage: VictoriaMetricsMigrationCopying, Message: "数据复制已完成，正在切换到 PVC"}
	}
	return &VictoriaMetricsStorageMigration{Stage: VictoriaMetricsMigrationCopying, Message: "正在复制并校验 VictoriaMetrics 历史数据"}
}

func scaleVictoriaMetrics(c *Client, replicas int32) error {
	deployment, err := c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Get(c.Ctx(), victoriaMetricsName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("读取 VictoriaMetrics 工作负载失败: %w", err)
	}
	deployment.Spec.Replicas = &replicas
	if _, err := c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Update(c.Ctx(), deployment, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("更新 VictoriaMetrics 副本失败: %w", err)
	}
	return nil
}

func jobSucceeded(job *batchv1.Job) bool {
	return job.Status.Succeeded > 0 || jobCondition(job, batchv1.JobComplete)
}

func jobFailed(job *batchv1.Job) bool {
	return job.Status.Failed > 0 || jobCondition(job, batchv1.JobFailed)
}

func jobCondition(job *batchv1.Job, conditionType batchv1.JobConditionType) bool {
	for _, condition := range job.Status.Conditions {
		if condition.Type == conditionType && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func jobStatusMessage(job *batchv1.Job) string {
	for _, condition := range job.Status.Conditions {
		if condition.Message != "" {
			return condition.Message
		}
	}
	return "数据复制任务失败，已恢复旧 hostPath 工作负载"
}
