package api

import (
	agentapi "github.com/cylism/cylism-manager/internal/api/agent"
	applicationapi "github.com/cylism/cylism-manager/internal/api/application"
	authapi "github.com/cylism/cylism-manager/internal/api/auth"
	"github.com/cylism/cylism-manager/internal/api/delivery"
	infrastructureapi "github.com/cylism/cylism-manager/internal/api/infrastructure"
	runtimeapi "github.com/cylism/cylism-manager/internal/api/runtime"
	systemapi "github.com/cylism/cylism-manager/internal/api/system"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	runtimepkg "github.com/cylism/cylism-manager/internal/runtime"
	runtimeidentity "github.com/cylism/cylism-manager/internal/runtime/identity"
	"github.com/cylism/cylism-manager/internal/service/cluster"
	networkservice "github.com/cylism/cylism-manager/internal/service/network"
	monitoringservice "github.com/cylism/cylism-manager/internal/service/observability/monitoring"
	storageservice "github.com/cylism/cylism-manager/internal/service/storage"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册所有 API 路由
func RegisterRoutes(r *gin.Engine, s *store.Store, encKey []byte, authCfg *authapi.AuthConfig, k8sClient *k8s.Client, adapters KubernetesDependencies) {
	applicationapi.K8s = k8sClient
	runtimeapi.K8s = k8sClient
	// This endpoint is authenticated with a projected Runtime installer token,
	// never with a browser JWT or the Runtime chat credential.
	artifactHandler := agentapi.NewAgentArtifactHandler("/usr/local/lib/cylism/runtime-tools", runtimeidentity.NewRuntimeTokenAuthorizer(k8sClient, s, nil))
	var agentMonitoring *monitoringservice.AgentDiskGrowthService
	if adapters.Monitoring.Query != nil {
		query := monitoringservice.NewQueryService(adapters.Monitoring.Query, adapters.Monitoring.Status)
		agentMonitoring = monitoringservice.NewAgentDiskGrowthService(query, nil)
	}
	agentHandler := agentapi.NewAgentHandler(s, k8sClient, runtimeidentity.NewRuntimeTokenAuthorizer(k8sClient, s, nil)).WithMonitoringDiskGrowth(agentMonitoring).WithRegistryVerifier(agentapi.DefaultAgentRegistryNodeVerifier(encKey)).WithMaintenanceInspector(agentapi.DefaultAgentMaintenanceInspector(encKey))
	registerPublicRoutes(r, artifactHandler, agentHandler)

	// 认证路由
	authHandler := authapi.NewAuthHandler(s, authCfg.JWTSecret, authCfg.AccessTokenTTL, authCfg.RefreshTokenTTL, s)
	registerAuthRoutes(r, authHandler, authCfg)

	// 业务 API（受 JWT 保护）
	apiGroup := r.Group("/api")
	apiGroup.Use(authapi.JWTAuthMiddleware(authCfg.JWTSecret))
	apiGroup.Use(systemapi.AuditMiddleware(s))
	runtimeRegistry := runtimepkg.BuiltinRegistry()
	runtimeHandler := runtimeapi.NewRuntimeHandler(s, encKey, runtimepkg.NewKubernetesManager(k8sClient, runtimeRegistry), runtimeRegistry)
	agentOperationHandler := agentapi.NewAgentOperationHandler(s, k8sClient).WithRegistryPullExecutor(agentapi.DefaultAgentRegistryPullExecutor(encKey)).WithMaintenanceCleanupExecutor(agentapi.DefaultAgentMaintenanceCleanupExecutor(encKey))
	systemComponentHandler := systemapi.NewSystemComponentHandler(s, adapters.SystemComponent)
	networkService := networkservice.NewService(s, s).WithIngressAdapter(k8sClient).WithStandardIngressAdapter(k8sClient).WithDNSAdapter(k8sClient).WithCertificateAdapter(k8sClient)
	clusterDNSHandler := infrastructureapi.NewClusterDNSHandler(s, k8sClient)
	networkHandler := infrastructureapi.NewNetworkHandler(networkService, infrastructureapi.NetworkHandler{
		DNSStatus: clusterDNSHandler.Status, DNSApply: clusterDNSHandler.Apply,
		DNSReset: clusterDNSHandler.Reset, DNSRollback: clusterDNSHandler.Rollback,
	})
	registerRuntimeRoutes(apiGroup, runtimeHandler, agentOperationHandler, systemComponentHandler, networkHandler)
	platformHandler := delivery.NewPlatformHandler(s, encKey, k8sClient)
	go platformHandler.Reconcile()

	dashHandler := systemapi.NewDashboardHandler(s)
	registerDashboardRoutes(apiGroup, dashHandler)

	clusterService := cluster.NewService(s, adapters.Nodes).WithServerInspector(infrastructureapi.ServerInspector{EncKey: encKey}).WithServerImporter(infrastructureapi.ServerInspector{EncKey: encKey}).WithMetricsInspector(infrastructureapi.ServerMetricsInspector{EncKey: encKey})
	serverHandler := infrastructureapi.NewServerHandler(encKey, clusterService)
	serverNetworkDiagnostics := infrastructureapi.NewServerNetworkDiagnosticsHandler(s, encKey)
	serverTerminal := infrastructureapi.NewServerTerminalHandler(s, encKey)

	siteHandler := infrastructureapi.NewSiteHandler(s)

	operationHandler := systemapi.NewOperationHandler(s)

	applicationHandler := applicationapi.NewApplicationHandler(s, encKey).WithDelegationSecret(authCfg.JWTSecret)
	imageRegistryHandler := delivery.NewImageRegistryHandler(s, encKey)
	nodeMirrorHandler := delivery.NewNodeRegistryMirrorHandler(s, encKey, func(server *model.Server, content []byte) (string, string) {
		return delivery.ApplyK3sRegistriesToNode(server, encKey, content)
	})
	managedRegistryHandler := delivery.NewManagedOCIRegistryHandler(s, encKey, k8sClient, func(server *model.Server, content []byte) (string, string) {
		return delivery.ApplyK3sRegistriesToNode(server, encKey, content)
	})
	registryProxyHandler := delivery.NewRegistryProxyHandler(s, encKey, k8sClient)
	go registryProxyHandler.Reconcile()
	chartHandler := delivery.NewChartRepositoryHandler(s)
	registerDeliveryRoutes(r, apiGroup, deliveryRouteHandlers{image: imageRegistryHandler, nodeMirrors: nodeMirrorHandler, managed: managedRegistryHandler, proxy: registryProxyHandler, chart: chartHandler, platform: platformHandler})
	monitoringHandler := systemapi.NewMonitoringHandler(adapters.Monitoring)
	registerMonitoringRoutes(apiGroup, monitoringHandler)
	alertingHandler := systemapi.NewAlertingHandler(authCfg.PlatformURL).
		WithDependencies(adapters.Alerting).
		WithAutomation(s, systemapi.NewAlertRuntimeDispatcher(s, encKey, runtimeRegistry))
	loggingHandler := systemapi.NewLoggingHandler(s, adapters.Logging)
	registerLoggingRoutes(apiGroup, loggingHandler)
	registerAlertingRoutes(r, apiGroup, alertingHandler)
	domainHandler := infrastructureapi.NewDomainHandlerWithService(s, k8sClient, networkHandler.Service)
	registerApplicationRoutes(r, apiGroup, applicationHandler, authCfg.JWTSecret, systemapi.AuditMiddleware(s))

	dbAdminHandler := systemapi.NewDBAdminHandler(s)

	// K8s 节点管理
	nodeHandler := infrastructureapi.NewNodeHandler(clusterService)
	nodeJoinProgress := infrastructureapi.NewNodeJoinProgressHandler(s, encKey, k8sClient)

	// IngressRoute 管理
	ingressHandler := infrastructureapi.NewIngressHandler(k8sClient)

	// Certificate 管理
	certHandler := infrastructureapi.NewCertHandlerWithNetworkAndClient(s, encKey, networkService, k8sClient)

	// K8s 资源管理
	storageService := storageservice.NewService(k8sClient, s, s)
	k8sHandler := infrastructureapi.NewK8sHandlerWithEncryption(s, storageService, encKey, k8sClient, s)
	storageHandler := infrastructureapi.NewStorageHandlerWithClient(storageService, s, encKey, k8sClient)
	storageHandler.ConfigureStorageExecutor()

	// Tailscale 管理
	tailscaleHandler := systemapi.NewTailscaleHandler(s, encKey)
	crdHandler := infrastructureapi.NewCRDHandler()
	auditHandler := systemapi.NewAuditHandler(s)
	registerInfrastructureRoutes(apiGroup, serverHandler, serverNetworkDiagnostics, serverTerminal, siteHandler, operationHandler, domainHandler, nodeHandler, nodeJoinProgress, ingressHandler, certHandler, k8sHandler, storageHandler, networkHandler, tailscaleHandler, crdHandler, auditHandler, dbAdminHandler)
}
