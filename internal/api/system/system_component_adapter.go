package system

import systemcomponentservice "github.com/cylism/cylism-manager/internal/service/system_component"

// SystemComponentAdapter is the service boundary used by the HTTP handler.
// The concrete Kubernetes implementation lives in internal/k8s.
type SystemComponentAdapter = systemcomponentservice.KubernetesAdapter
