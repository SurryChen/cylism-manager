package database

import (
	"context"
	"fmt"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type InstanceUsage struct {
	CheckedAt        time.Time `json:"checked_at"`
	PodName          string    `json:"pod_name"`
	NodeName         string    `json:"node_name"`
	Phase            string    `json:"phase"`
	Ready            bool      `json:"ready"`
	Restarts         int32     `json:"restarts"`
	LastTermination  string    `json:"last_termination,omitempty"`
	CPUMilli         *int64    `json:"cpu_millicores"`
	MemoryBytes      *int64    `json:"memory_bytes"`
	CPULimitMilli    int64     `json:"cpu_limit_millicores"`
	MemoryLimitBytes int64     `json:"memory_limit_bytes"`
	MetricsAvailable bool      `json:"metrics_available"`
	MetricsMessage   string    `json:"metrics_message,omitempty"`
}

// Report the running MySQL container's limits, not the desired instance budget.
// Missing samples remain null rather than suggesting the process uses no memory.
func ReadInstanceUsage(ctx context.Context, client *k8s.Client, item *model.DatabaseInstance) (*InstanceUsage, error) {
	if client == nil || client.Clientset == nil {
		return nil, fmt.Errorf("集群客户端不可用")
	}
	pod, err := client.Clientset.CoreV1().Pods(Namespace).Get(ctx, ResourceName(item.ID)+"-0", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("读取 MySQL Pod 失败：%w", err)
	}
	if pod.Labels[k8s.ManagedByLabel] != k8s.ManagedByValue || pod.Labels["app.kubernetes.io/name"] != ResourceName(item.ID) {
		return nil, fmt.Errorf("Pod 不属于当前平台实例")
	}
	result := &InstanceUsage{CheckedAt: time.Now(), PodName: pod.Name, NodeName: pod.Spec.NodeName, Phase: string(pod.Status.Phase)}
	for _, container := range pod.Spec.Containers {
		if container.Name == "mysql" {
			result.CPULimitMilli = container.Resources.Limits.Cpu().MilliValue()
			result.MemoryLimitBytes = container.Resources.Limits.Memory().Value()
		}
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == "mysql" {
			result.Ready = status.Ready && pod.DeletionTimestamp == nil && pod.Status.Phase == corev1.PodRunning
			result.Restarts = status.RestartCount
			if previous := status.LastTerminationState.Terminated; previous != nil {
				result.LastTermination = previous.Reason
			}
			if status.State.Waiting != nil {
				result.Phase = status.State.Waiting.Reason
			}
		}
	}
	samples, metricsErr := client.ListRuntimePodMetrics(ctx, Namespace)
	if metricsErr == nil {
		if sample, ok := samples[pod.Name]; ok {
			for _, container := range sample.Containers {
				if container.Name == "mysql" {
					result.CPUMilli = &container.CPUMilli
					result.MemoryBytes = &container.MemoryBytes
					result.MetricsAvailable = true
					break
				}
			}
		}
	}
	if !result.MetricsAvailable {
		result.MetricsMessage = "未取得 MySQL 指标样本，请检查集群 Metrics Server；Pod 刚启动时请稍后刷新"
	}
	return result, nil
}
