package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// IngressHandler IngressRoute 管理的 HTTP handler
type IngressHandler struct{}

// NewIngressHandler 创建 IngressHandler
func NewIngressHandler() *IngressHandler {
	return &IngressHandler{}
}

// ListRoutes 列出所有 IngressRoute
func (h *IngressHandler) ListRoutes(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	routes, err := K8s.ListIngressRoutes()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "routes": []interface{}{}})
		return
	}
	c.JSON(http.StatusOK, routes)
}

// CreateRoute 创建 IngressRoute
func (h *IngressHandler) CreateRoute(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "创建 IngressRoute - 待实现"})
}

// UpdateRoute 更新 IngressRoute
func (h *IngressHandler) UpdateRoute(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "更新 IngressRoute - 待实现"})
}

// DeleteRoute 删除 IngressRoute
func (h *IngressHandler) DeleteRoute(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	if err := K8s.DeleteIngressRoute(ns, name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// ListMiddlewares 列出 Middleware
func (h *IngressHandler) ListMiddlewares(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Middleware - 待实现", "data": []interface{}{}})
}

// ListTLSStores 列出 TLS Store
func (h *IngressHandler) ListTLSStores(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "TLS Store - 待实现", "data": []interface{}{}})
}
