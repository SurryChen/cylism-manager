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
	alertingservice "github.com/cylism/cylism-manager/internal/service/observability/alerting"
	loggingservice "github.com/cylism/cylism-manager/internal/service/observability/logging"
	monitoringservice "github.com/cylism/cylism-manager/internal/service/observability/monitoring"
	storageservice "github.com/cylism/cylism-manager/internal/service/storage"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册所有 API 路由
func RegisterRoutes(r *gin.Engine, s *store.Store, encKey []byte, authCfg *authapi.AuthConfig, k8sClient *k8s.Client) {
	applicationapi.K8s = k8sClient
	runtimeapi.K8s = k8sClient
	// This endpoint is authenticated with a projected Runtime installer token,
	// never with a browser JWT or the Runtime chat credential.
	artifactHandler := agentapi.NewAgentArtifactHandler("/usr/local/lib/cylism/runtime-tools", runtimeidentity.NewRuntimeTokenAuthorizer(k8sClient, s, nil))
	var agentMonitoring *monitoringservice.AgentDiskGrowthService
	if k8sClient != nil {
		monitoringClient := monitoringservice.HTTPClient{BaseURL: k8s.VictoriaMetricsServiceURL}
		query := monitoringservice.NewQueryService(monitoringClient.Query, k8s.VictoriaMetricsReadiness{Client: k8sClient})
		agentMonitoring = monitoringservice.NewAgentDiskGrowthService(query, nil)
	}
	agentHandler := agentapi.NewAgentHandler(s, k8sClient, runtimeidentity.NewRuntimeTokenAuthorizer(k8sClient, s, nil)).WithMonitoringDiskGrowth(agentMonitoring).WithRegistryVerifier(agentapi.DefaultAgentRegistryNodeVerifier(encKey)).WithMaintenanceInspector(agentapi.DefaultAgentMaintenanceInspector(encKey))
	registerPublicRoutes(r, artifactHandler, agentHandler)

	// 认证路由
	authHandler := authapi.NewAuthHandler(s, authCfg.JWTSecret, authCfg.AccessTokenTTL, authCfg.RefreshTokenTTL)
	registerAuthRoutes(r, authHandler, authCfg)

	// 业务 API（受 JWT 保护）
	apiGroup := r.Group("/api")
	apiGroup.Use(authapi.JWTAuthMiddleware(authCfg.JWTSecret))
	apiGroup.Use(systemapi.AuditMiddleware(s))
	runtimeRegistry := runtimepkg.BuiltinRegistry()
	runtimeHandler := runtimeapi.NewRuntimeHandler(s, encKey, runtimepkg.NewKubernetesManager(k8sClient, runtimeRegistry), runtimeRegistry)
	agentOperationHandler := agentapi.NewAgentOperationHandler(s, k8sClient).WithRegistryPullExecutor(agentapi.DefaultAgentRegistryPullExecutor(encKey)).WithMaintenanceCleanupExecutor(agentapi.DefaultAgentMaintenanceCleanupExecutor(encKey))
	var systemComponentAdapter systemapi.SystemComponentAdapter
	if k8sClient != nil {
		systemComponentAdapter = k8s.SystemComponentKubernetesAdapter{Client: k8sClient}
	}
	systemComponentHandler := systemapi.NewSystemComponentHandler(s, systemComponentAdapter)
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

	clusterService := cluster.NewService(s, clusterNodeAdapter(k8sClient)).WithServerInspector(infrastructureapi.ServerInspector{EncKey: encKey}).WithServerImporter(infrastructureapi.ServerInspector{EncKey: encKey}).WithMetricsInspector(infrastructureapi.ServerMetricsInspector{EncKey: encKey})
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
	var monitoringDeps systemapi.MonitoringDependencies
	if k8sClient != nil {
		monitoringClient := monitoringservice.HTTPClient{BaseURL: k8s.VictoriaMetricsServiceURL}
		monitoringDeps = systemapi.MonitoringDependencies{
			Query: monitoringClient.Query, Status: k8s.VictoriaMetricsReadiness{Client: k8sClient}, Component: k8s.MonitoringComponentAdapter{Client: k8sClient}, Consumers: monitoringservice.PVCConsumerReader{Pods: k8s.PodReader{Clientset: k8sClient.Clientset}},
		}
	}
	monitoringHandler := systemapi.NewMonitoringHandler(monitoringDeps)
	registerMonitoringRoutes(apiGroup, monitoringHandler)
	var alertingDeps systemapi.AlertingDependencies
	if k8sClient != nil {
		alertingClient := &alertingservice.HTTPClient{BaseURL: k8s.AlertmanagerServiceURL}
		alertingDeps = systemapi.AlertingDependencies{
			Alertmanager: alertingClient.Request,
			Component:    k8s.AlertingComponentAdapter{Client: k8sClient},
			Ready:        func() bool { return k8sClient.AlertingStatus().State == k8s.AlertingStateReady },
			Secrets:      k8s.SecretReader{Client: k8sClient},
			Sender:       alertingservice.DefaultNotificationSender{},
		}
	}
	alertingHandler := systemapi.NewAlertingHandler(authCfg.PlatformURL).
		WithDependencies(alertingDeps).
		WithAutomation(s, systemapi.NewAlertRuntimeDispatcher(s, encKey, runtimeRegistry))
	var loggingDeps systemapi.LoggingDependencies
	if k8sClient != nil {
		loggingClient := loggingservice.HTTPClient{BaseURL: k8s.LokiServiceURL()}
		loggingDeps = systemapi.LoggingDependencies{
			Query:        loggingClient.Query,
			Ready:        func() bool { return k8sClient.LoggingStatus().LokiReady >= 1 },
			Component:    k8s.LoggingComponentAdapter{Client: k8sClient},
			FilterReader: k8s.LoggingFilterReader{Clientset: k8sClient.Clientset},
		}
	}
	loggingHandler := systemapi.NewLoggingHandler(s, loggingDeps)
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
