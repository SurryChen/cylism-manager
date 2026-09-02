package api

import (
	"time"

	agentapi "github.com/cylism/cylism-manager/internal/api/agent"
	"github.com/cylism/cylism-manager/internal/api/delivery"
	infrastructureapi "github.com/cylism/cylism-manager/internal/api/infrastructure"
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

// K8s K8s 客户端全局单例，main.go 初始化
var K8s *k8s.Client

// AuthConfig 认证相关配置
type AuthConfig struct {
	JWTSecret       []byte
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	AdminUser       string
	AdminPassword   string
	PlatformURL     string
}

// RegisterRoutes 注册所有 API 路由
func RegisterRoutes(r *gin.Engine, s *store.Store, encKey []byte, authCfg *AuthConfig) {
	// This endpoint is authenticated with a projected Runtime installer token,
	// never with a browser JWT or the Runtime chat credential.
	artifactHandler := agentapi.NewAgentArtifactHandler("/usr/local/lib/cylism/runtime-tools", runtimeidentity.NewRuntimeTokenAuthorizer(K8s, s, nil))
	var agentMonitoring *monitoringservice.AgentDiskGrowthService
	if K8s != nil {
		monitoringClient := monitoringservice.HTTPClient{BaseURL: k8s.VictoriaMetricsServiceURL}
		query := monitoringservice.NewQueryService(monitoringClient.Query, k8s.VictoriaMetricsReadiness{Client: K8s})
		agentMonitoring = monitoringservice.NewAgentDiskGrowthService(query, nil)
	}
	agentHandler := agentapi.NewAgentHandler(s, K8s, runtimeidentity.NewRuntimeTokenAuthorizer(K8s, s, nil)).WithMonitoringDiskGrowth(agentMonitoring).WithRegistryVerifier(agentapi.DefaultAgentRegistryNodeVerifier(encKey)).WithMaintenanceInspector(agentapi.DefaultAgentMaintenanceInspector(encKey))
	registerPublicRoutes(r, artifactHandler, agentHandler)

	// 认证路由
	authHandler := NewAuthHandler(s, authCfg.JWTSecret, authCfg.AccessTokenTTL, authCfg.RefreshTokenTTL)
	registerAuthRoutes(r, authHandler, authCfg)

	// 业务 API（受 JWT 保护）
	apiGroup := r.Group("/api")
	apiGroup.Use(JWTAuthMiddleware(authCfg.JWTSecret))
	apiGroup.Use(systemapi.AuditMiddleware(s))
	runtimeRegistry := runtimepkg.BuiltinRegistry()
	runtimeHandler := NewRuntimeHandler(s, encKey, runtimepkg.NewKubernetesManager(K8s, runtimeRegistry), runtimeRegistry)
	agentOperationHandler := agentapi.NewAgentOperationHandler(s, K8s).WithRegistryPullExecutor(agentapi.DefaultAgentRegistryPullExecutor(encKey)).WithMaintenanceCleanupExecutor(agentapi.DefaultAgentMaintenanceCleanupExecutor(encKey))
	var systemComponentAdapter systemapi.SystemComponentAdapter
	if K8s != nil {
		systemComponentAdapter = k8s.SystemComponentKubernetesAdapter{Client: K8s}
	}
	systemComponentHandler := systemapi.NewSystemComponentHandler(s, systemComponentAdapter)
	networkService := networkservice.NewService(s, s).WithIngressAdapter(K8s).WithStandardIngressAdapter(K8s).WithDNSAdapter(K8s).WithCertificateAdapter(K8s)
	clusterDNSHandler := infrastructureapi.NewClusterDNSHandler(s, K8s)
	networkHandler := infrastructureapi.NewNetworkHandler(networkService, infrastructureapi.NetworkHandler{
		DNSStatus: clusterDNSHandler.Status, DNSApply: clusterDNSHandler.Apply,
		DNSReset: clusterDNSHandler.Reset, DNSRollback: clusterDNSHandler.Rollback,
	})
	registerRuntimeRoutes(apiGroup, runtimeHandler, agentOperationHandler, systemComponentHandler, networkHandler)
	platformHandler := delivery.NewPlatformHandler(s, encKey, K8s)
	go platformHandler.Reconcile()

	dashHandler := NewDashboardHandler(s)
	registerDashboardRoutes(apiGroup, dashHandler)

	clusterService := cluster.NewService(s, clusterNodeAdapter()).WithServerInspector(infrastructureapi.ServerInspector{EncKey: encKey}).WithServerImporter(infrastructureapi.ServerInspector{EncKey: encKey}).WithMetricsInspector(infrastructureapi.ServerMetricsInspector{EncKey: encKey})
	serverHandler := infrastructureapi.NewServerHandler(encKey, clusterService)
	serverNetworkDiagnostics := infrastructureapi.NewServerNetworkDiagnosticsHandler(s, encKey)
	serverTerminal := infrastructureapi.NewServerTerminalHandler(s, encKey)

	siteHandler := NewSiteHandler(s)

	operationHandler := NewOperationHandler(s)

	applicationHandler := NewApplicationHandler(s, encKey).WithDelegationSecret(authCfg.JWTSecret)
	imageRegistryHandler := NewImageRegistryHandler(s, encKey)
	nodeMirrorHandler := delivery.NewNodeRegistryMirrorHandler(s, encKey, func(server *model.Server, content []byte) (string, string) {
		return applyK3sRegistriesToNode(server, encKey, content)
	})
	managedRegistryHandler := delivery.NewManagedOCIRegistryHandler(s, encKey, K8s, func(server *model.Server, content []byte) (string, string) {
		return applyK3sRegistriesToNode(server, encKey, content)
	})
	registryProxyHandler := delivery.NewRegistryProxyHandler(s, encKey, K8s)
	go registryProxyHandler.Reconcile()
	chartHandler := NewChartRepositoryHandler(s)
	registerDeliveryRoutes(r, apiGroup, deliveryRouteHandlers{image: imageRegistryHandler, nodeMirrors: nodeMirrorHandler, managed: managedRegistryHandler, proxy: registryProxyHandler, chart: chartHandler, platform: platformHandler})
	var monitoringDeps systemapi.MonitoringDependencies
	if K8s != nil {
		monitoringClient := monitoringservice.HTTPClient{BaseURL: k8s.VictoriaMetricsServiceURL}
		monitoringDeps = systemapi.MonitoringDependencies{
			Query: monitoringClient.Query, Status: k8s.VictoriaMetricsReadiness{Client: K8s}, Component: k8s.MonitoringComponentAdapter{Client: K8s}, Consumers: monitoringservice.PVCConsumerReader{Pods: k8s.PodReader{Clientset: K8s.Clientset}},
		}
	}
	monitoringHandler := systemapi.NewMonitoringHandler(monitoringDeps)
	registerMonitoringRoutes(apiGroup, monitoringHandler)
	var alertingDeps systemapi.AlertingDependencies
	if K8s != nil {
		alertingClient := &alertingservice.HTTPClient{BaseURL: k8s.AlertmanagerServiceURL}
		alertingDeps = systemapi.AlertingDependencies{
			Alertmanager: alertingClient.Request,
			Component:    k8s.AlertingComponentAdapter{Client: K8s},
			Ready:        func() bool { return K8s.AlertingStatus().State == k8s.AlertingStateReady },
			Secrets:      k8s.SecretReader{Client: K8s},
			Sender:       alertingservice.DefaultNotificationSender{},
		}
	}
	alertingHandler := systemapi.NewAlertingHandler(authCfg.PlatformURL).
		WithDependencies(alertingDeps).
		WithAutomation(s, systemapi.NewAlertRuntimeDispatcher(s, encKey, runtimeRegistry))
	var loggingDeps systemapi.LoggingDependencies
	if K8s != nil {
		loggingClient := loggingservice.HTTPClient{BaseURL: k8s.LokiServiceURL()}
		loggingDeps = systemapi.LoggingDependencies{
			Query:        loggingClient.Query,
			Ready:        func() bool { return K8s.LoggingStatus().LokiReady >= 1 },
			Component:    k8s.LoggingComponentAdapter{Client: K8s},
			FilterReader: k8s.LoggingFilterReader{Clientset: K8s.Clientset},
		}
	}
	loggingHandler := systemapi.NewLoggingHandler(s, loggingDeps)
	registerLoggingRoutes(apiGroup, loggingHandler)
	registerAlertingRoutes(r, apiGroup, alertingHandler)
	domainHandler := infrastructureapi.NewDomainHandlerWithService(s, K8s, networkHandler.Service)
	registerApplicationRoutes(r, apiGroup, applicationHandler, authCfg.JWTSecret, systemapi.AuditMiddleware(s))

	dbAdminHandler := NewDBAdminHandler(s)

	// K8s 节点管理
	nodeHandler := infrastructureapi.NewNodeHandler(clusterService)
	nodeJoinProgress := infrastructureapi.NewNodeJoinProgressHandler(s, encKey, K8s)

	// IngressRoute 管理
	ingressHandler := infrastructureapi.NewIngressHandler(K8s)

	// Certificate 管理
	certHandler := infrastructureapi.NewCertHandlerWithNetworkAndClient(s, encKey, networkService, K8s)

	// K8s 资源管理
	storageService := storageservice.NewService(K8s, s, s)
	k8sHandler := infrastructureapi.NewK8sHandlerWithEncryption(s, storageService, encKey, K8s, s)
	storageHandler := infrastructureapi.NewStorageHandlerWithClient(storageService, s, encKey, K8s)
	storageHandler.ConfigureStorageExecutor()

	// Tailscale 管理
	tailscaleHandler := systemapi.NewTailscaleHandler(s, encKey)
	crdHandler := NewCRDHandler()
	auditHandler := systemapi.NewAuditHandler(s)
	registerInfrastructureRoutes(apiGroup, serverHandler, serverNetworkDiagnostics, serverTerminal, siteHandler, operationHandler, domainHandler, nodeHandler, nodeJoinProgress, ingressHandler, certHandler, k8sHandler, storageHandler, networkHandler, tailscaleHandler, crdHandler, auditHandler, dbAdminHandler)
}
