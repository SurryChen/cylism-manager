package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const ResizeAnnotation = "cylism.io/mysql-resource-task"

// Patch only the MySQL resource budget and its managed tuning. Keep volumes,
// credentials, image, replication filters and read-only startup flags intact.
func ResizeInstance(ctx context.Context, client *k8s.Client, item *model.DatabaseInstance, cpu, memory, revision string) error {
	if client == nil || client.Clientset == nil || item.Mode != "managed" {
		return fmt.Errorf("独立 MySQL 客户端不可用")
	}
	cpu, memory, err := ValidateMySQLResources(cpu, memory)
	if err != nil {
		return err
	}
	if cpu == "" {
		return fmt.Errorf("请设置资源上限")
	}
	sets := client.Clientset.AppsV1().StatefulSets(Namespace)
	set, err := sets.Get(ctx, ResourceName(item.ID), metav1.GetOptions{})
	if err != nil {
		return err
	}
	if set.Labels[k8s.ManagedByLabel] != k8s.ManagedByValue {
		return fmt.Errorf("实例工作负载不受平台管理")
	}
	if set.Spec.Replicas == nil || *set.Spec.Replicas != 1 {
		return fmt.Errorf("实例已停止或副本数异常")
	}
	if set.Spec.UpdateStrategy.Type == "OnDelete" || (set.Spec.UpdateStrategy.RollingUpdate != nil && set.Spec.UpdateStrategy.RollingUpdate.Partition != nil && *set.Spec.UpdateStrategy.RollingUpdate.Partition > 0) {
		return fmt.Errorf("实例更新策略不支持自动重启")
	}
	found := false
	for i := range set.Spec.Template.Spec.Containers {
		container := &set.Spec.Template.Spec.Containers[i]
		if container.Name != "mysql" {
			continue
		}
		found = true
		if container.Resources.Requests == nil {
			container.Resources.Requests = corev1.ResourceList{}
		}
		if container.Resources.Limits == nil {
			container.Resources.Limits = corev1.ResourceList{}
		}
		container.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(cpu)
		container.Resources.Limits[corev1.ResourceCPU] = resource.MustParse(cpu)
		container.Resources.Requests[corev1.ResourceMemory] = resource.MustParse(memory)
		container.Resources.Limits[corev1.ResourceMemory] = resource.MustParse(memory)
		managed := map[string]bool{}
		for _, key := range tunedOptions {
			managed[strings.ReplaceAll(key, "_", "-")] = true
		}
		args := []string{}
		for index := 0; index < len(container.Args); index++ {
			arg := container.Args[index]
			key, _, equal := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
			if managed[strings.ReplaceAll(key, "_", "-")] {
				if !equal && index+1 < len(container.Args) && !strings.HasPrefix(container.Args[index+1], "--") {
					index++
				}
				continue
			}
			args = append(args, arg)
		}
		for _, line := range strings.Split(EffectiveMyCNF("", memory), "\n") {
			if key, value, ok := strings.Cut(line, "="); ok {
				args = append(args, "--"+strings.ReplaceAll(key, "_", "-")+"="+value)
			}
		}
		container.Args = args
	}
	if !found {
		return fmt.Errorf("实例缺少 MySQL 容器")
	}
	if set.Spec.Template.Annotations == nil {
		set.Spec.Template.Annotations = map[string]string{}
	}
	set.Spec.Template.Annotations[ResizeAnnotation] = revision
	_, err = sets.Update(ctx, set, metav1.UpdateOptions{})
	return err
}

func InstanceResizeReady(ctx context.Context, client *k8s.Client, id uint, revision string) (bool, error) {
	set, err := client.Clientset.AppsV1().StatefulSets(Namespace).Get(ctx, ResourceName(id), metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	if set.Spec.Template.Annotations[ResizeAnnotation] != revision {
		return false, fmt.Errorf("资源调整版本发生变化")
	}
	if set.Status.ObservedGeneration < set.Generation || set.Status.ReadyReplicas != 1 || set.Status.UpdatedReplicas != 1 || set.Status.CurrentRevision == "" || set.Status.CurrentRevision != set.Status.UpdateRevision {
		return false, nil
	}
	pod, err := client.Clientset.CoreV1().Pods(Namespace).Get(ctx, ResourceName(id)+"-0", metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	if pod.DeletionTimestamp != nil || pod.Annotations[ResizeAnnotation] != revision || pod.Status.Phase != corev1.PodRunning {
		return false, nil
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == "mysql" && status.Ready {
			return true, nil
		}
	}
	return false, nil
}
