package bootstrap

import "github.com/cylism/cylism-manager/internal/k8s"

// NewKubernetesClient creates the Kubernetes client used by all adapters.
func NewKubernetesClient() (*k8s.Client, error) { return k8s.NewClient() }
