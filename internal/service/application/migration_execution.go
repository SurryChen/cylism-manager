package application

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

type MigrationExecutor interface {
	RunMigration(context.Context, ApplicationContext, ReleaseSpec) (string, error)
}

func releaseExecutionTimeout(spec ReleaseSpec) time.Duration {
	timeout := 3 * time.Minute
	if spec.Migration != nil && spec.Migration.Enabled {
		timeout += time.Duration(spec.Migration.TimeoutSeconds+30) * time.Second
	}
	return timeout
}

func migrationResourceName(app ApplicationContext) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s/%s/%d", app.Namespace, app.ApplicationName, app.ReleaseSequence)))
	base := app.ApplicationName
	if len(base) > 35 {
		base = base[:35]
	}
	return fmt.Sprintf("%s-migrate-%x", strings.TrimRight(base, "-"), sum[:8])
}

// The migration uses its own ConfigMap/Secret names so the running workload
// never observes the next release's configuration before migration succeeds.
func renderMigrationJob(app ApplicationContext, spec ReleaseSpec) (*batchv1.Job, *RenderedResources, error) {
	if spec.Migration == nil || !spec.Migration.Enabled || spec.Migration.Target == "" {
		return nil, nil, fmt.Errorf("数据库迁移目标未配置")
	}
	if issues := validateMigrationSpec(spec.Migration); len(issues) > 0 {
		return nil, nil, fmt.Errorf("%s", issues[0].Message)
	}
	if err := validateMigrationConnection(spec); err != nil {
		return nil, nil, err
	}
	migration := *spec.Migration
	originalApplication := app.ApplicationName
	app.ApplicationName = migrationResourceName(app)
	spec.Volumes = nil // A migration never competes with the live workload's RWO volumes.
	resources, err := RenderResources(app, spec)
	if err != nil {
		return nil, nil, err
	}
	var pod corev1.PodSpec
	if resources.Deployment != nil {
		pod = *resources.Deployment.Spec.Template.Spec.DeepCopy()
	} else {
		pod = *resources.StatefulSet.Spec.Template.Spec.DeepCopy()
	}
	container := &pod.Containers[0]
	container.Name = "migration"
	container.Command = append([]string(nil), migration.Command...)
	container.Args = nil
	container.ReadinessProbe = nil
	container.LivenessProbe = nil
	container.StartupProbe = nil
	container.Ports = nil
	// The executor contract is fixed; the signed callback can only select target.
	container.Env = append(container.Env, corev1.EnvVar{Name: "CYLISM_MIGRATION_TARGET", Value: migration.Target}, corev1.EnvVar{Name: "CYLISM_MIGRATION_DIR", Value: migration.Directory}, corev1.EnvVar{Name: "CYLISM_MIGRATION_DATABASE_PREFIX", Value: migration.DatabasePrefix})
	pod.RestartPolicy = corev1.RestartPolicyNever
	token := false
	pod.AutomountServiceAccountToken = &token
	zero := int32(0)
	deadline := int64(migration.TimeoutSeconds)
	ttl := int32(7 * 24 * 3600)
	labels := managedLabels(app)
	labels["cylism.io/task"] = "database-migration"
	labels["cylism.io/migration-application"] = originalApplication
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: app.ApplicationName, Namespace: app.Namespace, Labels: labels}, Spec: batchv1.JobSpec{BackoffLimit: &zero, ActiveDeadlineSeconds: &deadline, TTLSecondsAfterFinished: &ttl, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: pod}}}
	return job, resources, nil
}

func (a *KubernetesApplier) RunMigration(ctx context.Context, app ApplicationContext, spec ReleaseSpec) (string, error) {
	if a.migrations == nil {
		return "", fmt.Errorf("数据库迁移 Job 客户端未初始化")
	}
	job, resources, err := renderMigrationJob(app, spec)
	if err != nil {
		return "", err
	}
	jobs := a.migrations.BatchV1().Jobs(app.Namespace)
	// A previous Job may still run after the manager timed out or restarted.
	// Wait for its Kubernetes deadline instead of racing another image's SQL.
	previous, err := jobs.List(ctx, metav1.ListOptions{LabelSelector: "cylism.io/task=database-migration,cylism.io/migration-application=" + app.ApplicationName})
	if err != nil {
		return "", fmt.Errorf("检查现有迁移任务: %w", err)
	}
	for _, candidate := range previous.Items {
		if candidate.Name == job.Name {
			continue
		}
		terminal := false
		for _, condition := range candidate.Status.Conditions {
			if condition.Status == corev1.ConditionTrue && (condition.Type == batchv1.JobComplete || condition.Type == batchv1.JobFailed) {
				terminal = true
			}
		}
		if !terminal {
			return "", fmt.Errorf("先前迁移任务 %s 尚未结束，请核实任务状态后重试", candidate.Name)
		}
	}
	existing, err := jobs.Get(ctx, job.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		// Create the owner first; missing configuration keeps the Pod pending.
		existing, err = jobs.Create(ctx, job, metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(err) {
			existing, err = jobs.Get(ctx, job.Name, metav1.GetOptions{})
		}
		if err != nil {
			return "", fmt.Errorf("创建数据库迁移任务: %w", err)
		}
		if err = validateMigrationJobOwner(existing, job); err != nil {
			return "", err
		}
		owner := metav1.NewControllerRef(existing, batchv1.SchemeGroupVersion.WithKind("Job"))
		for _, secret := range []*corev1.Secret{resources.ImagePullSecret, resources.Secret} {
			if secret != nil {
				secret.OwnerReferences = []metav1.OwnerReference{*owner}
				if err = a.resources.ApplySecret(ctx, secret); err != nil {
					return "", err
				}
			}
		}
		if resources.ConfigMap != nil {
			resources.ConfigMap.OwnerReferences = []metav1.OwnerReference{*owner}
			if err = a.resources.ApplyConfigMap(ctx, resources.ConfigMap); err != nil {
				return "", err
			}
		}
	}
	if err != nil {
		return "", fmt.Errorf("提交数据库迁移任务: %w", err)
	}
	if err = validateMigrationJobOwner(existing, job); err != nil {
		return "", err
	}
	job = existing
	timeout := time.Duration(spec.Migration.TimeoutSeconds+15) * time.Second
	var terminalFailure bool
	var failureReason string
	err = wait.PollUntilContextTimeout(ctx, time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		current, e := jobs.Get(ctx, job.Name, metav1.GetOptions{})
		if e != nil {
			return false, e
		}
		if current.UID != job.UID {
			return false, fmt.Errorf("迁移任务已被替换，拒绝继续发布")
		}
		for _, condition := range current.Status.Conditions {
			if condition.Status != corev1.ConditionTrue {
				continue
			}
			if condition.Type == batchv1.JobComplete {
				return true, nil
			}
			if condition.Type == batchv1.JobFailed {
				terminalFailure = true
				failureReason = condition.Reason
				if len(failureReason) > 256 {
					failureReason = failureReason[:256]
				}
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return "", fmt.Errorf("迁移任务 %s 未确认完成；请检查任务状态后再重试: %w", job.Name, err)
	}
	logText := migrationJobLogs(ctx, a.migrations, job)
	logText = redactMigrationLog(logText, spec)
	if terminalFailure {
		return "", fmt.Errorf("迁移任务 %s 失败（%s），应用未更新；需核实数据库状态后重试。%s", job.Name, redactMigrationLog(failureReason, spec), logText)
	}
	if logText != "" {
		return fmt.Sprintf("目标 %s；Job %s\n%s", spec.Migration.Target, job.Name, logText), nil
	}
	return fmt.Sprintf("目标 %s；Job %s 已成功", spec.Migration.Target, job.Name), nil
}

func validateMigrationJobOwner(existing, desired *batchv1.Job) error {
	if existing.Labels[ManagedByLabel] != ManagedByValue || existing.Labels[ReleaseLabel] != desired.Labels[ReleaseLabel] || existing.Labels[ApplicationNameLabel] != desired.Name {
		return fmt.Errorf("迁移 Job 不属于当前发布，拒绝接管")
	}
	return nil
}

func migrationJobLogs(ctx context.Context, client kubernetes.Interface, job *batchv1.Job) string {
	pods, err := client.CoreV1().Pods(job.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + job.Name})
	if err != nil {
		return "日志暂不可用"
	}
	for _, pod := range pods.Items {
		owned := false
		for _, owner := range pod.OwnerReferences {
			if owner.UID == job.UID && owner.Kind == "Job" {
				owned = true
			}
		}
		if !owned {
			continue
		}
		limit := int64(16 << 10)
		stream, e := client.CoreV1().Pods(job.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{Container: "migration", LimitBytes: &limit}).Stream(ctx)
		if e != nil {
			continue
		}
		data, e := io.ReadAll(io.LimitReader(stream, limit))
		stream.Close()
		if e == nil {
			return string(data)
		}
	}
	return ""
}
func redactMigrationLog(value string, spec ReleaseSpec) string {
	for _, secret := range spec.Secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	if spec.RegistryCredential != "" {
		value = strings.ReplaceAll(value, spec.RegistryCredential, "[REDACTED]")
	}
	return strings.ToValidUTF8(value, "�")
}
