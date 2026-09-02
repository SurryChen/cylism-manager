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
	"github.com/cylism/cylism-manager/internal/repository"
	loggingservice "github.com/cylism/cylism-manager/internal/service/observability/logging"
	"github.com/gin-gonic/gin"
)

type lokiQueryFunc func(context.Context, string, url.Values) (*lokiQueryResponse, error)

func loggingQueryAdapter(fn lokiQueryFunc) loggingservice.QueryFunc {
	return func(ctx context.Context, path string, values url.Values) (loggingservice.Response, error) {
		result, err := fn(ctx, path, values)
		if err != nil {
			return loggingservice.Response{}, err
		}
		if result == nil {
			return loggingservice.Response{}, nil
		}
		streams := make([]loggingservice.Stream, 0, len(result.Data.Result))
		for _, stream := range result.Data.Result {
			streams = append(streams, loggingservice.Stream{Labels: stream.Stream, Values: stream.Values})
		}
		return loggingservice.Response{Streams: streams}, nil
	}
}

// LoggingHandler manages the platform-owned logging resources and proxies bounded queries.
type LoggingHandler struct {
	scope        repository.LoggingScopeRepository
	query        lokiQueryFunc
	now          func() time.Time
	component    *loggingservice.ComponentService
	filterReader loggingservice.FilterReader
	queryService *loggingservice.QueryService
	ready        func() bool
}

type LoggingDependencies struct {
	Query        loggingservice.QueryFunc
	Ready        func() bool
	Component    loggingservice.ComponentAdapter
	FilterReader loggingservice.FilterReader
}

type logQueryRequest struct {
	Range         string `json:"range"`
	Limit         int    `json:"limit"`
	Keyword       string `json:"keyword"`
	Namespace     string `json:"namespace"`
	Pod           string `json:"pod"`
	Container     string `json:"container"`
	Node          string `json:"node"`
	Workload      string `json:"workload"`
	StartTime     string `json:"start_time"`
	EndTime       string `json:"end_time"`
	ProjectID     uint   `json:"project_id"`
	EnvironmentID uint   `json:"environment_id"`
	ApplicationID uint   `json:"application_id"`
	RawLogQL      string `json:"logql"`
}

type lokiQueryResponse struct {
	Status string        `json:"status"`
	Data   lokiQueryData `json:"data"`
	Error  string        `json:"error"`
}

type lokiQueryData struct {
	ResultType string       `json:"resultType"`
	Result     []lokiStream `json:"result"`
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

type logLine struct {
	Timestamp string            `json:"timestamp"`
	Line      string            `json:"line"`
	Labels    map[string]string `json:"labels"`
}

func NewLoggingHandler(scope repository.LoggingScopeRepository, deps LoggingDependencies) *LoggingHandler {
	var component *loggingservice.ComponentService
	if deps.Component != nil {
		component = loggingservice.NewComponentService(deps.Component)
	}
	handler := &LoggingHandler{scope: scope, component: component, filterReader: deps.FilterReader, ready: deps.Ready, now: time.Now, query: func(ctx context.Context, path string, values url.Values) (*lokiQueryResponse, error) {
		if deps.Query == nil {
			return nil, fmt.Errorf("日志查询不可用")
		}
		result, err := deps.Query(ctx, path, values)
		if err != nil {
			return nil, err
		}
		response := &lokiQueryResponse{Status: "success", Data: lokiQueryData{ResultType: "streams"}}
		for _, stream := range result.Streams {
			response.Data.Result = append(response.Data.Result, lokiStream{Stream: stream.Labels, Values: stream.Values})
		}
		return response, nil
	}}
	handler.configureQueryService()
	return handler
}

func (h *LoggingHandler) configureQueryService() {
	h.queryService = loggingservice.NewQueryService(loggingQueryAdapter(func(ctx context.Context, path string, values url.Values) (*lokiQueryResponse, error) {
		return h.query(ctx, path, values)
	}), h.ready, h.scope)
}

func (h *LoggingHandler) Status(c *gin.Context) {
	if h.component == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	model.Success(c, h.component.Status(c.Request.Context()))
}

func (h *LoggingHandler) Install(c *gin.Context) {
	h.applyConfig(c, "日志采集配置已提交")
}

func (h *LoggingHandler) Update(c *gin.Context) {
	h.applyConfig(c, "日志采集运行配置已更新")
}

func (h *LoggingHandler) applyConfig(c *gin.Context, message string) {
	if h.component == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	var config k8s.LoggingConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		apiShared.BadRequest(c, "日志采集配置无效")
		return
	}
	status, err := h.component.Install(c.Request.Context(), config)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	model.SuccessWithMessage(c, status, message)
}

func (h *LoggingHandler) Uninstall(c *gin.Context) {
	if h.component == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	if err := h.component.Uninstall(c.Request.Context()); err != nil {
		apiShared.Error(c, http.StatusInternalServerError, model.CodeK8sAPIError, err.Error())
		return
	}
	model.SuccessWithMessage(c, gin.H{"data_retained": true}, "日志采集已卸载，系统管理的 Loki 存储卷已保留")
}

// Filters returns small Kubernetes-derived lists for structured log search controls.
func (h *LoggingHandler) Filters(c *gin.Context) {
	if h.filterReader == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	namespace := strings.TrimSpace(c.Query("namespace"))
	filters, err := loggingservice.Filters(c.Request.Context(), namespace, h.filterReader)
	if err != nil {
		apiShared.K8sAPIError(c, "读取日志筛选项失败: "+err.Error())
		return
	}
	model.Success(c, gin.H{"namespaces": filters.Namespaces, "pods": filters.Pods, "nodes": filters.Nodes})
}

func (h *LoggingHandler) Query(c *gin.Context) {
	if h.queryService == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	var request logQueryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "日志查询参数无效")
		return
	}
	serviceRequest := loggingservice.QueryRequest{ApplicationID: request.ApplicationID, EnvironmentID: request.EnvironmentID, ProjectID: request.ProjectID, Range: request.Range, Limit: request.Limit, Keyword: request.Keyword, Namespace: request.Namespace, Pod: request.Pod, Container: request.Container, Node: request.Node, Workload: request.Workload, StartTime: request.StartTime, EndTime: request.EndTime, RawLogQL: request.RawLogQL}
	serviceLines, err := h.queryService.Query(c.Request.Context(), serviceRequest, h.now().UTC())
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "日志") || strings.Contains(err.Error(), "时间") || strings.Contains(err.Error(), "应用") || strings.Contains(err.Error(), "环境") || strings.Contains(err.Error(), "项目") {
			status = http.StatusBadRequest
		}
		if strings.Contains(err.Error(), "尚未就绪") {
			status = http.StatusConflict
		}
		apiShared.Error(c, status, model.CodeK8sAPIError, err.Error())
		return
	}
	lines := make([]logLine, 0, len(serviceLines))
	for _, line := range serviceLines {
		lines = append(lines, logLine{Timestamp: line.Timestamp, Line: line.Line, Labels: line.Labels})
	}
	rangeName, limit := strings.TrimSpace(request.Range), request.Limit
	if rangeName == "" {
		rangeName = "1h"
	}
	if limit == 0 {
		limit = loggingservice.DefaultLimit
	}
	model.Success(c, gin.H{"range": rangeName, "lines": lines, "has_more": len(lines) >= limit})
}
