package api

import (
	"time"

	"github.com/cylism/cylism-manager/internal/agentauth"
	agentapi "github.com/cylism/cylism-manager/internal/api/agent"
	"github.com/cylism/cylism-manager/internal/api/delivery"
	infrastructureapi "github.com/cylism/cylism-manager/internal/api/infrastructure"
	systemapi "github.com/cylism/cylism-manager/internal/api/system"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	runtimepkg "github.com/cylism/cylism-manager/internal/runtime"
	"github.com/cylism/cylism-manager/internal/service/cluster"
	networkservice "github.com/cylism/cylism-manager/internal/service/network"
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
	systemapi.SetKubernetesClient(K8s)
	// 健康检查（无认证，用于 k8s 探活）
	r.GET("/health", func(c *gin.Context) {
		model.Success(c, gin.H{"status": "ok"})
	})
	// This endpoint is authenticated with a projected Runtime installer token,
	// never with a browser JWT or the Runtime chat credential.
	artifactHandler := agentapi.NewAgentArtifactHandler("/usr/local/lib/cylism/runtime-tools", agentauth.NewRuntimeTokenAuthorizer(K8s, s, nil))
	r.GET(agentapi.CLIArtifactPath, gin.WrapH(artifactHandler))
	r.GET(agentapi.CLIArtifactManifestPath, gin.WrapH(artifactHandler))
	agentHandler := agentapi.NewAgentHandler(s, K8s, agentauth.NewRuntimeTokenAuthorizer(K8s, s, nil)).WithRegistryVerifier(agentapi.DefaultAgentRegistryNodeVerifier(encKey)).WithMaintenanceInspector(agentapi.DefaultAgentMaintenanceInspector(encKey))
	r.GET("/api/agent/v1/cluster/status", gin.WrapF(agentHandler.ClusterStatus))
	r.GET("/api/agent/v1/capabilities/status", gin.WrapF(agentHandler.CapabilityStatus))
	r.GET("/api/agent/v1/workloads/get", gin.WrapF(agentHandler.WorkloadGet))
	r.GET("/api/agent/v1/workloads/logs", gin.WrapF(agentHandler.WorkloadLogs))
	r.GET("/api/agent/v1/pods/get", gin.WrapF(agentHandler.PodGet))
	r.GET("/api/agent/v1/events/list", gin.WrapF(agentHandler.EventList))
	r.GET("/api/agent/v1/pvcs/get", gin.WrapF(agentHandler.PVCGet))
	r.GET("/api/agent/v1/nodes/get", gin.WrapF(agentHandler.NodeGet))
	r.GET("/api/agent/v1/registries/status", gin.WrapF(agentHandler.RegistryStatus))
	r.GET("/api/agent/v1/registries/proxy-diagnose", gin.WrapF(agentHandler.RegistryProxyDiagnose))
	r.GET("/api/agent/v1/images/diagnose", gin.WrapF(agentHandler.ImageDiagnose))
	r.GET("/api/agent/v1/dns/status", gin.WrapF(agentHandler.DNSStatus))
	r.GET("/api/agent/v1/dns/resolve", gin.WrapF(agentHandler.DNSResolve))
	r.GET("/api/agent/v1/registries/node-verify", gin.WrapF(agentHandler.RegistryNodeVerify))
	r.POST("/api/agent/v1/registries/node-pull-check", gin.WrapF(agentHandler.RegistryNodePullCheck))
	r.POST("/api/agent/v1/deployments/scale", gin.WrapF(agentHandler.DeploymentScale))
	r.GET("/api/agent/v1/approvals/:operationID", gin.WrapF(agentHandler.ApprovalGet))
	r.GET("/api/agent/v1/alerts/get", gin.WrapF(agentHandler.AlertGet))
	r.GET("/api/agent/v1/alerts/list", gin.WrapF(agentHandler.AlertList))
	r.GET("/api/agent/v1/monitoring/disk-growth", gin.WrapF(agentHandler.MonitoringDiskGrowth))
	r.GET("/api/agent/v1/maintenance/disk-inspect", gin.WrapF(agentHandler.MaintenanceDiskInspect))
	r.POST("/api/agent/v1/maintenance/cleanup-request", gin.WrapF(agentHandler.MaintenanceCleanupRequest))

	// 认证路由
	authHandler := NewAuthHandler(s, authCfg.JWTSecret, authCfg.AccessTokenTTL, authCfg.RefreshTokenTTL)
	authGroup := r.Group("/api/auth")
	{
		authGroup.POST("/login", authHandler.Login)
		authGroup.POST("/refresh", authHandler.Refresh)
		authGroup.GET("/me", JWTAuthMiddleware(authCfg.JWTSecret), authHandler.Me)
	}

	// 业务 API（受 JWT 保护）
	apiGroup := r.Group("/api")
	apiGroup.Use(JWTAuthMiddleware(authCfg.JWTSecret))
	apiGroup.Use(systemapi.AuditMiddleware(s))
	runtimeRegistry := runtimepkg.BuiltinRegistry()
	runtimeHandler := NewRuntimeHandler(s, encKey, runtimepkg.NewKubernetesManager(K8s, runtimeRegistry), runtimeRegistry)
	agentOperationHandler := agentapi.NewAgentOperationHandler(s, K8s).WithRegistryPullExecutor(agentapi.DefaultAgentRegistryPullExecutor(encKey)).WithMaintenanceCleanupExecutor(agentapi.DefaultAgentMaintenanceCleanupExecutor(encKey))
	systemComponentHandler := systemapi.NewSystemComponentHandler(s)
	networkService := networkservice.NewService(s, s).WithIngressAdapter(K8s).WithStandardIngressAdapter(K8s).WithDNSAdapter(K8s).WithCertificateAdapter(K8s)
	clusterDNSHandler := infrastructureapi.NewClusterDNSHandler(s, K8s)
	networkHandler := infrastructureapi.NewNetworkHandler(networkService, infrastructureapi.NetworkHandler{
		DNSStatus: clusterDNSHandler.Status, DNSApply: clusterDNSHandler.Apply,
		DNSReset: clusterDNSHandler.Reset, DNSRollback: clusterDNSHandler.Rollback,
	})
	runtimes := apiGroup.Group("/runtimes")
	{
		runtimes.GET("/catalog", runtimeHandler.Catalog)
		runtimes.GET("", runtimeHandler.List)
		runtimes.POST("", runtimeHandler.Create)
		runtimes.GET("/:id", runtimeHandler.Get)
		runtimes.PUT("/:id", runtimeHandler.Update)
		runtimes.POST("/:id/deploy", runtimeHandler.Deploy)
		runtimes.POST("/:id/agent-tools/install", runtimeHandler.InstallAgentTools)
		runtimes.POST("/:id/agent-tools/update", runtimeHandler.UpdateAgentTools)
		runtimes.POST("/:id/agent-tools/uninstall", runtimeHandler.UninstallAgentTools)
		runtimes.GET("/:id/agent-capability-grants", agentOperationHandler.ListGrants)
		runtimes.PUT("/:id/agent-capability-grants", agentOperationHandler.ReplaceGrants)
		runtimes.GET("/:id/agent-operations", agentOperationHandler.ListOperations)
		runtimes.POST("/:id/health-check", runtimeHandler.Health)
		runtimes.POST("/:id/uninstall", runtimeHandler.Uninstall)
		runtimes.POST("/:id/chat", runtimeHandler.Chat)
		runtimes.GET("/:id/chat/sessions", runtimeHandler.ChatSessions)
		runtimes.GET("/:id/chat/sessions/:sid/messages", runtimeHandler.ChatSessionMessages)
		runtimes.PATCH("/:id/chat/sessions/:sid", runtimeHandler.RenameChatSession)
		runtimes.POST("/:id/chat/sessions/:sid/archive", runtimeHandler.ArchiveChatSession)
		runtimes.POST("/:id/chat/sessions/:sid/restore", runtimeHandler.RestoreChatSession)
		runtimes.GET("/:id/chat/sessions/:sid/export", runtimeHandler.ExportChatSession)
		runtimes.DELETE("/:id/chat/sessions/:sid", runtimeHandler.DeleteChatSession)
	}
	apiGroup.POST("/agent-operations/:operationID/approve", agentOperationHandler.Approve)
	apiGroup.POST("/agent-operations/:operationID/reject", agentOperationHandler.Reject)
	systemComponents := apiGroup.Group("/system-components")
	{
		systemComponents.GET("", systemComponentHandler.List)
		systemComponents.PUT("/:chart", systemComponentHandler.Update)
		systemComponents.POST("/:chart/revert", systemComponentHandler.Revert)
	}
	clusterDNS := apiGroup.Group("/cluster-dns")
	{
		clusterDNS.GET("", networkHandler.DNSStatus)
		clusterDNS.POST("", networkHandler.DNSApply)
		clusterDNS.DELETE("", networkHandler.DNSReset)
		clusterDNS.POST("/history/:revision/rollback", networkHandler.DNSRollback)
	}
	platformHandler := delivery.NewPlatformHandler(s, encKey, K8s)
	go platformHandler.Reconcile()

	// GitHub Actions calls this signed endpoint after pushing a platform image.
	r.POST("/api/platform/deployments", platformHandler.Webhook)

	dashHandler := NewDashboardHandler(s)
	apiGroup.GET("/dashboard", dashHandler.Get)

	clusterService := cluster.NewService(s, clusterNodeAdapter()).WithServerInspector(infrastructureapi.ServerInspector{EncKey: encKey}).WithServerImporter(infrastructureapi.ServerInspector{EncKey: encKey}).WithMetricsInspector(infrastructureapi.ServerMetricsInspector{EncKey: encKey})
	serverHandler := infrastructureapi.NewServerHandler(encKey, clusterService)
	serverNetworkDiagnostics := infrastructureapi.NewServerNetworkDiagnosticsHandler(s, encKey)
	serverTerminal := infrastructureapi.NewServerTerminalHandler(s, encKey)
	servers := apiGroup.Group("/servers")
	{
		servers.POST("", serverHandler.Create)
		servers.GET("", serverHandler.List)
		servers.GET("/resource-stats", serverHandler.ResourceStats)
		servers.GET("/network-diagnostics", serverNetworkDiagnostics.NetworkDiagnostics)
		servers.GET("/:id", serverHandler.Get)
		servers.PUT("/:id", serverHandler.Update)
		servers.DELETE("/:id", serverHandler.Delete)
		servers.POST("/:id/unbind", serverHandler.Unbind)
		servers.POST("/:id/probe", serverHandler.Probe)
		servers.POST("/:id/precheck", serverHandler.Precheck)
		servers.GET("/:id/stats", serverHandler.Stats)
		servers.GET("/:id/terminal", serverTerminal.Terminal)
	}

	siteHandler := NewSiteHandler(s)
	sites := apiGroup.Group("/sites")
	{
		sites.POST("", siteHandler.Create)
		sites.GET("", siteHandler.List)
		sites.GET("/:id", siteHandler.Get)
		sites.PUT("/:id", siteHandler.Update)
		sites.DELETE("/:id", siteHandler.Delete)
	}

	operationHandler := NewOperationHandler(s)
	apiGroup.GET("/operations", operationHandler.ListOperations)

	applicationHandler := NewApplicationHandler(s, encKey).WithDelegationSecret(authCfg.JWTSecret)
	imageRegistryHandler := NewImageRegistryHandler(s, encKey)
	imageRegistries := apiGroup.Group("/image-registries")
	{
		imageRegistries.GET("", imageRegistryHandler.List)
		imageRegistries.POST("", imageRegistryHandler.Create)
		imageRegistries.POST("/:id/verify", imageRegistryHandler.Verify)
		imageRegistries.PUT("/:id", imageRegistryHandler.Update)
		imageRegistries.DELETE("/:id", imageRegistryHandler.Delete)
	}
	nodeRegistryMirrors := apiGroup.Group("/node-registry-mirrors")
	{
		h := delivery.NewNodeRegistryMirrorHandler(s, encKey, func(server *model.Server, content []byte) (string, string) {
			return applyK3sRegistriesToNode(server, encKey, content)
		})
		nodeRegistryMirrors.GET("", h.List)
		nodeRegistryMirrors.POST("", h.Create)
		nodeRegistryMirrors.PUT("/:id", h.Update)
		nodeRegistryMirrors.DELETE("/:id", h.Delete)
		nodeRegistryMirrors.POST("/:id/verify", h.Verify)
		nodeRegistryMirrors.POST("/:id/apply", h.Apply)
		nodeRegistryMirrors.GET("/:id/apply-status", h.ApplyStatus)
	}
	managedOCIRegistries := apiGroup.Group("/managed-oci-registries")
	{
		h := delivery.NewManagedOCIRegistryHandler(s, encKey, K8s, func(server *model.Server, content []byte) (string, string) {
			return applyK3sRegistriesToNode(server, encKey, content)
		})
		managedOCIRegistries.GET("", h.List)
		managedOCIRegistries.GET("/storage-preflight", h.StoragePreflight)
		managedOCIRegistries.GET("/pvcs", h.ListEligiblePVCs)
		managedOCIRegistries.GET("/certificates", h.ListMatchingCertificates)
		managedOCIRegistries.POST("", h.Create)
		managedOCIRegistries.GET("/:id", h.Get)
		managedOCIRegistries.PUT("/:id", h.Update)
		managedOCIRegistries.POST("/:id/repair", h.Repair)
		managedOCIRegistries.POST("/:id/apply-node-access", h.ApplyNodeAccess)
		managedOCIRegistries.DELETE("/:id", h.Delete)
	}
	registryProxyHandler := delivery.NewRegistryProxyHandler(s, encKey, K8s)
	go registryProxyHandler.Reconcile()
	registryProxy := apiGroup.Group("/registry-proxy")
	{
		registryProxy.GET("", registryProxyHandler.Get)
		registryProxy.POST("/deploy", registryProxyHandler.Deploy)
		registryProxy.POST("/cleanup", registryProxyHandler.Cleanup)
	}
	registryProxies := apiGroup.Group("/registry-proxies")
	{
		registryProxies.GET("", registryProxyHandler.List)
		registryProxies.POST("", registryProxyHandler.Deploy)
		registryProxies.PUT("/:id", registryProxyHandler.Deploy)
		registryProxies.POST("/:id/cleanup", registryProxyHandler.Cleanup)
		registryProxies.POST("/:id/diagnose", registryProxyHandler.Diagnose)
		registryProxies.POST("/:id/migrate-resource-name", registryProxyHandler.MigrateResourceName)
	}
	chartRepositories := apiGroup.Group("/chart-repositories")
	{
		h := NewChartRepositoryHandler(s)
		chartRepositories.GET("", h.List)
		chartRepositories.POST("", h.Create)
		chartRepositories.PUT("/:id", h.Update)
		chartRepositories.DELETE("/:id", h.Delete)
		chartRepositories.POST("/:id/verify", h.Verify)
	}
	monitoring := apiGroup.Group("/monitoring")
	{
		h := systemapi.NewMonitoringHandler()
		monitoring.GET("/status", h.Status)
		monitoring.POST("/install", h.Install)
		monitoring.POST("/storage-migration", h.MigrateLegacyStorage)
		monitoring.DELETE("", h.Uninstall)
		monitoring.GET("/query", h.Query)
		monitoring.GET("/query-range", h.QueryRange)
		monitoring.GET("/dashboard", h.Dashboard)
		monitoring.GET("/disk-growth", h.DiskGrowth)
		monitoring.GET("/targets", h.Targets)
	}
	alertingHandler := systemapi.NewAlertingHandler(authCfg.PlatformURL).WithAutomation(s, systemapi.NewAlertRuntimeDispatcher(s, encKey, runtimeRegistry))
	alerts := apiGroup.Group("/monitoring/alerts")
	{
		alerts.GET("/status", alertingHandler.Status)
		alerts.POST("/install", alertingHandler.Install)
		alerts.PUT("/config", alertingHandler.Update)
		alerts.DELETE("", alertingHandler.Uninstall)
		alerts.GET("/overview", alertingHandler.Overview)
		alerts.GET("/silences", alertingHandler.ListSilences)
		alerts.POST("/silences", alertingHandler.CreateSilence)
		alerts.DELETE("/silences/:id", alertingHandler.DeleteSilence)
		alerts.POST("/test-notification", alertingHandler.TestNotification)
		alerts.GET("/automation-policy", alertingHandler.AutomationPolicy)
		alerts.PUT("/automation-policy", alertingHandler.UpdateAutomationPolicy)
		alerts.GET("/automation-events", alertingHandler.ListAutomationEvents)
	}
	loggingHandler := systemapi.NewLoggingHandler(s)
	logs := apiGroup.Group("/monitoring/logs")
	{
		logs.GET("/status", loggingHandler.Status)
		logs.POST("/install", loggingHandler.Install)
		logs.PUT("/config", loggingHandler.Update)
		logs.DELETE("", loggingHandler.Uninstall)
		logs.GET("/filters", loggingHandler.Filters)
		logs.POST("/query", loggingHandler.Query)
	}
	// Alertmanager is an in-cluster client rather than a browser client. Its
	// dedicated endpoint validates a per-install bearer token in the handler.
	r.POST("/api/monitoring/alerts/notify", alertingHandler.Notify)
	platform := apiGroup.Group("/platform")
	{
		platform.GET("/status", platformHandler.Status)
		platform.GET("/endpoint", platformHandler.EndpointStatus)
		platform.PUT("/endpoint", platformHandler.UpdateEndpoint)
		platform.POST("/endpoint/reconcile", platformHandler.ReconcileEndpoint)
		platform.POST("/endpoint/adopt-ingress", platformHandler.AdoptEndpointIngress)
		platform.POST("/releases", platformHandler.ManualUpdate)
		platform.POST("/webhook-secret", platformHandler.GenerateWebhookSecret)
		platform.PUT("/image-prefix", platformHandler.UpdateImagePrefix)
		platform.POST("/releases/:id/rollback", platformHandler.Rollback)
	}
	domainHandler := infrastructureapi.NewDomainHandlerWithService(s, K8s, networkHandler.Service)
	domains := apiGroup.Group("/domains")
	{
		domains.GET("", domainHandler.List)
		domains.POST("", domainHandler.Create)
		domains.GET("/importable-certificates", domainHandler.ListImportableCertificates)
		domains.POST("/import", domainHandler.ImportCertificate)
		domains.GET("/claimable", domainHandler.ListClaimable)
		domains.POST("/:id/claim", domainHandler.Claim)
		domains.PUT("/:id", domainHandler.Update)
		domains.POST("/:id/certificate", domainHandler.RetryCertificate)
		domains.GET("/:id/operations", domainHandler.ListOperations)
		domains.DELETE("/:id", domainHandler.Delete)
	}
	projects := apiGroup.Group("/projects")
	{
		projects.GET("", applicationHandler.ListProjects)
		projects.POST("", applicationHandler.CreateProject)
		projects.PUT("/:projectID", applicationHandler.UpdateProject)
		projects.DELETE("/:projectID", applicationHandler.DeleteProject)
		projects.GET("/:projectID/environments", applicationHandler.ListEnvironments)
		projects.GET("/environments/namespace-conflicts", applicationHandler.ListEnvironmentNamespaceConflicts)
		projects.POST("/:projectID/environments", applicationHandler.CreateEnvironment)
		projects.PUT("/:projectID/environments/:environmentID", applicationHandler.UpdateEnvironment)
		projects.POST("/:projectID/environments/:environmentID/sync-namespace", applicationHandler.SyncEnvironmentNamespace)
		projects.DELETE("/:projectID/environments/:environmentID", applicationHandler.DeleteEnvironment)
	}
	applications := apiGroup.Group("/applications")
	{
		applications.GET("", applicationHandler.ListApplications)
		applications.GET("/discovery", applicationHandler.DiscoverApplications)
		applications.POST("", applicationHandler.CreateApplication)
		applications.GET("/:id", applicationHandler.GetApplication)
		applications.PUT("/:id/capabilities", applicationHandler.UpdateCapabilities)
		applications.GET("/:id/managed-files", applicationHandler.ListManagedFiles)
		applications.POST("/:id/delegations", applicationHandler.CreateDelegation)
		applications.POST("/:id/integration-handoffs", applicationHandler.CreateIntegrationHandoff)
		applications.PUT("/:id/workload-kind", applicationHandler.UpdateWorkloadKind)
		applications.GET("/:id/runtime", applicationHandler.GetApplicationRuntime)
		applications.GET("/:id/deployment-templates", applicationHandler.ListDeploymentTemplates)
		applications.POST("/:id/deployment-templates", applicationHandler.CreateDeploymentTemplate)
		applications.GET("/:id/deployment-templates/:templateID", applicationHandler.GetDeploymentTemplate)
		applications.PUT("/:id/deployment-templates/:templateID", applicationHandler.UpdateDeploymentTemplate)
		applications.DELETE("/:id/deployment-templates/:templateID", applicationHandler.DeleteDeploymentTemplate)
		applications.POST("/:id/deployment-templates/:templateID/default", applicationHandler.SetDefaultDeploymentTemplate)
		applications.GET("/:id/endpoints", applicationHandler.ListApplicationEndpoints)
		applications.POST("/:id/endpoints", applicationHandler.CreateApplicationEndpoint)
		applications.PUT("/:id/endpoints/:endpointID", applicationHandler.UpdateApplicationEndpoint)
		applications.DELETE("/:id/endpoints/:endpointID", applicationHandler.DeleteApplicationEndpoint)
		applications.POST("/:id/releases", applicationHandler.CreateRelease)
		applications.POST("/:id/restarts", applicationHandler.RestartApplication)
		applications.GET("/:id/releases/:releaseID", applicationHandler.GetRelease)
		applications.POST("/:id/releases/:releaseID/retry", applicationHandler.RetryRelease)
		applications.POST("/:id/releases/:releaseID/rollback", applicationHandler.RollbackRelease)
	}

	// External integrations use a separate, short-lived delegation
	// instead of a browser JWT. This group must remain narrower than /api.
	integration := r.Group("/api/integrations/applications")
	integration.Use(DelegationAuthMiddleware(authCfg.JWTSecret))
	integration.Use(systemapi.AuditMiddleware(s))
	{
		integration.GET("/discovery", applicationHandler.IntegrationDiscoverApplications)
		integration.GET("/:id/runtime", applicationHandler.IntegrationGetApplicationRuntime)
		integration.GET("/:id/configmaps", applicationHandler.IntegrationListManagedConfigMaps)
		integration.GET("/:id/configmaps/:configMapID", applicationHandler.IntegrationGetManagedConfigMap)
		integration.PUT("/:id/configmaps/:configMapID", applicationHandler.IntegrationReplaceManagedConfigMap)
		integration.POST("/:id/restarts", applicationHandler.IntegrationRestartApplication)
		integration.GET("/:id/releases/:releaseID", applicationHandler.IntegrationGetRelease)
	}
	// Integration handoff and renewal use opaque bearer credentials, not browser JWTs.
	sessions := r.Group("/api/integrations/sessions")
	{
		sessions.POST("/exchange", applicationHandler.ExchangeIntegrationSession)
		sessions.POST("/delegation", applicationHandler.CreateIntegrationDelegation)
	}
	workspace := apiGroup.Group("/workspace")
	{
		workspace.GET("/overview", applicationHandler.WorkspaceOverview)
	}

	dbAdminHandler := NewDBAdminHandler(s)
	adminGroup := apiGroup.Group("/admin/tables")
	{
		adminGroup.GET("", dbAdminHandler.ListTables)
		adminGroup.GET("/:table", dbAdminHandler.ListRecords)
		adminGroup.POST("/:table", dbAdminHandler.CreateRecord)
		adminGroup.PUT("/:table/:id", dbAdminHandler.UpdateRecord)
		adminGroup.DELETE("/:table/:id", dbAdminHandler.DeleteRecord)
	}

	// K8s 节点管理
	nodeHandler := infrastructureapi.NewNodeHandler(clusterService)
	nodeJoinProgress := infrastructureapi.NewNodeJoinProgressHandler(s, encKey, K8s)
	nodes := apiGroup.Group("/nodes")
	{
		nodes.GET("", nodeHandler.ListNode)
		nodes.GET("/:id/labels", nodeHandler.GetLabels)
		nodes.PATCH("/:id/labels", nodeHandler.UpdateLabels)
		nodes.GET("/:id/join-progress", nodeJoinProgress.JoinProgress)
		nodes.GET("/:id/drain-plan", nodeHandler.DrainPlan)
		nodes.GET("/:id/removal-check", nodeHandler.RemovalCheck)
		nodes.POST("/:id/preimport", nodeHandler.PreImport)
		nodes.POST("/:id/import", nodeHandler.ConfirmImport)
		nodes.POST("/:id/add", nodeHandler.AddNode)
		nodes.POST("/:id/drain", nodeHandler.DrainNode)
		nodes.POST("/:id/force-drain", nodeHandler.ForceDrainNode)
		nodes.POST("/:id/rejoin", nodeHandler.RejoinNode)
		nodes.DELETE("/:id", nodeHandler.RemoveNode)
	}

	// IngressRoute 管理
	ingressHandler := infrastructureapi.NewIngressHandler(K8s)
	routes := apiGroup.Group("/routes")
	{
		routes.GET("", ingressHandler.ListRoutes)
		routes.POST("", ingressHandler.CreateRoute)
		routes.PUT("/:namespace/:name", ingressHandler.UpdateRoute)
		routes.DELETE("/:namespace/:name", ingressHandler.DeleteRoute)
		routes.GET("/middlewares", ingressHandler.ListMiddlewares)
		routes.GET("/tls-stores", ingressHandler.ListTLSStores)
	}

	// Certificate 管理
	certHandler := infrastructureapi.NewCertHandlerWithNetworkAndClient(s, encKey, networkService, K8s)
	certs := apiGroup.Group("/certs")
	{
		certs.GET("/status", certHandler.Status)
		certs.POST("/install", certHandler.Install)
		certs.GET("", certHandler.ListCerts)
		certs.GET("/issuers", certHandler.ListIssuers)
		certs.POST("/issuers", certHandler.CreateIssuer)
		certs.PUT("/issuers/:kind/:namespace/:name", certHandler.UpdateIssuer)
		certs.DELETE("/issuers/:kind/:namespace/:name", certHandler.DeleteIssuer)
		certs.GET("/dns-credentials", certHandler.ListDNSCredentials)
		certs.POST("/dns-credentials", certHandler.CreateDNSCredential)
		certs.PUT("/dns-credentials/:id", certHandler.UpdateDNSCredential)
		certs.DELETE("/dns-credentials/:id", certHandler.DeleteDNSCredential)
		certs.GET("/dns-providers", certHandler.ListDNSProviders)
		certs.GET("/dns-providers/:provider/status", certHandler.DNSProviderStatus)
		certs.POST("/dns-providers/:provider/install", certHandler.InstallDNSProvider)
		certs.POST("", certHandler.CreateCert)
		certs.GET("/:namespace/:name/operations", certHandler.ListOperations)
		certs.DELETE("/:namespace/:name", certHandler.DeleteCert)
	}

	// K8s 资源管理
	storageService := storageservice.NewService(K8s, s, s)
	k8sHandler := infrastructureapi.NewK8sHandlerWithEncryption(s, storageService, encKey, K8s, s)
	storageHandler := infrastructureapi.NewStorageHandlerWithClient(storageService, s, encKey, K8s)
	storageHandler.ConfigureStorageExecutor()
	k8sGroup := apiGroup.Group("/k8s")
	{
		k8sGroup.GET("/dashboard", k8sHandler.Dashboard)
		k8sGroup.GET("/namespaces", k8sHandler.ListNamespaces)
		k8sGroup.GET("/namespace-names", k8sHandler.ListNamespaceNames)
		k8sGroup.POST("/namespaces", k8sHandler.CreateNamespace)
		k8sGroup.PATCH("/namespaces/:name", k8sHandler.UpdateNamespace)
		k8sGroup.DELETE("/namespaces/:name", k8sHandler.DeleteNamespace)
		k8sGroup.GET("/pods", k8sHandler.ListPods)
		k8sGroup.GET("/pods/:namespace/:name/terminal", k8sHandler.PodTerminal)

		// 工作负载
		k8sGroup.GET("/deployments", k8sHandler.ListDeployments)
		k8sGroup.GET("/deployments/:namespace/:name", k8sHandler.GetDeployment)
		k8sGroup.GET("/deployments/:namespace/:name/pods", k8sHandler.ListDeploymentPods)
		k8sGroup.GET("/deployments/:namespace/:name/revisions", k8sHandler.ListDeploymentRevisions)
		k8sGroup.PATCH("/deployments/:namespace/:name/scale", k8sHandler.ScaleDeployment)
		k8sGroup.PATCH("/deployments/:namespace/:name/image", k8sHandler.UpdateDeploymentImage)
		k8sGroup.POST("/deployments/:namespace/:name/rollback", k8sHandler.RollbackDeployment)

		k8sGroup.GET("/statefulsets", k8sHandler.ListStatefulSets)
		k8sGroup.GET("/statefulsets/:namespace/:name", k8sHandler.GetStatefulSet)
		k8sGroup.PATCH("/statefulsets/:namespace/:name/scale", k8sHandler.ScaleStatefulSet)

		k8sGroup.GET("/daemonsets", k8sHandler.ListDaemonSets)
		k8sGroup.GET("/daemonsets/:namespace/:name", k8sHandler.GetDaemonSet)

		// 服务发现
		k8sGroup.GET("/services", k8sHandler.ListServicesV2)
		k8sGroup.GET("/services/:namespace/:name", k8sHandler.GetService)
		k8sGroup.GET("/services/:namespace/:name/endpoints", k8sHandler.GetServiceEndpoints)
		k8sGroup.PUT("/services/:namespace/:name", k8sHandler.UpdateService)
		k8sGroup.DELETE("/services/:namespace/:name", k8sHandler.DeleteService)

		// 配置管理
		k8sGroup.GET("/configmaps", k8sHandler.ListConfigMaps)
		k8sGroup.POST("/configmaps", k8sHandler.CreateConfigMap)
		k8sGroup.GET("/configmaps/:namespace/:name", k8sHandler.GetConfigMap)
		k8sGroup.PUT("/configmaps/:namespace/:name", k8sHandler.UpdateConfigMap)
		k8sGroup.DELETE("/configmaps/:namespace/:name", k8sHandler.DeleteConfigMap)
		k8sGroup.GET("/secrets", k8sHandler.ListSecrets)
		k8sGroup.POST("/secrets", k8sHandler.CreateOpaqueSecret)
		k8sGroup.GET("/secrets/:namespace/:name", k8sHandler.GetSecret)
		k8sGroup.PUT("/secrets/:namespace/:name", k8sHandler.UpdateOpaqueSecret)
		k8sGroup.DELETE("/secrets/:namespace/:name", k8sHandler.DeleteOpaqueSecret)
		k8sGroup.GET("/storage-classes", storageHandler.ListStorageClasses)
		k8sGroup.GET("/persistent-volume-claims", storageHandler.ListPersistentVolumeClaims)
		k8sGroup.GET("/persistent-volume-claims/usage", storageHandler.ListPersistentVolumeClaimUsage)
		k8sGroup.POST("/persistent-volume-claims", storageHandler.CreatePersistentVolumeClaim)
		k8sGroup.GET("/persistent-volume-migrations", storageHandler.ListPersistentVolumeMigrations)
		k8sGroup.GET("/persistent-volume-migrations/:id", storageHandler.GetPersistentVolumeMigration)
		k8sGroup.POST("/persistent-volume-migrations/:id/cleanup", storageHandler.CleanupPersistentVolumeMigration)
		k8sGroup.POST("/persistent-volume-claims/:name/migrations", storageHandler.CreatePersistentVolumeMigration)
		k8sGroup.GET("/persistent-volume-claims/:name/backups", storageHandler.ListPersistentVolumeBackups)
		k8sGroup.POST("/persistent-volume-claims/:name/backups", storageHandler.CreatePersistentVolumeBackup)
		k8sGroup.POST("/persistent-volume-claims/:name/backups/:backupID/restore", storageHandler.RestorePersistentVolumeBackup)
		k8sGroup.GET("/persistent-volume-claims/:name/imports", storageHandler.ListHostDirectoryPVCImports)
		k8sGroup.POST("/persistent-volume-claims/:name/imports", storageHandler.CreateHostDirectoryPVCImport)
		k8sGroup.DELETE("/persistent-volume-claims/:name/imports/:id/backup", storageHandler.DeleteHostDirectoryPVCImportBackup)
		k8sGroup.DELETE("/persistent-volume-claims/:name", storageHandler.DeletePersistentVolumeClaim)

		// 标准 Ingress
		k8sGroup.GET("/ingresses", networkHandler.ListStandardIngresses)
		k8sGroup.GET("/ingresses/:namespace/:name", networkHandler.GetStandardIngress)
		k8sGroup.POST("/ingresses", networkHandler.CreateStandardIngress)
		k8sGroup.DELETE("/ingresses/:namespace/:name", networkHandler.DeleteStandardIngress)
		k8sGroup.GET("/ingress-controller", networkHandler.StandardIngressController)
	}

	// Tailscale 管理
	tailscaleHandler := systemapi.NewTailscaleHandler(s, encKey)
	tailscale := apiGroup.Group("/tailscale")
	{
		tailscale.POST("/init", tailscaleHandler.Init)
		tailscale.GET("/status", tailscaleHandler.Status)
		tailscale.GET("/install-script", tailscaleHandler.InstallScript)
	}

	// 系统状态
	crdHandler := NewCRDHandler()
	apiGroup.GET("/system/crds", crdHandler.CheckCRDs)

	auditHandler := systemapi.NewAuditHandler(s)
	apiGroup.GET("/audit-logs", auditHandler.List)
}
