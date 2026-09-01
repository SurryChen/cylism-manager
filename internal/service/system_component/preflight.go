package system_component

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

type NodePodReader interface {
	ListNodes(context.Context) ([]corev1.Node, error)
	ListPods(context.Context, string, string) ([]corev1.Pod, error)
}

func ValidateHAIncrease(ctx context.Context, reader NodePodReader, name, namespace string, currentReady, currentAvailable int32, requested int32, fixedNode string) error {
	if requested < 2 {
		return nil
	}
	if fixedNode != "" {
		return fmt.Errorf("%s 高可用副本不能固定在单个节点，请选择自动调度", name)
	}
	if currentReady < 1 || currentAvailable < 1 {
		return fmt.Errorf("%s 当前未健康，不允许在故障状态下增加副本", name)
	}
	nodes, err := reader.ListNodes(ctx)
	if err != nil {
		return fmt.Errorf("读取节点预检状态失败: %w", err)
	}
	candidates := 0
	for _, node := range nodes {
		if node.Spec.Unschedulable || !nodeReady(&node) || node.Status.Allocatable.Cpu().IsZero() || node.Status.Allocatable.Memory().IsZero() {
			continue
		}
		candidates++
	}
	if candidates < 2 {
		return fmt.Errorf("%s 高可用需要至少 2 个 Ready、可调度且具备 CPU/内存可分配的节点，当前仅 %d 个", name, candidates)
	}
	pods, err := reader.ListPods(ctx, namespace, "k8s-app="+name)
	if err != nil {
		return fmt.Errorf("读取 %s Pod 预检状态失败: %w", name, err)
	}
	for _, pod := range pods {
		for _, status := range pod.Status.ContainerStatuses {
			if status.State.Waiting != nil {
				switch status.State.Waiting.Reason {
				case "ErrImagePull", "ImagePullBackOff", "InvalidImageName":
					return fmt.Errorf("%s 存在镜像拉取失败，完成镜像源诊断后再增加副本", name)
				}
			}
		}
	}
	return nil
}

func nodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
