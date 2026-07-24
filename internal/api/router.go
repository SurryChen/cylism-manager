package api

import (
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
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
		c.JSON(200, gin.H{"status": "ok"})
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
	nodeHandler := NewNodeHandler(s)
	nodes := apiGroup.Group("/nodes")
	{
		nodes.GET("", nodeHandler.ListNode)
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
	certHandler := NewCertHandler()
	certs := apiGroup.Group("/certs")
	{
		certs.GET("", certHandler.ListCerts)
		certs.POST("", certHandler.CreateCert)
		certs.DELETE("/:namespace/:name", certHandler.DeleteCert)
	}

	// K8s 资源管理
	k8sHandler := NewK8sHandler()
	k8sGroup := apiGroup.Group("/k8s")
	{
		k8sGroup.GET("/dashboard", k8sHandler.Dashboard)
		k8sGroup.GET("/pods", k8sHandler.ListPods)
		k8sGroup.GET("/services", k8sHandler.ListServices)
		k8sGroup.GET("/deployments", k8sHandler.ListDeployments)
		k8sGroup.GET("/services/:namespace/:name", k8sHandler.GetService)
		k8sGroup.PUT("/services/:namespace/:name", k8sHandler.UpdateService)
		k8sGroup.DELETE("/services/:namespace/:name", k8sHandler.DeleteService)
	}

	// 系统状态
	crdHandler := NewCRDHandler()
	apiGroup.GET("/system/crds", crdHandler.CheckCRDs)

	auditHandler := NewAuditHandler(s)
	apiGroup.GET("/audit-logs", auditHandler.List)
}
