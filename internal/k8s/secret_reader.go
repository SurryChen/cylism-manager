package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretReader exposes the minimal Secret read boundary used by observability
// services. It keeps Kubernetes client assembly out of HTTP handlers.
type SecretReader struct{ Client *Client }

func (r SecretReader) Get(ctx context.Context, namespace, name string) (map[string][]byte, error) {
	if r.Client == nil || r.Client.Clientset == nil {
		return nil, fmt.Errorf("Kubernetes 客户端未初始化")
	}
	secret, err := r.Client.Clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return secret.Data, nil
}
