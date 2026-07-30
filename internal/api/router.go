package api

import (
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
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

	dashHandler := NewDashboardHandler(s)
	apiGroup.GET("/dashboard", dashHandler.Get)

	serverHandler := NewServerHandler(s, encKey)
	servers := apiGroup.Group("/servers")
	{
		servers.POST("", serverHandler.Create)
		servers.GET("", serverHandler.List)
		servers.GET("/:id", serverHandler.Get)
		servers.PUT("/:id", serverHandler.Update)
		servers.DELETE("/:id", serverHandler.Delete)
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
	domainHandler := NewDomainHandler(s)
	domains := apiGroup.Group("/domains")
	{
		domains.GET("", domainHandler.List)
		domains.POST("", domainHandler.Create)
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
		nodes.GET("/:id/join-progress", nodeHandler.JoinProgress)
		nodes.POST("/:id/preimport", nodeHandler.PreImport)
		nodes.POST("/:id/import", nodeHandler.ConfirmImport)
		nodes.POST("/:id/add", nodeHandler.AddNode)
		nodes.POST("/:id/drain", nodeHandler.DrainNode)
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
	k8sHandler := NewK8sHandler()
	k8sGroup := apiGroup.Group("/k8s")
	{
		k8sGroup.GET("/dashboard", k8sHandler.Dashboard)
		k8sGroup.GET("/namespaces", k8sHandler.ListNamespaces)
		k8sGroup.POST("/namespaces", k8sHandler.CreateNamespace)
		k8sGroup.PATCH("/namespaces/:name", k8sHandler.UpdateNamespace)
		k8sGroup.DELETE("/namespaces/:name", k8sHandler.DeleteNamespace)
		k8sGroup.GET("/pods", k8sHandler.ListPods)

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
