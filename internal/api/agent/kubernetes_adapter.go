package agent

import (
	"github.com/cylism/cylism-manager/internal/k8s"
	"k8s.io/client-go/kubernetes"
)

// KubernetesAdapter exposes only the typed client capability used by agent
// HTTP operations. Construction of the concrete client remains in bootstrap.
type KubernetesAdapter interface {
	KubernetesAvailable() bool
	Clientset() kubernetes.Interface
}

type clientAdapter struct{ client *k8s.Client }

// NewKubernetesAdapter adapts the concrete client at the composition root.
// Handlers consume only KubernetesAdapter.
func NewKubernetesAdapter(client *k8s.Client) KubernetesAdapter {
	if client == nil {
		return nil
	}
	return clientAdapter{client: client}
}

func (a clientAdapter) KubernetesAvailable() bool {
	return a.client != nil && a.client.Clientset != nil
}
func (a clientAdapter) Clientset() kubernetes.Interface {
	if a.client == nil {
		return nil
	}
	return a.client.Clientset
}
