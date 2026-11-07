package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type OperatorInstallation struct {
	State        string    `json:"state"`
	Managed      bool      `json:"-"`
	CreatedAt    time.Time `json:"-"`
	Progress     string    `json:"progress"`
	Failure      string    `json:"failure,omitempty"`
	CanCancel    bool      `json:"can_cancel"`
	CanUninstall bool      `json:"can_uninstall"`
}

// OperatorInstallationStatus follows the Helm job and the controller's real readiness.
// Transient image pulls and scheduling failures remain running, with their reason in progress.
func OperatorInstallationStatus(ctx context.Context, client *k8s.Client) (*OperatorInstallation, error) {
	state, err := OperatorState(ctx, client)
	if err != nil {
		return nil, err
	}
	result := &OperatorInstallation{State: state, Progress: "等待 Helm 安装 MySQL Operator"}
	chart, err := client.DynamicClient.Resource(mysqlOperatorHelmGVR).Namespace("kube-system").Get(ctx, mysqlOperatorHelmName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if err == nil {
		result.Managed = chart.GetLabels()[k8s.ManagedByLabel] == k8s.ManagedByValue
		result.CreatedAt = chart.GetCreationTimestamp().Time
	}
	if state == "ready" && result.Managed {
		clusters, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace("").List(ctx, metav1.ListOptions{Limit: 1})
		if err != nil {
			return nil, fmt.Errorf("检查 MySQL 集群使用情况：%w", err)
		}
		result.CanUninstall = len(clusters.Items) == 0
	}
	if state == "suspended" {
		result.Progress = "MySQL Operator 已随集群停用；可通过恢复集群重新启用"
		return result, nil
	}
	if state == "cancelling" {
		return operatorCleanupStatus(ctx, client, result)
	}
	if state == "ready" {
		result.Progress = "MySQL Operator 已就绪"
		return result, nil
	}
	if !result.Managed {
		return result, nil
	}
	result.CanCancel = true
	jobs, err := client.Clientset.BatchV1().Jobs("kube-system").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, job := range jobs.Items {
		if job.Name != "helm-install-"+mysqlOperatorHelmName {
			continue
		}
		for _, condition := range job.Status.Conditions {
			if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
				result.State = "failed"
				result.Failure = fmt.Sprintf("Helm 安装失败：%s %s", condition.Reason, condition.Message)
				result.Progress = result.Failure
				return result, nil
			}
		}
		if job.Status.Succeeded > 0 {
			result.Progress = "Helm 安装已完成，等待 MySQL Operator Pod 就绪"
		}
	}
	for _, namespace := range []string{"kube-system", "mysql-operator"} {
		pods, err := client.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		for _, pod := range pods.Items {
			if namespace == "kube-system" && !strings.HasPrefix(pod.Name, "helm-install-"+mysqlOperatorHelmName+"-") {
				continue
			}
			if namespace == "mysql-operator" && !strings.HasPrefix(pod.Name, "mysql-operator-") && pod.Labels["app.kubernetes.io/name"] != "mysql-operator" {
				continue
			}
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse {
					result.Progress = fmt.Sprintf("%s：%s %s", pod.Name, condition.Reason, condition.Message)
				}
			}
			statuses := append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...)
			for _, container := range statuses {
				if waiting := container.State.Waiting; waiting != nil {
					result.Progress = fmt.Sprintf("%s / %s：%s %s", pod.Name, container.Name, waiting.Reason, waiting.Message)
				}
				if stopped := container.State.Terminated; stopped != nil && stopped.ExitCode != 0 {
					result.Progress = fmt.Sprintf("%s / %s：退出码 %d，%s %s", pod.Name, container.Name, stopped.ExitCode, stopped.Reason, stopped.Message)
				}
			}
		}
	}
	return result, nil
}

// A deleting HelmChart can wait indefinitely on image pulls, scheduling, or a
// failed uninstall job. Report the cleanup itself rather than the old install job.
func operatorCleanupStatus(ctx context.Context, client *k8s.Client, result *OperatorInstallation) (*OperatorInstallation, error) {
	result.Progress = "等待 Helm 控制器创建 MySQL Operator 卸载任务"
	name := "helm-delete-" + mysqlOperatorHelmName
	job, err := client.Clientset.BatchV1().Jobs("kube-system").Get(ctx, name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if err == nil {
		result.Progress = "Helm 正在卸载 MySQL Operator"
		for _, condition := range job.Status.Conditions {
			if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
				result.Failure = fmt.Sprintf("Helm 卸载失败：%s %s", condition.Reason, condition.Message)
				result.Progress = result.Failure
				return result, nil
			}
		}
		if job.Status.Succeeded > 0 {
			result.Progress = "Helm 卸载已完成，等待清理 HelmChart 和 Operator 资源"
			return result, nil
		}
	}
	pods, err := client.Clientset.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + name})
	if err != nil {
		return nil, err
	}
	// Ignore old terminating pods left by Helm retries and inspect the latest
	// live attempt; otherwise an obsolete failure can hide current progress.
	var latest *corev1.Pod
	terminating := 0
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.DeletionTimestamp != nil {
			terminating++
			continue
		}
		if job != nil {
			for _, owner := range pod.OwnerReferences {
				if owner.Kind == "Job" && owner.UID != job.UID {
					pod = nil
					break
				}
			}
		}
		if pod != nil && (latest == nil || latest.CreationTimestamp.Before(&pod.CreationTimestamp)) {
			latest = pod
		}
	}
	if latest == nil {
		if terminating > 0 {
			result.Progress += fmt.Sprintf("；%d 个旧卸载 Pod 正在终止", terminating)
		}
		return result, nil
	}
	result.Progress = fmt.Sprintf("卸载 Pod %s：%s", latest.Name, latest.Status.Phase)
	for _, condition := range latest.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse {
			result.Progress = fmt.Sprintf("%s：%s %s", latest.Name, condition.Reason, condition.Message)
			return result, nil
		}
	}
	for _, container := range append(latest.Status.InitContainerStatuses, latest.Status.ContainerStatuses...) {
		if waiting := container.State.Waiting; waiting != nil {
			result.Progress = fmt.Sprintf("%s / %s：%s %s", latest.Name, container.Name, waiting.Reason, waiting.Message)
		}
		if stopped := container.State.Terminated; stopped != nil && stopped.ExitCode != 0 {
			result.Progress = fmt.Sprintf("%s / %s：退出码 %d，%s %s", latest.Name, container.Name, stopped.ExitCode, stopped.Reason, stopped.Message)
		}
	}
	// ContainerCreating alone omits image-pull, mount, and sandbox errors.
	events, err := client.Clientset.CoreV1().Events("kube-system").List(ctx, metav1.ListOptions{FieldSelector: "involvedObject.uid=" + string(latest.UID)})
	if err != nil {
		return nil, err
	}
	var recent *corev1.Event
	for i := range events.Items {
		event := &events.Items[i]
		if event.InvolvedObject.UID != latest.UID {
			continue
		}
		if recent == nil || recent.LastTimestamp.Before(&event.LastTimestamp) {
			recent = event
		}
	}
	if recent != nil && latest.Status.Phase == corev1.PodPending {
		result.Progress += fmt.Sprintf("；%s %s", recent.Reason, recent.Message)
	}
	return result, nil
}
