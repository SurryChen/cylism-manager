package infrastructure

import (
	"net/http"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	networkservice "github.com/cylism/cylism-manager/internal/service/network"
	"github.com/gin-gonic/gin"
)

// IngressHandler exposes Traefik IngressRoute resources as an infrastructure
// HTTP adapter. Kubernetes operations remain behind api.K8s for compatibility
// with the existing client initialization path.
type IngressHandler struct {
	client  *k8s.Client
	service *networkservice.Service
}

func NewIngressHandler(client *k8s.Client) *IngressHandler {
	return &IngressHandler{client: client, service: networkservice.NewService(nil).WithIngressAdapter(client)}
}

func (h *IngressHandler) ListRoutes(c *gin.Context) {
	if h.client == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	routes, err := h.service.ListIngressRoutes()
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, routes)
}

func (h *IngressHandler) CreateRoute(c *gin.Context) {
	if h.client == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	model.SuccessWithMessage(c, nil, "创建 IngressRoute - 待实现")
}

func (h *IngressHandler) UpdateRoute(c *gin.Context) {
	if h.client == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	model.SuccessWithMessage(c, nil, "更新 IngressRoute - 待实现")
}

func (h *IngressHandler) DeleteRoute(c *gin.Context) {
	if h.client == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	if err := h.service.DeleteIngressRoute(c.Param("namespace"), c.Param("name")); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	model.SuccessWithMessage(c, nil, "删除成功")
}

func (h *IngressHandler) ListMiddlewares(c *gin.Context) {
	if h.client == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	model.SuccessWithMessage(c, nil, "Middleware - 待实现")
}

func (h *IngressHandler) ListTLSStores(c *gin.Context) {
	if h.client == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	model.SuccessWithMessage(c, nil, "TLS Store - 待实现")
}
