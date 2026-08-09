package api

import (
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	runtimepkg "github.com/cylism/cylism-manager/internal/runtime"
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
	// 健康检查（无认证，用于 k8s 探活）
	r.GET("/health", func(c *gin.Context) {
		model.Success(c, gin.H{"status": "ok"})
	})

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
	apiGroup.Use(AuditMiddleware(s))
	runtimeRegistry := runtimepkg.BuiltinRegistry()
	runtimeHandler := NewRuntimeHandler(s, encKey, runtimepkg.NewKubernetesManager(K8s, runtimeRegistry), runtimeRegistry)
	runtimes := apiGroup.Group("/runtimes")
	{
		runtimes.GET("/catalog", runtimeHandler.Catalog)
		runtimes.GET("", runtimeHandler.List)
		runtimes.POST("", runtimeHandler.Create)
		runtimes.GET("/:id", runtimeHandler.Get)
		runtimes.PUT("/:id", runtimeHandler.Update)
		runtimes.POST("/:id/deploy", runtimeHandler.Deploy)
		runtimes.POST("/:id/health-check", runtimeHandler.Health)
		runtimes.POST("/:id/uninstall", runtimeHandler.Uninstall)
		runtimes.POST("/:id/chat", runtimeHandler.Chat)
		runtimes.GET("/:id/chat/sessions", runtimeHandler.ChatSessions)
		runtimes.GET("/:id/chat/sessions/:sid/messages", runtimeHandler.ChatSessionMessages)
	}
	platformHandler := NewPlatformHandler(s, encKey)
	go platformHandler.Reconcile()

	// GitHub Actions calls this signed endpoint after pushing a platform image.
	r.POST("/api/platform/deployments", platformHandler.Webhook)

	dashHandler := NewDashboardHandler(s)
	apiGroup.GET("/dashboard", dashHandler.Get)

	serverHandler := NewServerHandler(s, encKey)
	servers := apiGroup.Group("/servers")
	{
		servers.POST("", serverHandler.Create)
		servers.GET("", serverHandler.List)
		servers.GET("/resource-stats", serverHandler.ResourceStats)
		servers.GET("/:id", serverHandler.Get)
		servers.PUT("/:id", serverHandler.Update)
		servers.DELETE("/:id", serverHandler.Delete)
		servers.POST("/:id/unbind", serverHandler.Unbind)
		servers.POST("/:id/probe", serverHandler.Probe)
		servers.POST("/:id/precheck", serverHandler.Precheck)
		servers.GET("/:id/stats", serverHandler.Stats)
		servers.GET("/:id/terminal", serverHandler.Terminal)
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

	applicationHandler := NewApplicationHandler(s, encKey)
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
		h := NewNodeRegistryMirrorHandler(s, encKey)
		nodeRegistryMirrors.GET("", h.List)
		nodeRegistryMirrors.POST("", h.Create)
		nodeRegistryMirrors.PUT("/:id", h.Update)
		nodeRegistryMirrors.DELETE("/:id", h.Delete)
		nodeRegistryMirrors.POST("/:id/verify", h.Verify)
		nodeRegistryMirrors.POST("/:id/apply", h.Apply)
		nodeRegistryMirrors.GET("/:id/apply-status", h.ApplyStatus)
	}
	registryProxyHandler := NewRegistryProxyHandler(s)
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
		h := NewMonitoringHandler()
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
	alertingHandler := NewAlertingHandler(authCfg.PlatformURL)
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
	}
	loggingHandler := NewLoggingHandler(s)
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
		platform.POST("/releases", platformHandler.ManualUpdate)
		platform.POST("/webhook-secret", platformHandler.GenerateWebhookSecret)
		platform.PUT("/image-prefix", platformHandler.UpdateImagePrefix)
		platform.POST("/releases/:id/rollback", platformHandler.Rollback)
	}
	domainHandler := NewDomainHandler(s)
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
		applications.POST("", applicationHandler.CreateApplication)
		applications.GET("/:id", applicationHandler.GetApplication)
		applications.PUT("/:id/workload-kind", applicationHandler.UpdateWorkloadKind)
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
		applications.GET("/:id/releases/:releaseID", applicationHandler.GetRelease)
		applications.POST("/:id/releases/:releaseID/retry", applicationHandler.RetryRelease)
		applications.POST("/:id/releases/:releaseID/rollback", applicationHandler.RollbackRelease)
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
	nodeHandler := NewNodeHandler(s, encKey)
	nodes := apiGroup.Group("/nodes")
	{
		nodes.GET("", nodeHandler.ListNode)
		nodes.GET("/:id/labels", nodeHandler.GetLabels)
		nodes.PATCH("/:id/labels", nodeHandler.UpdateLabels)
		nodes.GET("/:id/join-progress", nodeHandler.JoinProgress)
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
	ingressHandler := NewIngressHandler()
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
	certHandler := NewCertHandlerWithEncryption(s, encKey)
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
	k8sHandler := NewK8sHandlerWithEncryption(s, encKey)
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
		k8sGroup.GET("/configmaps/:namespace/:name", k8sHandler.GetConfigMap)
		k8sGroup.GET("/secrets", k8sHandler.ListSecrets)
		k8sGroup.GET("/secrets/:namespace/:name", k8sHandler.GetSecret)
		k8sGroup.GET("/storage-classes", k8sHandler.ListStorageClasses)
		k8sGroup.GET("/persistent-volume-claims", k8sHandler.ListPersistentVolumeClaims)
		k8sGroup.GET("/persistent-volume-claims/usage", k8sHandler.ListPersistentVolumeClaimUsage)
		k8sGroup.POST("/persistent-volume-claims", k8sHandler.CreatePersistentVolumeClaim)
		k8sGroup.GET("/persistent-volume-migrations", k8sHandler.ListPersistentVolumeMigrations)
		k8sGroup.GET("/persistent-volume-migrations/:id", k8sHandler.GetPersistentVolumeMigration)
		k8sGroup.POST("/persistent-volume-migrations/:id/cleanup", k8sHandler.CleanupPersistentVolumeMigration)
		k8sGroup.POST("/persistent-volume-claims/:name/migrations", k8sHandler.CreatePersistentVolumeMigration)
		k8sGroup.GET("/persistent-volume-claims/:name/backups", k8sHandler.ListPersistentVolumeBackups)
		k8sGroup.POST("/persistent-volume-claims/:name/backups", k8sHandler.CreatePersistentVolumeBackup)
		k8sGroup.POST("/persistent-volume-claims/:name/backups/:backupID/restore", k8sHandler.RestorePersistentVolumeBackup)
		k8sGroup.GET("/persistent-volume-claims/:name/imports", k8sHandler.ListHostDirectoryPVCImports)
		k8sGroup.POST("/persistent-volume-claims/:name/imports", k8sHandler.CreateHostDirectoryPVCImport)
		k8sGroup.DELETE("/persistent-volume-claims/:name/imports/:id/backup", k8sHandler.DeleteHostDirectoryPVCImportBackup)
		k8sGroup.DELETE("/persistent-volume-claims/:name", k8sHandler.DeletePersistentVolumeClaim)

		// 标准 Ingress
		k8sGroup.GET("/ingresses", k8sHandler.ListIngresses)
		k8sGroup.GET("/ingresses/:namespace/:name", k8sHandler.GetIngress)
		k8sGroup.POST("/ingresses", k8sHandler.CreateIngress)
		k8sGroup.DELETE("/ingresses/:namespace/:name", k8sHandler.DeleteIngress)
		k8sGroup.GET("/ingress-controller", k8sHandler.GetIngressController)
	}

	// Tailscale 管理
	tailscaleHandler := NewTailscaleHandler(s, encKey)
	tailscale := apiGroup.Group("/tailscale")
	{
		tailscale.POST("/init", tailscaleHandler.Init)
		tailscale.GET("/status", tailscaleHandler.Status)
		tailscale.GET("/install-script", tailscaleHandler.InstallScript)
	}

	// 系统状态
	crdHandler := NewCRDHandler()
	apiGroup.GET("/system/crds", crdHandler.CheckCRDs)

	auditHandler := NewAuditHandler(s)
	apiGroup.GET("/audit-logs", auditHandler.List)
}
