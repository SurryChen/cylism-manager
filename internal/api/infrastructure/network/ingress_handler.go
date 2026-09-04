package network

import (
	"net/http"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	networkservice "github.com/cylism/cylism-manager/internal/service/network"
	"github.com/gin-gonic/gin"
)

// IngressHandler exposes Traefik IngressRoute resources as an infrastructure
// HTTP adapter. Kubernetes operations remain behind api.K8s for compatibility
// with the existing client initialization path.
type IngressHandler struct {
	service *networkservice.Service
}

func NewIngressHandler(service *networkservice.Service) *IngressHandler {
	return &IngressHandler{service: service}
}

func (h *IngressHandler) ListRoutes(c *gin.Context) {
	if h.service == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	routes, err := h.service.ListIngressRoutesContext(c.Request.Context())
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	apiShared.Success(c, routes)
}

func (h *IngressHandler) CreateRoute(c *gin.Context) {
	if h.service == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	apiShared.SuccessWithMessage(c, nil, "创建 IngressRoute - 待实现")
}

func (h *IngressHandler) UpdateRoute(c *gin.Context) {
	if h.service == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	apiShared.SuccessWithMessage(c, nil, "更新 IngressRoute - 待实现")
}

func (h *IngressHandler) DeleteRoute(c *gin.Context) {
	if h.service == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	if err := h.service.DeleteIngressRouteContext(c.Request.Context(), c.Param("namespace"), c.Param("name")); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, nil, "删除成功")
}

func (h *IngressHandler) ListMiddlewares(c *gin.Context) {
	if h.service == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	apiShared.SuccessWithMessage(c, nil, "Middleware - 待实现")
}

func (h *IngressHandler) ListTLSStores(c *gin.Context) {
	if h.service == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	apiShared.SuccessWithMessage(c, nil, "TLS Store - 待实现")
}
