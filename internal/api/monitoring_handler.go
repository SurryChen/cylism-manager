package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

const maxPromQLLength = 2048

type monitoringQueryFunc func(context.Context, string, url.Values) (interface{}, error)

// MonitoringHandler manages the platform-owned VictoriaMetrics instance.
type MonitoringHandler struct {
	query monitoringQueryFunc
}

func NewMonitoringHandler() *MonitoringHandler {
	return &MonitoringHandler{query: queryVictoriaMetrics}
}

func (h *MonitoringHandler) Status(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	model.Success(c, K8s.VictoriaMetricsStatus())
}

func (h *MonitoringHandler) Install(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	var config k8s.VictoriaMetricsConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "VictoriaMetrics 配置无效")
		return
	}
	status, err := K8s.InstallVictoriaMetrics(config)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	model.SuccessWithMessage(c, status, "VictoriaMetrics 配置已提交")
}

func (h *MonitoringHandler) Uninstall(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	if err := K8s.UninstallVictoriaMetrics(); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeK8sAPIError, err.Error())
		return
	}
	model.SuccessWithMessage(c, gin.H{"data_retained": true}, "VictoriaMetrics 已卸载，本地数据目录未删除")
}

func (h *MonitoringHandler) Query(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	query := strings.TrimSpace(c.Query("query"))
	if query == "" || len(query) > maxPromQLLength {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "PromQL 查询不能为空且不能超过 2048 个字符")
		return
	}
	status := K8s.VictoriaMetricsStatus()
	if status.State != k8s.VictoriaMetricsStateReady {
		model.Error(c, http.StatusConflict, model.CodeConflict, "VictoriaMetrics 尚未就绪")
		return
	}
	result, err := h.query(c.Request.Context(), "/api/v1/query", url.Values{"query": []string{query}})
	if err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "查询 VictoriaMetrics 失败: "+err.Error())
		return
	}
	model.Success(c, result)
}

func (h *MonitoringHandler) Targets(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	status := K8s.VictoriaMetricsStatus()
	if status.State != k8s.VictoriaMetricsStateReady {
		model.Error(c, http.StatusConflict, model.CodeConflict, "VictoriaMetrics 尚未就绪")
		return
	}
	result, err := h.query(c.Request.Context(), "/api/v1/targets", nil)
	if err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "读取采集目标失败: "+err.Error())
		return
	}
	model.Success(c, result)
}

func queryVictoriaMetrics(ctx context.Context, path string, values url.Values) (interface{}, error) {
	requestURL := k8s.VictoriaMetricsServiceURL() + path
	if len(values) > 0 {
		requestURL += "?" + values.Encode()
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("VictoriaMetrics 返回 %s", response.Status)
	}
	var payload struct {
		Status    string      `json:"status"`
		Data      interface{} `json:"data"`
		ErrorType string      `json:"errorType"`
		Error     string      `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析 VictoriaMetrics 响应失败: %w", err)
	}
	if payload.Status != "success" {
		if payload.Error == "" {
			payload.Error = "VictoriaMetrics 查询未成功"
		}
		return nil, fmt.Errorf("%s", payload.Error)
	}
	return payload.Data, nil
}
