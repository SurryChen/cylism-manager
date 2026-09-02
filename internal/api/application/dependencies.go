package applicationapi

import "github.com/cylism/cylism-manager/internal/k8s"

// K8s is the Kubernetes client supplied by the API composition layer.
// It is retained here as a package dependency during the physical handler
// migration; handlers do not construct clients themselves.
var K8s *k8s.Client
