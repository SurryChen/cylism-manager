package api

import (
	"github.com/cylism/cylism-manager/internal/api/delivery"
	"github.com/gin-gonic/gin"
)

type deliveryRouteHandlers struct {
	image       *delivery.ImageRegistryHandler
	nodeMirrors *delivery.NodeRegistryMirrorHandler
	managed     *delivery.ManagedOCIRegistryHandler
	proxy       *delivery.RegistryProxyHandler
	chart       *delivery.ChartRepositoryHandler
	platform    *delivery.PlatformHandler
}

func registerDeliveryRoutes(r *gin.Engine, apiGroup *gin.RouterGroup, h deliveryRouteHandlers) {
	imageRegistries := apiGroup.Group("/image-registries")
	imageRegistries.GET("", h.image.List)
	imageRegistries.POST("", h.image.Create)
	imageRegistries.POST("/:id/verify", h.image.Verify)
	imageRegistries.PUT("/:id", h.image.Update)
	imageRegistries.DELETE("/:id", h.image.Delete)

	nodeRegistryMirrors := apiGroup.Group("/node-registry-mirrors")
	nodeRegistryMirrors.GET("", h.nodeMirrors.List)
	nodeRegistryMirrors.POST("", h.nodeMirrors.Create)
	nodeRegistryMirrors.POST("/inspect-actual-config", h.nodeMirrors.InspectActualConfig)
	nodeRegistryMirrors.POST("/nodes/:id/restart-k3s", h.nodeMirrors.RestartNodeK3s)
	nodeRegistryMirrors.PUT("/:id", h.nodeMirrors.Update)
	nodeRegistryMirrors.DELETE("/:id", h.nodeMirrors.Delete)
	nodeRegistryMirrors.POST("/:id/verify", h.nodeMirrors.Verify)
	nodeRegistryMirrors.POST("/:id/apply", h.nodeMirrors.Apply)
	nodeRegistryMirrors.GET("/:id/apply-status", h.nodeMirrors.ApplyStatus)

	managedOCIRegistries := apiGroup.Group("/managed-oci-registries")
	managedOCIRegistries.GET("", h.managed.List)
	managedOCIRegistries.POST("/:id/refresh-status", h.managed.RefreshStatus)
	managedOCIRegistries.GET("/storage-preflight", h.managed.StoragePreflight)
	managedOCIRegistries.GET("/pvcs", h.managed.ListEligiblePVCs)
	managedOCIRegistries.GET("/certificates", h.managed.ListMatchingCertificates)
	managedOCIRegistries.POST("", h.managed.Create)
	managedOCIRegistries.GET("/:id/catalog", h.managed.ListCatalog)
	managedOCIRegistries.GET("/:id/catalog/tags", h.managed.ListCatalogTags)
	managedOCIRegistries.POST("/:id/catalog/tags/preflight-delete", h.managed.PreflightCatalogTagDelete)
	managedOCIRegistries.DELETE("/:id/catalog/tags", h.managed.DeleteCatalogTag)
	managedOCIRegistries.POST("/:id/catalog/repositories/preflight-delete", h.managed.PreflightCatalogRepositoryDelete)
	managedOCIRegistries.DELETE("/:id/catalog/repositories", h.managed.DeleteCatalogRepository)
	managedOCIRegistries.GET("/:id", h.managed.Get)
	managedOCIRegistries.PUT("/:id", h.managed.Update)
	managedOCIRegistries.POST("/:id/repair", h.managed.Repair)
	managedOCIRegistries.POST("/:id/apply-node-access", h.managed.ApplyNodeAccess)
	managedOCIRegistries.DELETE("/:id", h.managed.Delete)

	registryProxy := apiGroup.Group("/registry-proxy")
	registryProxy.GET("", h.proxy.Get)
	registryProxy.POST("/deploy", h.proxy.Deploy)
	registryProxy.POST("/cleanup", h.proxy.Cleanup)
	registryProxies := apiGroup.Group("/registry-proxies")
	registryProxies.GET("", h.proxy.List)
	registryProxies.POST("", h.proxy.Deploy)
	registryProxies.PUT("/:id", h.proxy.Deploy)
	registryProxies.POST("/:id/cleanup", h.proxy.Cleanup)
	registryProxies.POST("/:id/diagnose", h.proxy.Diagnose)
	registryProxies.POST("/:id/migrate-resource-name", h.proxy.MigrateResourceName)

	chartRepositories := apiGroup.Group("/chart-repositories")
	chartRepositories.GET("", h.chart.List)
	chartRepositories.POST("", h.chart.Create)
	chartRepositories.PUT("/:id", h.chart.Update)
	chartRepositories.DELETE("/:id", h.chart.Delete)
	chartRepositories.POST("/:id/verify", h.chart.Verify)

	platform := apiGroup.Group("/platform")
	platform.GET("/status", h.platform.Status)
	platform.GET("/endpoint", h.platform.EndpointStatus)
	platform.PUT("/endpoint", h.platform.UpdateEndpoint)
	platform.POST("/endpoint/reconcile", h.platform.ReconcileEndpoint)
	platform.POST("/endpoint/adopt-ingress", h.platform.AdoptEndpointIngress)
	platform.POST("/releases", h.platform.ManualUpdate)
	platform.POST("/webhook-secret", h.platform.GenerateWebhookSecret)
	platform.PUT("/image-prefix", h.platform.UpdateImagePrefix)
	platform.POST("/releases/:id/rollback", h.platform.Rollback)
	r.POST("/api/platform/deployments", h.platform.Webhook)
}
