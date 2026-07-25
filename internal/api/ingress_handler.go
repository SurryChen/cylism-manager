package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/cylism/cylism-manager/internal/model"
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
		model.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, routes)
}

// CreateRoute 创建 IngressRoute
func (h *IngressHandler) CreateRoute(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	model.SuccessWithMessage(c, nil, "创建 IngressRoute - 待实现")
}

// UpdateRoute 更新 IngressRoute
func (h *IngressHandler) UpdateRoute(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	model.SuccessWithMessage(c, nil, "更新 IngressRoute - 待实现")
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
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
		return
	}
	model.SuccessWithMessage(c, nil, "删除成功")
}

// ListMiddlewares 列出 Middleware
func (h *IngressHandler) ListMiddlewares(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	model.SuccessWithMessage(c, nil, "Middleware - 待实现")
}

// ListTLSStores 列出 TLS Store
func (h *IngressHandler) ListTLSStores(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	model.SuccessWithMessage(c, nil, "TLS Store - 待实现")
}
