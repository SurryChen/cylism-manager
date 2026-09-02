package system

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	monitoringservice "github.com/cylism/cylism-manager/internal/service/observability/monitoring"
	"github.com/gin-gonic/gin"
)

type monitoringQueryFunc func(context.Context, string, url.Values) (interface{}, error)

// MonitoringHandler manages the platform-owned VictoriaMetrics instance.
type MonitoringHandler struct {
	query        monitoringQueryFunc
	queryService *monitoringservice.QueryService
	component    *monitoringservice.ComponentService
	consumers    monitoringservice.ConsumerReader
	status       monitoringservice.StatusReader
}

type MonitoringDependencies struct {
	Query     monitoringservice.QueryFunc
	Status    monitoringservice.StatusReader
	Component monitoringservice.ComponentAdapter
	Consumers monitoringservice.ConsumerReader
}

func NewMonitoringHandler(deps MonitoringDependencies) *MonitoringHandler {
	var component *monitoringservice.ComponentService
	if deps.Component != nil {
		component = monitoringservice.NewComponentService(deps.Component)
	}
	handler := &MonitoringHandler{component: component, consumers: deps.Consumers, status: deps.Status}
	handler.query = monitoringQueryFunc(deps.Query)
	handler.queryService = monitoringservice.NewQueryService(func(ctx context.Context, path string, values url.Values) (interface{}, error) {
		if handler.query == nil {
			return nil, fmt.Errorf("VictoriaMetrics 查询不可用")
		}
		return handler.query(ctx, path, values)
	}, deps.Status)
	return handler
}

// WithQuery replaces the VictoriaMetrics transport for focused handler tests
// and for embedders that provide their own transport.
func (h *MonitoringHandler) WithQuery(query monitoringQueryFunc) *MonitoringHandler {
	h.query = query
	h.queryService = monitoringservice.NewQueryService(monitoringservice.QueryFunc(query), h.status)
	return h
}

func (h *MonitoringHandler) Status(c *gin.Context) {
	if h.component == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	model.Success(c, h.component.Status(c.Request.Context()))
}

func (h *MonitoringHandler) Install(c *gin.Context) {
	if h.component == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	var config k8s.VictoriaMetricsConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		apiShared.BadRequest(c, "VictoriaMetrics 配置无效")
		return
	}
	status, err := h.component.Install(c.Request.Context(), config)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	model.SuccessWithMessage(c, status, "VictoriaMetrics 配置已提交")
}

func (h *MonitoringHandler) Uninstall(c *gin.Context) {
	if h.component == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	if err := h.component.Uninstall(c.Request.Context()); err != nil {
		apiShared.Error(c, http.StatusInternalServerError, model.CodeK8sAPIError, err.Error())
		return
	}
	model.SuccessWithMessage(c, gin.H{"data_retained": true}, "VictoriaMetrics 已卸载，系统管理的存储卷已保留")
}

func (h *MonitoringHandler) MigrateLegacyStorage(c *gin.Context) {
	if h.component == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	var request k8s.VictoriaMetricsMigrationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "迁移存储配置无效")
		return
	}
	status, err := h.component.Migrate(c.Request.Context(), request)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	model.SuccessWithMessage(c, status, "已停止 VictoriaMetrics，正在复制并校验历史数据")
}

func (h *MonitoringHandler) Query(c *gin.Context) {
	if h.queryService == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	result, err := h.queryService.Query(c.Request.Context(), c.Query("query"))
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "PromQL") {
			status = http.StatusBadRequest
		}
		if strings.Contains(err.Error(), "尚未就绪") {
			status = http.StatusConflict
		}
		apiShared.Error(c, status, model.CodeK8sAPIError, "查询 VictoriaMetrics 失败: "+err.Error())
		return
	}
	model.Success(c, result)
}

// QueryRange exposes a bounded set of history windows for dashboard charts.
func (h *MonitoringHandler) QueryRange(c *gin.Context) {
	if h.queryService == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	rangeName := c.DefaultQuery("range", "6h")
	result, err := h.queryService.QueryRange(c.Request.Context(), c.Query("query"), rangeName, time.Now().UTC())
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "PromQL") || strings.Contains(err.Error(), "时间范围") {
			status = http.StatusBadRequest
		}
		if strings.Contains(err.Error(), "尚未就绪") {
			status = http.StatusConflict
		}
		apiShared.Error(c, status, model.CodeK8sAPIError, "查询 VictoriaMetrics 历史指标失败: "+err.Error())
		return
	}
	model.Success(c, result)
}

// Dashboard batches the four default node trend queries into one browser
// request while preserving concurrent reads against VictoriaMetrics.
func (h *MonitoringHandler) Dashboard(c *gin.Context) {
	if h.queryService == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	rangeName := c.DefaultQuery("range", "6h")
	trends, err := h.queryService.Dashboard(c.Request.Context(), rangeName, time.Now().UTC())
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "时间范围") {
			status = http.StatusBadRequest
		} else if strings.Contains(err.Error(), "尚未就绪") {
			status = http.StatusConflict
		}
		apiShared.Error(c, status, model.CodeK8sAPIError, "查询 VictoriaMetrics 趋势指标失败: "+err.Error())
		return
	}
	model.Success(c, gin.H{"range": rangeName, "trends": trends})
}

// DiskGrowth ranks positive filesystem growth from the metrics already
// collected by the managed VictoriaMetrics instance. The browser selects only
// a bounded window and optional node; all PromQL is controlled here.
func (h *MonitoringHandler) DiskGrowth(c *gin.Context) {
	if h.queryService == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	rangeName := c.DefaultQuery("range", "6h")
	node := strings.TrimSpace(c.Query("node"))
	data, err := h.queryService.DiskGrowth(c.Request.Context(), rangeName, node, h.consumers)
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "时间范围") {
			status = http.StatusBadRequest
		} else if strings.Contains(err.Error(), "节点名称无效") || strings.Contains(err.Error(), "尚未就绪") {
			status = http.StatusBadRequest
			if strings.Contains(err.Error(), "尚未就绪") {
				status = http.StatusConflict
			}
		}
		apiShared.Error(c, status, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, data)
}

func (h *MonitoringHandler) Targets(c *gin.Context) {
	if h.queryService == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	result, err := h.queryService.Targets(c.Request.Context())
	if err != nil {
		apiShared.K8sAPIError(c, "读取采集目标失败: "+err.Error())
		return
	}
	model.Success(c, result)
}
