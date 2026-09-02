package bootstrap

import (
	"context"
	deliveryapi "github.com/cylism/cylism-manager/internal/api/delivery"
	infrastructureapi "github.com/cylism/cylism-manager/internal/api/infrastructure"
	"github.com/cylism/cylism-manager/internal/k8s"
	runtimepkg "github.com/cylism/cylism-manager/internal/runtime"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	authservice "github.com/cylism/cylism-manager/internal/service/auth"
	"github.com/cylism/cylism-manager/internal/service/cluster"
	networkservice "github.com/cylism/cylism-manager/internal/service/network"
	alertingservice "github.com/cylism/cylism-manager/internal/service/observability/alerting"
	loggingservice "github.com/cylism/cylism-manager/internal/service/observability/logging"
	monitoringservice "github.com/cylism/cylism-manager/internal/service/observability/monitoring"
	platformservice "github.com/cylism/cylism-manager/internal/service/platform"
	registryservice "github.com/cylism/cylism-manager/internal/service/registry"
	storageservice "github.com/cylism/cylism-manager/internal/service/storage"
	systemcomponentservice "github.com/cylism/cylism-manager/internal/service/system_component"
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
	AlertingQuery       *alertingservice.QueryService
	AlertingAutomation  *alertingservice.AutomationService
	AuthTemporaryTokens *authservice.TemporaryTokenService
	RuntimeManager      *runtimepkg.KubernetesManager
	RuntimeRegistry     *runtimepkg.Registry
	SystemComponentList *systemcomponentservice.ComponentListService
	SystemComponent     *systemcomponentservice.ComponentService
}

func BuildServices(repos Repositories, client *k8s.Client, adapters KubernetesAdapters, encKey []byte) Services {
	clusterSvc := cluster.NewService(repos.Cluster, adapters.Nodes).
		WithServerInspector(infrastructureapi.ServerInspector{EncKey: encKey}).
		WithServerImporter(infrastructureapi.ServerInspector{EncKey: encKey}).
		WithMetricsInspector(infrastructureapi.ServerMetricsInspector{EncKey: encKey})
	networkSvc := networkservice.NewService(repos.Network, repos.DNSCredentials).
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
	if repos.Platform != nil {
		platformRelease = platformservice.NewReleaseService(repos.Platform, encKey, platformAdapter)
	}
	if repos.Mirror != nil {
		registryMirror = registryservice.NewMirrorService(repos.Mirror, encKey)
	}
	if repos.Managed != nil {
		registryManaged = registryservice.NewManagedRegistryService(repos.Managed, encKey)
	}
	if repos.Proxy != nil {
		registryProxy = registryservice.NewProxyService(repos.Proxy, encKey)
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
	var alertingAutomation *alertingservice.AutomationService
	if repos.Alerting != nil {
		alertingAutomation = alertingservice.NewAutomationService(repos.Alerting, nil)
	}
	var alertingQuery *alertingservice.QueryService
	if adapters.Alerting.Alertmanager != nil {
		alertingQuery = alertingservice.NewQueryService(alertingservice.NewClient(adapters.Alerting.Alertmanager), adapters.Alerting.Ready)
	}
	var authTemporaryTokens *authservice.TemporaryTokenService
	if repos.Users != nil && repos.TemporaryToken != nil {
		authTemporaryTokens = authservice.NewTemporaryTokenService(repos.Users, repos.TemporaryToken)
	}
	loggingClient := loggingservice.HTTPClient{BaseURL: k8s.LokiServiceURL()}
	var loggingReady func(context.Context) bool
	if adapters.Logging.Ready != nil {
		loggingReady = adapters.Logging.Ready
	}
	systemComponentList := &systemcomponentservice.ComponentListService{Repo: repos.SystemComponents, Adapter: adapters.SystemComponent}
	return Services{
		Cluster:             clusterSvc,
		Network:             networkSvc,
		Storage:             storageservice.NewService(adapters.Storage, repos.StorageEnv, repos.StorageRecords),
		Monitoring:          monitoringQuery,
		ApplicationQuery:    applicationservice.NewQueryService(repos.Application),
		PlatformRelease:     platformRelease,
		RegistryMirror:      registryMirror,
		RegistryManaged:     registryManaged,
		RegistryProxy:       registryProxy,
		MonitoringComponent: monitoringComponent,
		LoggingQuery:        loggingservice.NewQueryService(loggingClient.Query, loggingReady, repos.LoggingScope),
		LoggingComponent:    loggingComponent,
		AlertingComponent:   alertingComponent,
		AlertingQuery:       alertingQuery,
		AlertingAutomation:  alertingAutomation,
		AuthTemporaryTokens: authTemporaryTokens,
		RuntimeManager:      runtimepkg.NewKubernetesManager(client, runtimepkg.BuiltinRegistry()),
		RuntimeRegistry:     runtimepkg.BuiltinRegistry(),
		SystemComponentList: systemComponentList,
		SystemComponent:     systemcomponentservice.NewComponentService(repos.SystemComponents, adapters.SystemComponent, systemComponentList),
	}
}
