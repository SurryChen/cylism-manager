package api

import (
	"github.com/cylism/cylism-manager/internal/api/system"
	"github.com/cylism/cylism-manager/internal/service/cluster"
)

// KubernetesDependencies contains already-assembled Kubernetes capabilities
// needed by the Router. The composition root creates these values; the API
// package only wires them into handlers.
type KubernetesDependencies struct {
	Nodes           cluster.NodeAdapter
	SystemComponent system.SystemComponentAdapter
	Monitoring      system.MonitoringDependencies
	Logging         system.LoggingDependencies
	Alerting        system.AlertingDependencies
}
