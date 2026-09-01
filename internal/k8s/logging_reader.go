package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// LoggingFilterReader exposes only the Kubernetes reads needed by log filters.
type LoggingFilterReader struct{ Clientset kubernetes.Interface }

func (r LoggingFilterReader) ListPods(ctx context.Context, namespace string) ([]corev1.Pod, error) {
	if r.Clientset == nil {
		return nil, nil
	}
	list, err := r.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (r LoggingFilterReader) ListNodes(ctx context.Context) ([]corev1.Node, error) {
	if r.Clientset == nil {
		return nil, nil
	}
	list, err := r.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}
