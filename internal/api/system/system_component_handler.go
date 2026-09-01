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
}

func NewSystemComponentHandler(configs repository.SystemComponentRepository, adapter SystemComponentAdapter) *SystemComponentHandler {
	return &SystemComponentHandler{configs: configs, adapter: adapter, listService: &systemcomponentservice.ComponentListService{Repo: configs, Adapter: adapter}}
}

// WithAdapter replaces the Kubernetes boundary for focused handler tests.
func (h *SystemComponentHandler) WithAdapter(adapter SystemComponentAdapter) *SystemComponentHandler {
	h.adapter = adapter
	h.listService = &systemcomponentservice.ComponentListService{Repo: h.configs, Adapter: adapter}
	return h
}

type systemComponentUpdateRequest struct {
	ValuesContent      string  `json:"values_content"`
	TraefikReadTimeout *string `json:"traefik_read_timeout"`
}

func (h *SystemComponentHandler) List(c *gin.Context) {
	if h.listService == nil {
		model.Error(c, http.StatusServiceUnavailable, model.CodeK8sUnavailable, "Kubernetes 集群未连接")
		return
	}
	result, err := h.listService.List(c.Request.Context())
	if err != nil {
		model.Error(c, http.StatusServiceUnavailable, model.CodeK8sUnavailable, err.Error())
		return
	}
	model.Success(c, result)
}

func (h *SystemComponentHandler) Update(c *gin.Context) {
	chart := strings.TrimSpace(c.Param("chart"))
	var req systemComponentUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "请求参数无效")
		return
	}
	timeout := ""
	if req.TraefikReadTimeout != nil {
		timeout = *req.TraefikReadTimeout
	}
	result, err := systemcomponentservice.Update(c.Request.Context(), h.configs, h.adapter, chart, req.ValuesContent, timeout, apiShared.UserID(c), time.Now())
	if err != nil {
		status, code := workflowHTTPError(err)
		model.Error(c, status, code, err.Error())
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
	revertErr := systemcomponentservice.RevertManaged(c.Request.Context(), h.configs, h.adapter, chart)
	if revertErr != nil {
		status, code := workflowHTTPError(revertErr)
		model.Error(c, status, code, "恢复系统组件默认配置失败: "+revertErr.Error())
		return
	}
	model.SuccessWithMessage(c, nil, "已恢复系统组件默认配置")
}

// Reconcile re-applies stored static Deployment fields after a K3s manifest
// re-render. It re-detects every component first and stops if its control source
// changed, avoiding a fight with helm-controller or a user-installed workload.
func (h *SystemComponentHandler) Reconcile() {
	_ = h.Run(context.Background(), 5*time.Minute)
}

// Run exposes the lifecycle to the application startup layer while keeping
// reconciliation implementation in the system-component service.
func (h *SystemComponentHandler) Run(ctx context.Context, interval time.Duration) error {
	if h.adapter == nil || !h.adapter.Available() {
		return nil
	}
	return systemcomponentservice.Run(ctx, interval, h.configs, h.adapter, systemcomponentservice.ParseStaticDeploymentConfig, func(ctx context.Context, node string) error {
		return systemcomponentservice.ValidateNode(ctx, h.adapter, node)
	}, time.Now)
}
