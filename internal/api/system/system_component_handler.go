package system

import (
	"context"
	"net/http"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	systemcomponentservice "github.com/cylism/cylism-manager/internal/service/system_component"
	"github.com/gin-gonic/gin"
)

type SystemComponentHandler struct {
	configs     repository.SystemComponentRepository
	adapter     SystemComponentAdapter
	listService *systemcomponentservice.ComponentListService
	service     *systemcomponentservice.ComponentService
}

// SystemComponentAdapter is the service boundary used by the HTTP handler.
// The concrete Kubernetes implementation lives in internal/k8s.
type SystemComponentAdapter = systemcomponentservice.KubernetesAdapter

// NewSystemComponentHandlerWithComposedDependencies is the Bootstrap entry
// point for the singleton component Handler.
func NewSystemComponentHandlerWithComposedDependencies(configs repository.SystemComponentRepository, adapter SystemComponentAdapter, service *systemcomponentservice.ComponentService, listService *systemcomponentservice.ComponentListService) *SystemComponentHandler {
	return &SystemComponentHandler{configs: configs, adapter: adapter, listService: listService, service: service}
}

// WithAdapter replaces the Kubernetes boundary for focused handler tests.
func (h *SystemComponentHandler) WithAdapter(adapter SystemComponentAdapter) *SystemComponentHandler {
	h.adapter = adapter
	h.listService = &systemcomponentservice.ComponentListService{Repo: h.configs, Adapter: adapter}
	h.service = systemcomponentservice.NewComponentService(h.configs, adapter, h.listService)
	return h
}

type systemComponentUpdateRequest struct {
	ValuesContent      string  `json:"values_content"`
	TraefikReadTimeout *string `json:"traefik_read_timeout"`
}

func (h *SystemComponentHandler) List(c *gin.Context) {
	if h.listService == nil {
		apiShared.Error(c, http.StatusServiceUnavailable, model.CodeK8sUnavailable, "Kubernetes 集群未连接")
		return
	}
	result, err := h.listService.List(c.Request.Context())
	if err != nil {
		apiShared.Error(c, http.StatusServiceUnavailable, model.CodeK8sUnavailable, err.Error())
		return
	}
	model.Success(c, result)
}

func (h *SystemComponentHandler) Update(c *gin.Context) {
	chart := strings.TrimSpace(c.Param("chart"))
	var req systemComponentUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.ValidationError(c, "请求参数无效")
		return
	}
	timeout := ""
	if req.TraefikReadTimeout != nil {
		timeout = *req.TraefikReadTimeout
	}
	result, err := h.service.Update(c.Request.Context(), chart, req.ValuesContent, timeout, apiShared.UserID(c))
	if err != nil {
		status, code := workflowHTTPError(err)
		apiShared.Error(c, status, code, err.Error())
		return
	}
	model.SuccessWithMessage(c, result.Config, "系统组件配置已应用")
}

func workflowHTTPError(err error) (int, int) {
	status, code := http.StatusBadGateway, model.CodeK8sAPIError
	if we, ok := err.(*systemcomponentservice.WorkflowError); ok {
		switch we.Kind {
		case "validation":
			status, code = http.StatusBadRequest, model.CodeValidationFail
		case "conflict":
			status, code = http.StatusConflict, model.CodeValidationFail
		case "unavailable":
			status, code = http.StatusServiceUnavailable, model.CodeK8sUnavailable
		case "db":
			status, code = http.StatusInternalServerError, model.CodeDBError
		}
	}
	return status, code
}

func (h *SystemComponentHandler) Revert(c *gin.Context) {
	chart := strings.TrimSpace(c.Param("chart"))
	revertErr := h.service.Revert(c.Request.Context(), chart)
	if revertErr != nil {
		status, code := workflowHTTPError(revertErr)
		apiShared.Error(c, status, code, "恢复系统组件默认配置失败: "+revertErr.Error())
		return
	}
	model.SuccessWithMessage(c, nil, "已恢复系统组件默认配置")
}

// Run exposes the lifecycle to the application startup layer while keeping
// reconciliation implementation in the system-component service.
func (h *SystemComponentHandler) Run(ctx context.Context, interval time.Duration) error {
	if h.adapter == nil || !h.adapter.Available() {
		return nil
	}
	return h.service.Run(ctx, interval)
}
