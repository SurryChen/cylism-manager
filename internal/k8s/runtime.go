package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RuntimePodUsage struct {
	CPUMilli    int64                   `json:"cpu_millicores"`
	MemoryBytes int64                   `json:"memory_bytes"`
	Containers  []RuntimeContainerUsage `json:"containers"`
}

type RuntimeContainerUsage struct {
	Name        string `json:"name"`
	CPUMilli    int64  `json:"cpu_millicores"`
	MemoryBytes int64  `json:"memory_bytes"`
}

func (c *Client) ListRuntimePodMetrics(ctx context.Context, namespace string) (map[string]RuntimePodUsage, error) {
	if c == nil || c.Clientset == nil || c.Config == nil {
		return nil, fmt.Errorf("Kubernetes metrics API 不可用")
	}
	raw, err := c.Clientset.CoreV1().RESTClient().Get().AbsPath("/apis/metrics.k8s.io/v1beta1/namespaces", namespace, "pods").Do(ctx).Raw()
	if err != nil {
		return nil, err
	}
	var response struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Containers []struct {
				Name  string            `json:"name"`
				Usage map[string]string `json:"usage"`
			} `json:"containers"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}
	usage := make(map[string]RuntimePodUsage, len(response.Items))
	for _, pod := range response.Items {
		item := RuntimePodUsage{}
		for _, container := range pod.Containers {
			containerUsage := RuntimeContainerUsage{Name: container.Name}
			cpu, cpuErr := resource.ParseQuantity(container.Usage["cpu"])
			memory, memoryErr := resource.ParseQuantity(container.Usage["memory"])
			if cpuErr == nil {
				containerUsage.CPUMilli = cpu.MilliValue()
				item.CPUMilli += containerUsage.CPUMilli
			}
			if memoryErr == nil {
				containerUsage.MemoryBytes = memory.Value()
				item.MemoryBytes += containerUsage.MemoryBytes
			}
			item.Containers = append(item.Containers, containerUsage)
		}
		usage[pod.Metadata.Name] = item
	}
	return usage, nil
}

func (c *Client) ReadRuntimePodLogs(ctx context.Context, namespace, pod, container string, tail int64, previous bool) (string, error) {
	if c == nil || c.Clientset == nil {
		return "", fmt.Errorf("Kubernetes 客户端未初始化")
	}
	limit := int64(256 * 1024)
	raw, err := c.Clientset.CoreV1().Pods(namespace).GetLogs(pod, &corev1.PodLogOptions{
		Container: container, TailLines: &tail, LimitBytes: &limit, Previous: previous, Timestamps: true,
	}).DoRaw(ctx)
	return string(raw), err
}

// The runtime reader methods form the read-only adapter consumed by
// service/application. Keeping the clientset access here prevents view
// services from depending on the full Kubernetes client wrapper.
func (c *Client) ListRuntimeServices(ctx context.Context, namespace string) ([]corev1.Service, error) {
	if c == nil || c.Clientset == nil {
		return nil, fmt.Errorf("Kubernetes 客户端未初始化")
	}
	items, err := c.Clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return items.Items, nil
}

func (c *Client) ListRuntimeDeployments(ctx context.Context, namespace string) ([]appsv1.Deployment, error) {
	if c == nil || c.Clientset == nil {
		return nil, fmt.Errorf("Kubernetes 客户端未初始化")
	}
	items, err := c.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return items.Items, nil
}

func (c *Client) ListRuntimeStatefulSets(ctx context.Context, namespace string) ([]appsv1.StatefulSet, error) {
	if c == nil || c.Clientset == nil {
		return nil, fmt.Errorf("Kubernetes 客户端未初始化")
	}
	items, err := c.Clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return items.Items, nil
}

func (c *Client) ListRuntimePods(ctx context.Context, namespace, selector string) ([]corev1.Pod, error) {
	if c == nil || c.Clientset == nil {
		return nil, fmt.Errorf("Kubernetes 客户端未初始化")
	}
	items, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	return items.Items, nil
}
