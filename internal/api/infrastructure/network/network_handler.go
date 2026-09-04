package network

import (
	"net/http"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	networkservice "github.com/cylism/cylism-manager/internal/service/network"
	"github.com/gin-gonic/gin"
)

// NetworkHandler is the infrastructure HTTP boundary for domains,
// certificates, ingress and CoreDNS policy endpoints. Business workflows are
// provided by the network service and existing compatibility executors.
type NetworkHandler struct {
	Service                                                                                *networkservice.Service
	RouteList, RouteCreate, RouteUpdate, RouteDelete, RouteListMiddlewares, RouteTLSStores gin.HandlerFunc
	DNSStatus, DNSApply, DNSReset, DNSRollback                                             gin.HandlerFunc
	IngressList, IngressGet, IngressCreate, IngressDelete, IngressController               gin.HandlerFunc
}

func (h *NetworkHandler) ListStandardIngresses(c *gin.Context) {
	result, err := h.Service.ListStandardIngressesContext(c.Request.Context(), c.Query("namespace"))
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	if result == nil {
		result = []k8sclient.IngressStdInfo{}
	}
	apiShared.Success(c, result)
}

func (h *NetworkHandler) GetStandardIngress(c *gin.Context) {
	result, err := h.Service.GetStandardIngressContext(c.Request.Context(), c.Param("namespace"), c.Param("name"))
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	apiShared.Success(c, result)
}

func (h *NetworkHandler) CreateStandardIngress(c *gin.Context) {
	var req struct {
		Namespace   string `json:"namespace"`
		Name        string `json:"name"`
		Host        string `json:"host"`
		Path        string `json:"path"`
		ServiceName string `json:"service_name"`
		ServicePort string `json:"service_port"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "invalid request")
		return
	}
	result, err := h.Service.CreateStandardIngressContext(c.Request.Context(), req.Namespace, req.Name, req.Host, req.Path, req.ServiceName, req.ServicePort)
	if err != nil {
		apiShared.BadRequest(c, err.Error())
		return
	}
	apiShared.Success(c, result)
}

func (h *NetworkHandler) DeleteStandardIngress(c *gin.Context) {
	if err := h.Service.DeleteStandardIngressContext(c.Request.Context(), c.Param("namespace"), c.Param("name")); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, nil, "删除成功")
}

func (h *NetworkHandler) StandardIngressController(c *gin.Context) {
	status, err := h.Service.DetectIngressControllerContext(c.Request.Context())
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	apiShared.Success(c, status)
}

func NewNetworkHandler(service *networkservice.Service, h NetworkHandler) *NetworkHandler {
	h.Service = service
	return &h
}
