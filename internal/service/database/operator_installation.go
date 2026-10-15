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
	if state == "cancelling" {
		result.Progress = "已请求中止安装，等待 Helm 清理安装资源"
		return result, nil
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
