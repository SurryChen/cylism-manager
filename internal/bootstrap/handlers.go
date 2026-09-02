package bootstrap

import (
	"context"
	api "github.com/cylism/cylism-manager/internal/api"
	agentapi "github.com/cylism/cylism-manager/internal/api/agent"
	applicationapi "github.com/cylism/cylism-manager/internal/api/application"
	authapi "github.com/cylism/cylism-manager/internal/api/auth"
	deliveryapi "github.com/cylism/cylism-manager/internal/api/delivery"
	infrastructureapi "github.com/cylism/cylism-manager/internal/api/infrastructure"
	runtimeapi "github.com/cylism/cylism-manager/internal/api/runtime"
	systemapi "github.com/cylism/cylism-manager/internal/api/system"
	"github.com/cylism/cylism-manager/internal/model"
	runtimepkg "github.com/cylism/cylism-manager/internal/runtime"
	runtimeidentity "github.com/cylism/cylism-manager/internal/runtime/identity"
	monitoringservice "github.com/cylism/cylism-manager/internal/service/observability/monitoring"
)

// BuildRouteDependencies constructs every HTTP dependency exactly once. The
// API router only binds the resulting handlers and never creates services or
// adapters itself.
func (c *Container) BuildRouteDependencies() api.RouteDependencies {
	key := c.configEncryptionKey()
	authenticator := runtimeidentity.NewRuntimeTokenAuthorizer(c.K8s, c.Store, nil)
	artifact := agentapi.NewAgentArtifactHandler("/usr/local/lib/cylism/runtime-tools", authenticator)
	var monitoring *monitoringservice.AgentDiskGrowthService
	if c.Adapters.Monitoring.Query != nil {
		monitoring = monitoringservice.NewAgentDiskGrowthService(c.Services.Monitoring, nil)
	}
	agentAdapter := agentapi.NewKubernetesAdapter(c.K8s)
	agent := agentapi.NewAgentHandler(c.Store, agentAdapter, authenticator).WithMonitoringDiskGrowth(monitoring).WithRegistryVerifier(agentapi.DefaultAgentRegistryNodeVerifier(key)).WithMaintenanceInspector(agentapi.DefaultAgentMaintenanceInspector(key))
	agentOp := agentapi.NewAgentOperationHandler(c.Store, agentAdapter).WithRegistryPullExecutor(agentapi.DefaultAgentRegistryPullExecutor(key)).WithMaintenanceCleanupExecutor(agentapi.DefaultAgentMaintenanceCleanupExecutor(key))
	runtimeHandler := runtimeapi.NewRuntimeHandler(c.Store, key, c.Services.RuntimeManager, c.Services.RuntimeRegistry)
	platform := deliveryapi.NewPlatformHandlerWithService(c.Store, key, deliveryapi.NewPlatformKubernetesAdapter(c.K8s), c.Services.PlatformRelease)
	networkService := c.Services.Network
	clusterService := c.Services.Cluster
	clusterDNS := infrastructureapi.NewClusterDNSHandler(c.Store, infrastructureapi.NewClusterDNSAdapter(c.K8s))
	ingress := infrastructureapi.NewIngressHandler(networkService)
	nodeJoin := infrastructureapi.NewNodeJoinProgressHandler(c.Store, key, infrastructureapi.NewNodeJoinAdapter(c.K8s))
	networkHandler := infrastructureapi.NewNetworkHandler(networkService, infrastructureapi.NetworkHandler{
		DNSStatus: clusterDNS.Status, DNSApply: clusterDNS.Apply, DNSReset: clusterDNS.Reset, DNSRollback: clusterDNS.Rollback,
	})
	monitoringDeps := c.Adapters.Monitoring
	monitoringDeps.QueryService = c.Services.Monitoring
	monitoringDeps.ComponentService = c.Services.MonitoringComponent
	monitoringHandler := systemapi.NewMonitoringHandler(monitoringDeps)
	alertingDeps := c.Adapters.Alerting
	alertingDeps.ComponentService = c.Services.AlertingComponent
	alertingDeps.QueryService = c.Services.AlertingQuery
	dispatcher := systemapi.NewAlertRuntimeDispatcher(c.Store, key, runtimepkg.BuiltinRegistry())
	alertingHandler := systemapi.NewAlertingHandler(c.Auth.PlatformURL).WithDependencies(alertingDeps).
		WithAutomationService(c.Store, c.Services.AlertingAutomation.WithDispatcher(dispatcher), dispatcher)
	loggingDeps := c.Adapters.Logging
	loggingDeps.QueryService = c.Services.LoggingQuery
	loggingDeps.ComponentService = c.Services.LoggingComponent
	loggingHandler := systemapi.NewLoggingHandler(c.Store, loggingDeps)
	applicationHandler := applicationapi.NewApplicationHandlerWithQuery(c.Store, c.Services.ApplicationQuery, key, applicationapi.NewKubernetesDependencies(c.K8s))
	image := deliveryapi.NewImageRegistryHandler(c.Store, key)
	nodeMirrors := deliveryapi.NewNodeRegistryMirrorHandlerWithService(c.Store, key, func(ctx context.Context, server *model.Server, content []byte) (string, string) {
		return deliveryapi.ApplyK3sRegistriesToNode(ctx, server, key, content)
	}, c.Services.RegistryMirror)
	managed := deliveryapi.NewManagedOCIRegistryHandlerWithService(c.Store, key, c.Adapters.Registry.ManagedResources, c.Adapters.Registry.ManagedStatus, func(ctx context.Context, server *model.Server, content []byte) (string, string) {
		return deliveryapi.ApplyK3sRegistriesToNode(ctx, server, key, content)
	}, c.Services.RegistryManaged)
	proxy := deliveryapi.NewRegistryProxyHandlerWithService(c.Store, key, c.Adapters.Registry.ProxyResources, c.Adapters.Registry.ProxyDiagnostics, c.Services.RegistryProxy)
	storageService := c.Services.Storage
	pvcAdapter, pvcMigration, pvcWorkloads := infrastructureapi.NewPVCAdapters(c.K8s)
	storageHandler := infrastructureapi.NewStorageHandlerWithDependencies(storageService, c.Store, key, pvcAdapter, pvcMigration, pvcWorkloads)
	storageHandler.ConfigureStorageExecutor()
	return api.RouteDependencies{
		Store: c.Store, AuthConfig: c.Auth, Audit: systemapi.AuditMiddleware(c.Store),
		Artifact: artifact, Agent: agent, Auth: authapi.NewAuthHandlerWithTemporaryService(c.Repositories.Users, c.Auth.JWTSecret, c.Auth.AccessTokenTTL, c.Auth.RefreshTokenTTL, c.Services.AuthTemporaryTokens),
		Runtime: runtimeHandler, AgentOp: agentOp, Components: c.componentHandler(), Network: networkHandler,
		Dashboard: systemapi.NewDashboardHandler(c.Store), Platform: platform, Image: image, NodeMirrors: nodeMirrors, Managed: managed, Proxy: proxy, Chart: deliveryapi.NewChartRepositoryHandler(c.Store),
		Monitoring: monitoringHandler, Alerting: alertingHandler, Logging: loggingHandler, Application: applicationHandler,
		Server: infrastructureapi.NewServerHandler(key, clusterService), NetworkDiag: infrastructureapi.NewServerNetworkDiagnosticsHandler(c.Store, key), Terminal: infrastructureapi.NewServerTerminalHandler(c.Store, key),
		Site: infrastructureapi.NewSiteHandler(c.Store), Operation: systemapi.NewOperationHandler(c.Store), Domain: infrastructureapi.NewDomainHandlerWithService(c.Store, infrastructureapi.NewDomainKubernetesAdapter(c.K8s), networkService), Node: infrastructureapi.NewNodeHandler(clusterService),
		NodeJoin: nodeJoin, Ingress: ingress, Certificate: infrastructureapi.NewCertHandlerWithDependencies(c.Store, key, networkService, infrastructureapi.NewCertificateKubernetesAdapter(c.K8s)),
		K8s: infrastructureapi.NewK8sHandlerWithAdapterAndEncryption(c.Store, storageService, key, infrastructureapi.NewK8sResourceAdapter(c.K8s), c.Store), Storage: storageHandler, Tailscale: systemapi.NewTailscaleHandler(c.Store, key), CRD: infrastructureapi.NewCRDHandler(), AuditLog: systemapi.NewAuditHandler(c.Store), DBAdmin: systemapi.NewDBAdminHandler(c.Store),
	}
}
