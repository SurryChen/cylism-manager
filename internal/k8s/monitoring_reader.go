package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// PodReader exposes only the pod listing needed by monitoring diagnostics.
// Keeping this adapter in k8s prevents HTTP handlers from knowing how the
// Kubernetes client is assembled.
type PodReader struct {
	Clientset kubernetes.Interface
}

func (r PodReader) ListPods(ctx context.Context) ([]corev1.Pod, error) {
	if r.Clientset == nil {
		return nil, nil
	}
	list, err := r.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}
