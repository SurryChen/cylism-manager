package bootstrap

import (
	api "github.com/cylism/cylism-manager/internal/api"
	systemapi "github.com/cylism/cylism-manager/internal/api/system"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/service/cluster"
	alertingservice "github.com/cylism/cylism-manager/internal/service/observability/alerting"
	loggingservice "github.com/cylism/cylism-manager/internal/service/observability/logging"
	monitoringservice "github.com/cylism/cylism-manager/internal/service/observability/monitoring"
)

// KubernetesAdapters contains the narrow Kubernetes capabilities assembled by
// the composition root. Services receive only the interfaces they declare.
// Additional domain adapters are added here as their migrations progress.
type KubernetesAdapters struct {
	Nodes           cluster.NodeAdapter
	SystemComponent systemapi.SystemComponentAdapter
	Monitoring      systemapi.MonitoringDependencies
	Logging         systemapi.LoggingDependencies
	Alerting        systemapi.AlertingDependencies
}

// NewKubernetesClient creates the Kubernetes client used by all adapters.
func NewKubernetesClient() (*k8s.Client, error) { return k8s.NewClient() }

// BuildKubernetesAdapters creates all currently migrated Kubernetes adapters.
// A nil client means the process is running in degraded mode; callers receive
// nil optional adapters instead of having to read global API state.
func BuildKubernetesAdapters(client *k8s.Client) KubernetesAdapters {
	if client == nil {
		return KubernetesAdapters{}
	}
	monitoringClient := monitoringservice.HTTPClient{BaseURL: k8s.VictoriaMetricsServiceURL}
	loggingClient := loggingservice.HTTPClient{BaseURL: k8s.LokiServiceURL()}
	alertingClient := &alertingservice.HTTPClient{BaseURL: k8s.AlertmanagerServiceURL}
	return KubernetesAdapters{
		Nodes:           client,
		SystemComponent: k8s.SystemComponentKubernetesAdapter{Client: client},
		Monitoring: systemapi.MonitoringDependencies{
			Query:     monitoringClient.Query,
			Status:    k8s.VictoriaMetricsReadiness{Client: client},
			Component: k8s.MonitoringComponentAdapter{Client: client},
			Consumers: monitoringservice.PVCConsumerReader{Pods: k8s.PodReader{Clientset: client.Clientset}},
		},
		Logging: systemapi.LoggingDependencies{
			Query:        loggingClient.Query,
			Ready:        func() bool { return client.LoggingStatus().LokiReady >= 1 },
			Component:    k8s.LoggingComponentAdapter{Client: client},
			FilterReader: k8s.LoggingFilterReader{Clientset: client.Clientset},
		},
		Alerting: systemapi.AlertingDependencies{
			Alertmanager: alertingClient.Request,
			Component:    k8s.AlertingComponentAdapter{Client: client},
			Ready:        func() bool { return client.AlertingStatus().State == k8s.AlertingStateReady },
			Secrets:      k8s.SecretReader{Client: client},
			Sender:       alertingservice.DefaultNotificationSender{},
		},
	}
}

// RouterDependencies adapts bootstrap-owned wiring to the API route binder
// without exposing the composition container itself to the API package.
func (a KubernetesAdapters) RouterDependencies() api.KubernetesDependencies {
	return api.KubernetesDependencies{
		Nodes:           a.Nodes,
		SystemComponent: a.SystemComponent,
		Monitoring:      a.Monitoring,
		Logging:         a.Logging,
		Alerting:        a.Alerting,
	}
}
