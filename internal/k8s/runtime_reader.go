package k8s

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
