package runtimeapi

import "github.com/cylism/cylism-manager/internal/k8s"

// K8s is supplied by the API composition layer for runtime views.
var K8s *k8s.Client
