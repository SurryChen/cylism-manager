package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	maxLogRange       = 24 * time.Hour
	maxLogLimit       = 500
	defaultLogLimit   = 200
	maxLogKeywordSize = 256
)

var loggingRanges = map[string]time.Duration{
	"1h":  time.Hour,
	"6h":  6 * time.Hour,
	"24h": 24 * time.Hour,
}

type lokiQueryFunc func(context.Context, string, url.Values) (*lokiQueryResponse, error)

// LoggingHandler manages the platform-owned logging resources and proxies bounded queries.
type LoggingHandler struct {
	store *store.Store
	query lokiQueryFunc
	now   func() time.Time
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

type logPodFilter struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	NodeName   string   `json:"node_name,omitempty"`
	Containers []string `json:"containers"`
}

func NewLoggingHandler(stores ...*store.Store) *LoggingHandler {
	handler := &LoggingHandler{query: queryLoki, now: time.Now}
	if len(stores) > 0 {
		handler.store = stores[0]
	}
	return handler
}

func (h *LoggingHandler) Status(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	model.Success(c, K8s.LoggingStatus())
}

func (h *LoggingHandler) Install(c *gin.Context) {
	h.applyConfig(c, "日志采集配置已提交")
}

func (h *LoggingHandler) Update(c *gin.Context) {
	h.applyConfig(c, "日志采集运行配置已更新")
}

func (h *LoggingHandler) applyConfig(c *gin.Context, message string) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	var config k8s.LoggingConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "日志采集配置无效")
		return
	}
	status, err := K8s.InstallLogging(config)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	model.SuccessWithMessage(c, status, message)
}

func (h *LoggingHandler) Uninstall(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	if err := K8s.UninstallLogging(); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeK8sAPIError, err.Error())
		return
	}
	model.SuccessWithMessage(c, gin.H{"data_retained": true}, "日志采集已卸载，系统管理的 Loki 存储卷已保留")
}

// Filters returns small Kubernetes-derived lists for structured log search controls.
func (h *LoggingHandler) Filters(c *gin.Context) {
	if K8s == nil || K8s.Clientset == nil {
		k8sUnavailable(c)
		return
	}
	namespace := strings.TrimSpace(c.Query("namespace"))
	pods, err := K8s.Clientset.CoreV1().Pods(namespace).List(K8s.Ctx(), metav1.ListOptions{})
	if err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "读取日志筛选项失败: "+err.Error())
		return
	}
	nodes, err := K8s.Clientset.CoreV1().Nodes().List(K8s.Ctx(), metav1.ListOptions{})
	if err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "读取节点筛选项失败: "+err.Error())
		return
	}
	namespaceSet := map[string]struct{}{}
	podOptions := make([]logPodFilter, 0, len(pods.Items))
	for _, pod := range pods.Items {
		namespaceSet[pod.Namespace] = struct{}{}
		containers := make([]string, 0, len(pod.Spec.Containers))
		for _, container := range pod.Spec.Containers {
			containers = append(containers, container.Name)
		}
		sort.Strings(containers)
		podOptions = append(podOptions, logPodFilter{Name: pod.Name, Namespace: pod.Namespace, NodeName: pod.Spec.NodeName, Containers: containers})
	}
	sort.Slice(podOptions, func(i, j int) bool {
		if podOptions[i].Namespace != podOptions[j].Namespace {
			return podOptions[i].Namespace < podOptions[j].Namespace
		}
		return podOptions[i].Name < podOptions[j].Name
	})
	namespaces := make([]string, 0, len(namespaceSet))
	for item := range namespaceSet {
		namespaces = append(namespaces, item)
	}
	sort.Strings(namespaces)
	nodeNames := make([]string, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		nodeNames = append(nodeNames, node.Name)
	}
	sort.Strings(nodeNames)
	model.Success(c, gin.H{"namespaces": namespaces, "pods": podOptions, "nodes": nodeNames})
}

func (h *LoggingHandler) Query(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	if K8s.LoggingStatus().State != k8s.LoggingStateReady {
		model.Error(c, http.StatusConflict, model.CodeConflict, "日志采集尚未就绪")
		return
	}
	var request logQueryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "日志查询参数无效")
		return
	}
	rangeDuration, err := validateLogQuery(&request)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.applyApplicationScope(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	query, err := buildLogQL(request)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	end := h.now().UTC()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	result, err := h.query(ctx, "/loki/api/v1/query_range", url.Values{
		"query":     []string{query},
		"start":     []string{strconv.FormatInt(end.Add(-rangeDuration).Unix(), 10)},
		"end":       []string{strconv.FormatInt(end.Unix(), 10)},
		"limit":     []string{strconv.Itoa(request.Limit)},
		"direction": []string{"BACKWARD"},
	})
	if err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "查询 Loki 日志失败: "+err.Error())
		return
	}
	lines := normalizeLogLines(result, request.Limit)
	model.Success(c, gin.H{"range": request.Range, "lines": lines, "has_more": len(lines) >= request.Limit})
}

func validateLogQuery(request *logQueryRequest) (time.Duration, error) {
	request.Range = strings.TrimSpace(request.Range)
	if request.Range == "" {
		request.Range = "1h"
	}
	rangeDuration, ok := loggingRanges[request.Range]
	if !ok || rangeDuration > maxLogRange {
		return 0, fmt.Errorf("日志时间范围最多支持 24 小时")
	}
	if strings.TrimSpace(request.RawLogQL) != "" {
		return 0, fmt.Errorf("日志查询仅支持平台提供的筛选条件，不能提交原始 LogQL")
	}
	request.Keyword = strings.TrimSpace(request.Keyword)
	if len(request.Keyword) > maxLogKeywordSize || strings.ContainsAny(request.Keyword, "\r\n") {
		return 0, fmt.Errorf("日志关键字不能超过 256 个字符且不能包含换行")
	}
	if request.Limit == 0 {
		request.Limit = defaultLogLimit
	}
	if request.Limit < 1 || request.Limit > maxLogLimit {
		return 0, fmt.Errorf("单次日志查询最多返回 500 行")
	}
	for _, value := range []*string{&request.Namespace, &request.Pod, &request.Container, &request.Node, &request.Workload} {
		*value = strings.TrimSpace(*value)
		if len(*value) > 253 || strings.ContainsAny(*value, "\r\n") {
			return 0, fmt.Errorf("日志筛选值格式无效")
		}
	}
	return rangeDuration, nil
}

func (h *LoggingHandler) applyApplicationScope(request *logQueryRequest) error {
	if request.ApplicationID == 0 && request.EnvironmentID == 0 && request.ProjectID == 0 {
		return nil
	}
	if h.store == nil {
		return fmt.Errorf("日志应用范围校验不可用")
	}
	if request.ApplicationID != 0 {
		application, err := h.store.GetApplication(request.ApplicationID)
		if err != nil {
			return fmt.Errorf("应用不存在")
		}
		if request.ProjectID != 0 && request.ProjectID != application.ProjectID {
			return fmt.Errorf("应用不属于所选项目")
		}
		if request.EnvironmentID != 0 && request.EnvironmentID != application.EnvironmentID {
			return fmt.Errorf("应用不属于所选环境")
		}
		if request.Namespace != "" && request.Namespace != application.Environment.Namespace {
			return fmt.Errorf("应用与命名空间不匹配")
		}
		if request.Workload != "" && request.Workload != application.Name {
			return fmt.Errorf("应用与工作负载不匹配")
		}
		request.ProjectID = application.ProjectID
		request.EnvironmentID = application.EnvironmentID
		request.Namespace = application.Environment.Namespace
		request.Workload = application.Name
	}
	if request.EnvironmentID != 0 {
		environment, err := h.store.GetEnvironmentByID(request.EnvironmentID)
		if err != nil {
			return fmt.Errorf("环境不存在")
		}
		if request.ProjectID != 0 && request.ProjectID != environment.ProjectID {
			return fmt.Errorf("环境不属于所选项目")
		}
		if request.Namespace != "" && request.Namespace != environment.Namespace {
			return fmt.Errorf("环境与命名空间不匹配")
		}
		request.ProjectID = environment.ProjectID
		request.Namespace = environment.Namespace
	}
	if request.ProjectID != 0 {
		if _, err := h.store.GetProject(request.ProjectID); err != nil {
			return fmt.Errorf("项目不存在")
		}
	}
	return nil
}

func buildLogQL(request logQueryRequest) (string, error) {
	filters := []struct{ label, value string }{
		{"namespace", request.Namespace},
		{"pod", request.Pod},
		{"container", request.Container},
		{"node", request.Node},
		{"workload", request.Workload},
	}
	labels := make([]string, 0, len(filters))
	for _, filter := range filters {
		if filter.value == "" {
			continue
		}
		labels = append(labels, filter.label+`="`+escapeLogQL(filter.value)+`"`)
	}
	selector := "{" + strings.Join(labels, ",") + "}"
	if request.Keyword != "" {
		selector += ` |= "` + escapeLogQL(request.Keyword) + `"`
	}
	return selector, nil
}

func escapeLogQL(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", `"`, `\"`)
	return replacer.Replace(value)
}

func normalizeLogLines(response *lokiQueryResponse, limit int) []logLine {
	if response == nil {
		return []logLine{}
	}
	lines := make([]logLine, 0)
	for _, stream := range response.Data.Result {
		for _, value := range stream.Values {
			if len(value) < 2 {
				continue
			}
			labels := make(map[string]string, len(stream.Stream))
			for key, item := range stream.Stream {
				labels[key] = item
			}
			lines = append(lines, logLine{Timestamp: formatLokiTimestamp(value[0]), Line: value[1], Labels: labels})
		}
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].Timestamp > lines[j].Timestamp })
	if len(lines) > limit {
		return lines[:limit]
	}
	return lines
}

func formatLokiTimestamp(raw string) string {
	nanoseconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return raw
	}
	return time.Unix(0, nanoseconds).UTC().Format(time.RFC3339Nano)
}

func queryLoki(ctx context.Context, path string, values url.Values) (*lokiQueryResponse, error) {
	requestURL := k8s.LokiServiceURL() + path
	if len(values) > 0 {
		requestURL += "?" + values.Encode()
	}
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
		return nil, fmt.Errorf("Loki 返回 %s", response.Status)
	}
	var payload lokiQueryResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析 Loki 响应失败: %w", err)
	}
	if payload.Status != "success" {
		if payload.Error == "" {
			payload.Error = "Loki 查询未成功"
		}
		return nil, fmt.Errorf("%s", payload.Error)
	}
	return &payload, nil
}
