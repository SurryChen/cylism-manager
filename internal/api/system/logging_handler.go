package system

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

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
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
	maxLogQueryTerms  = 16
	maxLogQueryGroups = 8
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
	if k8sClient == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	model.Success(c, k8sClient.LoggingStatus())
}

func (h *LoggingHandler) Install(c *gin.Context) {
	h.applyConfig(c, "日志采集配置已提交")
}

func (h *LoggingHandler) Update(c *gin.Context) {
	h.applyConfig(c, "日志采集运行配置已更新")
}

func (h *LoggingHandler) applyConfig(c *gin.Context, message string) {
	if k8sClient == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	var config k8s.LoggingConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "日志采集配置无效")
		return
	}
	status, err := k8sClient.InstallLogging(config)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	model.SuccessWithMessage(c, status, message)
}

func (h *LoggingHandler) Uninstall(c *gin.Context) {
	if k8sClient == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	if err := k8sClient.UninstallLogging(); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeK8sAPIError, err.Error())
		return
	}
	model.SuccessWithMessage(c, gin.H{"data_retained": true}, "日志采集已卸载，系统管理的 Loki 存储卷已保留")
}

// Filters returns small Kubernetes-derived lists for structured log search controls.
func (h *LoggingHandler) Filters(c *gin.Context) {
	if k8sClient == nil || k8sClient.Clientset == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	namespace := strings.TrimSpace(c.Query("namespace"))
	pods, err := k8sClient.Clientset.CoreV1().Pods(namespace).List(k8sClient.Ctx(), metav1.ListOptions{})
	if err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "读取日志筛选项失败: "+err.Error())
		return
	}
	nodes, err := k8sClient.Clientset.CoreV1().Nodes().List(k8sClient.Ctx(), metav1.ListOptions{})
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
	if k8sClient == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	if status := k8sClient.LoggingStatus(); status.LokiReady < 1 {
		model.Error(c, http.StatusConflict, model.CodeConflict, "日志采集尚未就绪")
		return
	}
	var request logQueryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "日志查询参数无效")
		return
	}
	if err := validateLogQuery(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.applyApplicationScope(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	queries, err := buildLogQLQueries(request)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	end := h.now().UTC()
	start, end, err := resolveLogQueryBounds(request, end)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	results := make([]*lokiQueryResponse, 0, len(queries))
	for _, query := range queries {
		result, queryErr := h.query(ctx, "/loki/api/v1/query_range", url.Values{
			"query":     []string{query},
			"start":     []string{strconv.FormatInt(start.UnixNano(), 10)},
			"end":       []string{strconv.FormatInt(end.UnixNano(), 10)},
			"limit":     []string{strconv.Itoa(request.Limit)},
			"direction": []string{"BACKWARD"},
		})
		if queryErr != nil {
			model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "查询 Loki 日志失败: "+queryErr.Error())
			return
		}
		results = append(results, result)
	}
	lines := normalizeLogLines(results, request.Limit)
	model.Success(c, gin.H{"range": request.Range, "lines": lines, "has_more": len(lines) >= request.Limit})
}

func validateLogQuery(request *logQueryRequest) error {
	request.Range = strings.TrimSpace(request.Range)
	if request.Range == "" {
		request.Range = "1h"
	}
	if request.Range != "custom" {
		rangeDuration, ok := loggingRanges[request.Range]
		if !ok || rangeDuration > maxLogRange {
			return fmt.Errorf("日志时间范围最多支持 24 小时")
		}
	}
	if strings.TrimSpace(request.RawLogQL) != "" {
		return fmt.Errorf("日志查询仅支持平台提供的筛选条件，不能提交原始 LogQL")
	}
	request.Keyword = strings.TrimSpace(request.Keyword)
	if len(request.Keyword) > maxLogKeywordSize || strings.ContainsAny(request.Keyword, "\r\n") {
		return fmt.Errorf("日志关键字不能超过 256 个字符且不能包含换行")
	}
	if request.Limit == 0 {
		request.Limit = defaultLogLimit
	}
	if request.Limit < 1 || request.Limit > maxLogLimit {
		return fmt.Errorf("单次日志查询最多返回 500 行")
	}
	for _, value := range []*string{&request.Namespace, &request.Pod, &request.Container, &request.Node, &request.Workload} {
		*value = strings.TrimSpace(*value)
		if len(*value) > 253 || strings.ContainsAny(*value, "\r\n") {
			return fmt.Errorf("日志筛选值格式无效")
		}
	}
	request.StartTime = strings.TrimSpace(request.StartTime)
	request.EndTime = strings.TrimSpace(request.EndTime)
	return nil
}

func resolveLogQueryBounds(request logQueryRequest, now time.Time) (time.Time, time.Time, error) {
	if request.StartTime != "" || request.EndTime != "" {
		if request.StartTime == "" || request.EndTime == "" {
			return time.Time{}, time.Time{}, fmt.Errorf("精确时间范围必须同时填写开始和结束时间")
		}
		start, err := time.Parse(time.RFC3339Nano, request.StartTime)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("开始时间格式无效")
		}
		end, err := time.Parse(time.RFC3339Nano, request.EndTime)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("结束时间格式无效")
		}
		if !end.After(start) {
			return time.Time{}, time.Time{}, fmt.Errorf("结束时间必须晚于开始时间")
		}
		if end.Sub(start) > maxLogRange {
			return time.Time{}, time.Time{}, fmt.Errorf("日志时间范围最多支持 24 小时")
		}
		return start.UTC(), end.UTC(), nil
	}
	if request.Range == "custom" {
		return time.Time{}, time.Time{}, fmt.Errorf("精确时间范围必须同时填写开始和结束时间")
	}
	return now.Add(-loggingRanges[request.Range]), now, nil
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
	queries, err := buildLogQLQueries(request)
	if err != nil {
		return "", err
	}
	if len(queries) != 1 {
		return "", fmt.Errorf("日志表达式包含多个 OR 分支")
	}
	return queries[0], nil
}

func buildLogQLQueries(request logQueryRequest) ([]string, error) {
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
	if len(labels) == 0 {
		// Loki rejects an empty selector; every Kubernetes Pod log has a namespace label.
		labels = append(labels, `namespace=~".+"`)
	}
	selector := "{" + strings.Join(labels, ",") + "}"
	branches, err := parseLogKeywordExpression(request.Keyword)
	if err != nil {
		return nil, err
	}
	queries := make([]string, 0, len(branches))
	for _, branch := range branches {
		query := selector
		for _, term := range branch {
			query += ` |= "` + escapeLogQL(term) + `"`
		}
		queries = append(queries, query)
	}
	return queries, nil
}

func escapeLogQL(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", `"`, `\"`)
	return replacer.Replace(value)
}

type logExpressionTokenType int

const (
	logExpressionTerm logExpressionTokenType = iota
	logExpressionAnd
	logExpressionOr
)

type logExpressionToken struct {
	kind   logExpressionTokenType
	value  string
	quoted bool
}

// parseLogKeywordExpression accepts only a bounded OR-of-ANDs expression.
func parseLogKeywordExpression(raw string) ([][]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return [][]string{{}}, nil
	}
	tokens, hasOperator, err := tokenizeLogExpression(raw)
	if err != nil {
		return nil, err
	}
	if !hasOperator {
		if len(tokens) == 1 && tokens[0].quoted {
			return [][]string{{tokens[0].value}}, nil
		}
		return [][]string{{raw}}, nil
	}

	branches := make([][]string, 1)
	expectingTerm := true
	termCount := 0
	for _, token := range tokens {
		if expectingTerm {
			if token.kind != logExpressionTerm {
				return nil, fmt.Errorf("日志表达式缺少关键字")
			}
			if strings.ContainsAny(token.value, "()") {
				return nil, fmt.Errorf("日志表达式暂不支持括号")
			}
			termCount++
			if termCount > maxLogQueryTerms {
				return nil, fmt.Errorf("日志表达式最多支持 %d 个关键词", maxLogQueryTerms)
			}
			branches[len(branches)-1] = append(branches[len(branches)-1], token.value)
			expectingTerm = false
			continue
		}
		switch token.kind {
		case logExpressionAnd:
			expectingTerm = true
		case logExpressionOr:
			if len(branches) >= maxLogQueryGroups {
				return nil, fmt.Errorf("日志表达式最多支持 %d 个 OR 分支", maxLogQueryGroups)
			}
			branches = append(branches, nil)
			expectingTerm = true
		default:
			return nil, fmt.Errorf("日志表达式中的关键词之间需要使用 AND 或 OR")
		}
	}
	if expectingTerm {
		return nil, fmt.Errorf("日志表达式不能以 AND 或 OR 结束")
	}
	return branches, nil
}

func tokenizeLogExpression(raw string) ([]logExpressionToken, bool, error) {
	tokens := make([]logExpressionToken, 0)
	hasOperator := false
	for offset := 0; offset < len(raw); {
		for offset < len(raw) && (raw[offset] == ' ' || raw[offset] == '\t') {
			offset++
		}
		if offset == len(raw) {
			break
		}
		if raw[offset] == '"' {
			value, next, err := readQuotedLogTerm(raw, offset)
			if err != nil {
				return nil, false, err
			}
			tokens = append(tokens, logExpressionToken{kind: logExpressionTerm, value: value, quoted: true})
			offset = next
			continue
		}
		start := offset
		for offset < len(raw) && raw[offset] != ' ' && raw[offset] != '\t' {
			if raw[offset] == '"' {
				return nil, false, fmt.Errorf("日志表达式中的引号必须独立包裹字符串")
			}
			offset++
		}
		value := raw[start:offset]
		switch {
		case strings.EqualFold(value, "AND"):
			tokens = append(tokens, logExpressionToken{kind: logExpressionAnd})
			hasOperator = true
		case strings.EqualFold(value, "OR"):
			tokens = append(tokens, logExpressionToken{kind: logExpressionOr})
			hasOperator = true
		default:
			tokens = append(tokens, logExpressionToken{kind: logExpressionTerm, value: value})
		}
	}
	return tokens, hasOperator, nil
}

func readQuotedLogTerm(raw string, offset int) (string, int, error) {
	var value strings.Builder
	for offset++; offset < len(raw); offset++ {
		current := raw[offset]
		if current == '"' {
			next := offset + 1
			if next < len(raw) && raw[next] != ' ' && raw[next] != '\t' {
				return "", 0, fmt.Errorf("日志表达式中的引号必须独立包裹字符串")
			}
			return value.String(), next, nil
		}
		if current != '\\' {
			value.WriteByte(current)
			continue
		}
		offset++
		if offset == len(raw) {
			return "", 0, fmt.Errorf("日志表达式包含未完成的转义字符")
		}
		escaped := raw[offset]
		if escaped != '"' && escaped != '\\' && escaped != '/' {
			return "", 0, fmt.Errorf(`日志表达式仅支持 \"、\\ 和 \/ 转义`)
		}
		value.WriteByte(escaped)
	}
	return "", 0, fmt.Errorf("日志表达式中的字符串缺少结束引号")
}

func normalizeLogLines(responses []*lokiQueryResponse, limit int) []logLine {
	lines := make([]logLine, 0)
	seen := make(map[string]struct{})
	for _, response := range responses {
		if response == nil {
			continue
		}
		for _, stream := range response.Data.Result {
			for _, value := range stream.Values {
				if len(value) < 2 {
					continue
				}
				labels := make(map[string]string, len(stream.Stream))
				for key, item := range stream.Stream {
					labels[key] = item
				}
				timestamp := formatLokiTimestamp(value[0])
				serializedLabels, _ := json.Marshal(labels)
				key := timestamp + "\x00" + string(serializedLabels) + "\x00" + value[1]
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				lines = append(lines, logLine{Timestamp: timestamp, Line: value[1], Labels: labels})
			}
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
