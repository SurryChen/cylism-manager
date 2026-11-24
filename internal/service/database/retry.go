package database

import (
	"fmt"
	"strings"

	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
)

func ValidateManagedPod(item *model.MySQLCluster, pod *corev1.Pod) error {
	if pod.Namespace != item.Namespace || !strings.HasPrefix(pod.Name, item.ResourceName+"-") || pod.Labels["mysql.oracle.com/cluster"] != item.ResourceName {
		return fmt.Errorf("Pod 不属于当前 MySQL 集群")
	}
	owned := false
	for _, owner := range pod.OwnerReferences {
		if owner.Controller != nil && *owner.Controller && (owner.Kind == "StatefulSet" || owner.Kind == "ReplicaSet") && strings.HasPrefix(owner.Name, item.ResourceName) {
			owned = true
		}
	}
	if !owned {
		return fmt.Errorf("Pod 没有可重建它的集群控制器")
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return fmt.Errorf("Pod 已就绪，无需重建")
		}
	}
	return nil
}

// ValidateRetryPod limits a retry to a failed, controller-owned Pod of this cluster.
// The caller deletes only the Pod; StatefulSets, claims and volumes are retained.
func ValidateRetryPod(item *model.MySQLCluster, pod *corev1.Pod) error {
	if err := ValidateManagedPod(item, pod); err != nil {
		return err
	}
	for _, status := range append(append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...), pod.Status.ContainerStatuses...) {
		if status.State.Waiting != nil && retryablePodReason(status.State.Waiting.Reason) {
			return nil
		}
		if status.State.Terminated != nil && status.State.Terminated.ExitCode != 0 && retryablePodReason(status.State.Terminated.Reason) {
			return nil
		}
	}
	if retryablePodReason(pod.Status.Reason) {
		return nil
	}
	return fmt.Errorf("Pod 尚无明确的可重试错误，请先检查事件和容器状态")
}

// PendingPodImages follows init-container order so a manual pull targets the
// image currently preventing the Pod from advancing.
func PendingPodImages(pod *corev1.Pod) []string {
	statuses := make(map[string]corev1.ContainerStatus, len(pod.Status.InitContainerStatuses))
	for _, status := range pod.Status.InitContainerStatuses {
		statuses[status.Name] = status
	}
	for _, container := range pod.Spec.InitContainers {
		status := statuses[container.Name]
		if status.State.Terminated != nil && status.State.Terminated.ExitCode == 0 {
			continue
		}
		if status.ImageID == "" && container.Image != "" {
			return []string{container.Image}
		}
		return nil
	}
	seen := map[string]bool{}
	var images []string
	for _, container := range pod.Spec.Containers {
		if container.Image == "" || seen[container.Image] {
			continue
		}
		for _, status := range pod.Status.ContainerStatuses {
			if status.Name == container.Name && status.ImageID == "" {
				images = append(images, container.Image)
				seen[container.Image] = true
				break
			}
		}
	}
	return images
}
