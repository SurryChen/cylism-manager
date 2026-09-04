package bootstrap

import (
	"context"
	api "github.com/cylism/cylism-manager/internal/api"
	agentapi "github.com/cylism/cylism-manager/internal/api/agent"
	applicationapi "github.com/cylism/cylism-manager/internal/api/application"
	authapi "github.com/cylism/cylism-manager/internal/api/auth"
	deliveryapi "github.com/cylism/cylism-manager/internal/api/delivery"
	clusterapi "github.com/cylism/cylism-manager/internal/api/infrastructure/cluster"
	kubernetesapi "github.com/cylism/cylism-manager/internal/api/infrastructure/kubernetes"
	networkapi "github.com/cylism/cylism-manager/internal/api/infrastructure/network"
	storageapi "github.com/cylism/cylism-manager/internal/api/infrastructure/storage"
	runtimeapi "github.com/cylism/cylism-manager/internal/api/runtime"
	apisShared "github.com/cylism/cylism-manager/internal/api/shared"
	systemapi "github.com/cylism/cylism-manager/internal/api/system"
	"github.com/cylism/cylism-manager/internal/model"
	runtimepkg "github.com/cylism/cylism-manager/internal/runtime"
	runtimeidentity "github.com/cylism/cylism-manager/internal/runtime/identity"
	maintenance "github.com/cylism/cylism-manager/internal/service/maintenance"
	alertingservice "github.com/cylism/cylism-manager/internal/service/observability/alerting"
	monitoringservice "github.com/cylism/cylism-manager/internal/service/observability/monitoring"
	registryservice "github.com/cylism/cylism-manager/internal/service/registry"
	"github.com/cylism/cylism-manager/internal/transport"
	"time"
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
	maintenanceSSH := maintenance.SSHExecutorFunc(func(ctx context.Context, timeout time.Duration, server *model.Server, command string) ([]byte, error) {
		return transport.SSHExecContext(ctx, timeout, append(transport.BuildSSHArgs(server, key, server.Host), command))
	})
	diskInspection := maintenance.NewDiskInspectionService(maintenanceSSH)
	cleanup := maintenance.NewCleanupService(maintenanceSSH)
	registrySSH := registryservice.SSHExecutorFunc(maintenanceSSH)
	registryVerifier := registryservice.NewNodeVerifierService(registrySSH)
	registryPull := registryservice.NewNodePullService(registrySSH)
	nodeMirrorApplier := registryservice.NewNodeMirrorApplier(registrySSH)
	agent := agentapi.NewAgentHandlerWithKubernetesAdapter(c.Store, agentAdapter, authenticator).WithMonitoringDiskGrowth(monitoring).WithRegistryVerifier(registryVerifier).WithMaintenanceInspector(diskInspection)
	agentOp := agentapi.NewAgentOperationHandlerWithKubernetesAdapter(c.Store, agentAdapter).WithRegistryPullExecutor(registryPull).WithMaintenanceCleanupExecutor(cleanup)
	runtimeHandler := runtimeapi.NewRuntimeHandlerWithDependencies(c.Store, key, c.Services.RuntimeManager, c.Services.RuntimeRegistry)
	platform := deliveryapi.NewPlatformHandlerWithDependencies(c.Store, deliveryapi.NewPlatformKubernetesAdapter(c.K8s), c.Services.PlatformRelease)
	networkService := c.Services.Network
	clusterService := c.Services.Cluster
	clusterDNS := networkapi.NewClusterDNSHandlerWithAdapter(c.Store, networkapi.NewClusterDNSAdapter(c.K8s))
	ingress := networkapi.NewIngressHandler(networkService)
	nodeJoin := clusterapi.NewNodeJoinProgressHandler(c.Store, key, clusterapi.NewNodeJoinAdapter(c.K8s))
	networkHandler := networkapi.NewNetworkHandler(networkService, networkapi.NetworkHandler{
		DNSStatus: clusterDNS.Status, DNSApply: clusterDNS.Apply, DNSReset: clusterDNS.Reset, DNSRollback: clusterDNS.Rollback,
	})
	monitoringDeps := c.Adapters.Monitoring
	monitoringDeps.QueryService = c.Services.Monitoring
	monitoringDeps.ComponentService = c.Services.MonitoringComponent
	monitoringHandler := systemapi.NewMonitoringHandlerWithComposedDependencies(monitoringDeps)
	alertingDeps := c.Adapters.Alerting
	alertingDeps.ComponentService = c.Services.AlertingComponent
	alertingDeps.QueryService = c.Services.AlertingQuery
	dispatcher := alertingservice.NewRuntimeDispatcher(c.Store, key, runtimepkg.BuiltinRegistry())
	alertingHandler := systemapi.NewAlertingHandlerWithComposedDependencies(c.Auth.PlatformURL, alertingDeps, c.Store, c.Services.AlertingAutomation.WithDispatcher(dispatcher), dispatcher)
	loggingDeps := c.Adapters.Logging
	loggingDeps.QueryService = c.Services.LoggingQuery
	loggingDeps.ComponentService = c.Services.LoggingComponent
	loggingHandler := systemapi.NewLoggingHandlerWithComposedDependencies(c.Store, loggingDeps)
	// Integration delegations are verified by the route middleware with JWTSecret;
	// keep the handler's signing key aligned with that verifier.
	applicationHandler := applicationapi.NewApplicationHandlerWithDependencies(c.Store, c.Services.ApplicationQuery, key, applicationapi.NewKubernetesAdapter(c.K8s)).WithDelegationSecret(c.Auth.JWTSecret)
	image := deliveryapi.NewImageRegistryHandler(c.Store, key)
	nodeMirrors := deliveryapi.NewNodeRegistryMirrorHandlerWithDependencies(key, nodeMirrorApplier.Apply, c.Services.RegistryMirror)
	managed := deliveryapi.NewManagedOCIRegistryHandlerWithDependencies(c.Store, c.Adapters.Registry.ManagedResources, c.Adapters.Registry.ManagedStatus, nodeMirrorApplier.Apply, c.Services.RegistryManaged)
	proxy := deliveryapi.NewRegistryProxyHandlerWithDependencies(c.Store, key, c.Adapters.Registry.ProxyResources, c.Adapters.Registry.ProxyDiagnostics, c.Services.RegistryProxy).WithReconciler(c.Services.RegistryProxyReconciler)
	storageService := c.Services.Storage
	pvcAdapter, pvcMigration, pvcWorkloads := storageapi.NewPVCAdapters(c.K8s)
	storageHandler := storageapi.NewStorageHandlerWithDependencies(storageService, c.Store, key, pvcAdapter, pvcMigration, pvcWorkloads)
	storageHandler.ConfigureStorageExecutor()
	return api.RouteDependencies{
		Auth: api.AuthDependencies{
			Config:  c.Auth,
			Audit:   apisShared.AuditMiddleware(c.Store),
			Handler: authapi.NewAuthHandlerWithDependencies(c.Repositories.Users, c.Auth, c.Services.AuthTemporaryTokens),
		},
		Application: api.ApplicationDependencies{Handler: applicationHandler},
		RuntimeAgent: api.RuntimeAgentDependencies{
			Artifact: artifact, Agent: agent, Runtime: runtimeHandler, AgentOp: agentOp,
			Components: c.componentHandler(), Network: networkHandler,
		},
		Delivery: api.DeliveryDependencies{
			Platform: platform, Image: image, NodeMirrors: nodeMirrors, Managed: managed,
			Proxy: proxy, Chart: deliveryapi.NewChartRepositoryHandler(c.Store),
		},
		Infrastructure: api.InfrastructureDependencies{
			Network: networkHandler, Server: clusterapi.NewServerHandler(key, clusterService),
			NetworkDiag: clusterapi.NewServerNetworkDiagnosticsHandler(c.Store, key),
			Terminal:    clusterapi.NewServerTerminalHandler(c.Store, key),
			Site:        networkapi.NewSiteHandler(c.Store), Operation: systemapi.NewOperationHandler(c.Store),
			Domain: networkapi.NewDomainHandlerWithDependencies(networkapi.NewDomainKubernetesAdapter(c.K8s), networkService),
			Node:   clusterapi.NewNodeHandler(clusterService), NodeJoin: nodeJoin, Ingress: ingress,
			Certificate: networkapi.NewCertHandlerWithComposedDependencies(c.Store, key, networkService, networkapi.NewCertificateKubernetesAdapter(c.K8s)),
			K8s:         kubernetesapi.NewK8sHandlerWithAdapterAndEncryption(c.Store, storageService, key, kubernetesapi.NewK8sResourceAdapter(c.K8s), c.Store),
			Storage:     storageHandler, Tailscale: systemapi.NewTailscaleHandler(c.Store, key),
			CRD: kubernetesapi.NewCRDHandler(), AuditLog: systemapi.NewAuditHandler(c.Store), DBAdmin: systemapi.NewDBAdminHandler(c.Store),
		},
		System: api.SystemDependencies{
			Dashboard: systemapi.NewDashboardHandler(c.Store), Monitoring: monitoringHandler,
			Alerting: alertingHandler, Logging: loggingHandler,
		},
	}
}
