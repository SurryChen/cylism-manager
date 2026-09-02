package bootstrap

import (
	"context"
	deliveryapi "github.com/cylism/cylism-manager/internal/api/delivery"
	infrastructureapi "github.com/cylism/cylism-manager/internal/api/infrastructure"
	"github.com/cylism/cylism-manager/internal/k8s"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/cylism/cylism-manager/internal/service/cluster"
	networkservice "github.com/cylism/cylism-manager/internal/service/network"
	alertingservice "github.com/cylism/cylism-manager/internal/service/observability/alerting"
	loggingservice "github.com/cylism/cylism-manager/internal/service/observability/logging"
	monitoringservice "github.com/cylism/cylism-manager/internal/service/observability/monitoring"
	platformservice "github.com/cylism/cylism-manager/internal/service/platform"
	registryservice "github.com/cylism/cylism-manager/internal/service/registry"
	storageservice "github.com/cylism/cylism-manager/internal/service/storage"
	"github.com/cylism/cylism-manager/internal/store"
)

// Services contains all application services assembled by the composition
// root. Handlers consume these instances instead of constructing services.
type Services struct {
	Cluster             *cluster.Service
	Network             *networkservice.Service
	Storage             *storageservice.Service
	Monitoring          *monitoringservice.QueryService
	ApplicationQuery    *applicationservice.QueryService
	PlatformRelease     *platformservice.ReleaseService
	RegistryMirror      *registryservice.MirrorService
	RegistryManaged     *registryservice.ManagedRegistryService
	RegistryProxy       *registryservice.ProxyService
	MonitoringComponent *monitoringservice.ComponentService
	LoggingQuery        *loggingservice.QueryService
	LoggingComponent    *loggingservice.ComponentService
	AlertingComponent   *alertingservice.ComponentService
}

func BuildServices(db *store.Store, client *k8s.Client, adapters KubernetesAdapters, encKey []byte) Services {
	clusterSvc := cluster.NewService(db, adapters.Nodes).
		WithServerInspector(infrastructureapi.ServerInspector{EncKey: encKey}).
		WithServerImporter(infrastructureapi.ServerInspector{EncKey: encKey}).
		WithMetricsInspector(infrastructureapi.ServerMetricsInspector{EncKey: encKey})
	networkSvc := networkservice.NewService(db, db).
		WithIngressAdapter(adapters.Network.Ingress).
		WithStandardIngressAdapter(adapters.Network.StandardIngress).
		WithDNSAdapter(adapters.Network.DNS).
		WithCertificateAdapter(adapters.Network.Certificate)
	monitoringQuery := monitoringservice.NewQueryService(adapters.Monitoring.Query, adapters.Monitoring.Status)
	platformAdapter := deliveryapi.NewPlatformKubernetesAdapter(client)
	var platformRelease *platformservice.ReleaseService
	var registryMirror *registryservice.MirrorService
	var registryManaged *registryservice.ManagedRegistryService
	var registryProxy *registryservice.ProxyService
	if db != nil {
		platformRelease = platformservice.NewReleaseService(db, encKey, platformAdapter)
		registryMirror = registryservice.NewMirrorService(db, encKey)
		registryManaged = registryservice.NewManagedRegistryService(db, encKey)
		registryProxy = registryservice.NewProxyService(db, encKey)
	}
	var monitoringComponent *monitoringservice.ComponentService
	if adapters.Monitoring.Component != nil {
		monitoringComponent = monitoringservice.NewComponentService(adapters.Monitoring.Component)
	}
	var loggingComponent *loggingservice.ComponentService
	if adapters.Logging.Component != nil {
		loggingComponent = loggingservice.NewComponentService(adapters.Logging.Component)
	}
	var alertingComponent *alertingservice.ComponentService
	if adapters.Alerting.Component != nil {
		alertingComponent = alertingservice.NewComponentService(adapters.Alerting.Component)
	}
	loggingClient := loggingservice.HTTPClient{BaseURL: k8s.LokiServiceURL()}
	var loggingReady func(context.Context) bool
	if adapters.Logging.Ready != nil {
		loggingReady = adapters.Logging.Ready
	}
	return Services{
		Cluster:             clusterSvc,
		Network:             networkSvc,
		Storage:             storageservice.NewService(adapters.Storage, db, db),
		Monitoring:          monitoringQuery,
		ApplicationQuery:    applicationservice.NewQueryService(db),
		PlatformRelease:     platformRelease,
		RegistryMirror:      registryMirror,
		RegistryManaged:     registryManaged,
		RegistryProxy:       registryProxy,
		MonitoringComponent: monitoringComponent,
		LoggingQuery:        loggingservice.NewQueryService(loggingClient.Query, loggingReady, db),
		LoggingComponent:    loggingComponent,
		AlertingComponent:   alertingComponent,
	}
}
