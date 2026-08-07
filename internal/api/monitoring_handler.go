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
	"sync"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const maxPromQLLength = 2048

var monitoringRanges = map[string]struct {
	window time.Duration
	step   time.Duration
}{
	"1h":  {window: time.Hour, step: 30 * time.Second},
	"6h":  {window: 6 * time.Hour, step: 2 * time.Minute},
	"24h": {window: 24 * time.Hour, step: 5 * time.Minute},
	"7d":  {window: 7 * 24 * time.Hour, step: 30 * time.Minute},
}

type monitoringQueryFunc func(context.Context, string, url.Values) (interface{}, error)

type monitoringDashboardQuery struct {
	key   string
	query string
}

type diskGrowthQuery struct {
	key   string
	query string
}

type diskGrowthItem struct {
	Node        string   `json:"node,omitempty"`
	MountPoint  string   `json:"mount_point,omitempty"`
	Namespace   string   `json:"namespace,omitempty"`
	PVC         string   `json:"pvc,omitempty"`
	Pod         string   `json:"pod,omitempty"`
	Container   string   `json:"container,omitempty"`
	GrowthBytes float64  `json:"growth_bytes"`
	Consumers   []string `json:"consumers,omitempty"`
}

var monitoringDashboardQueries = []monitoringDashboardQuery{
	{key: "cpu", query: `100 - (avg by (node, instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)`},
	{key: "memory", query: `100 * (1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)`},
	{key: "disk", query: `max by (node, instance) (100 * (1 - node_filesystem_avail_bytes{mountpoint="/",fstype!~"tmpfs|overlay"} / node_filesystem_size_bytes{mountpoint="/",fstype!~"tmpfs|overlay"}))`},
	{key: "network", query: `sum by (node, instance) (rate(node_network_receive_bytes_total{device!~"lo|veth.*"}[5m])) / 1024 / 1024`},
}

// MonitoringHandler manages the platform-owned VictoriaMetrics instance.
type MonitoringHandler struct {
	query monitoringQueryFunc
}

func monitoringDataStoreAvailable(status *k8s.VictoriaMetricsStatus) bool {
	return status != nil && status.ReadyReplicas > 0
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
	model.SuccessWithMessage(c, gin.H{"data_retained": true}, "VictoriaMetrics 已卸载，系统管理的存储卷已保留")
}

func (h *MonitoringHandler) MigrateLegacyStorage(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	var request k8s.VictoriaMetricsMigrationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "迁移存储配置无效")
		return
	}
	status, err := K8s.StartVictoriaMetricsHostPathMigration(request)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	model.SuccessWithMessage(c, status, "已停止 VictoriaMetrics，正在复制并校验历史数据")
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
	if !monitoringDataStoreAvailable(status) {
		model.Error(c, http.StatusConflict, model.CodeConflict, "VictoriaMetrics 存储实例尚未就绪")
		return
	}
	result, err := h.query(c.Request.Context(), "/api/v1/query", url.Values{"query": []string{query}})
	if err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "查询 VictoriaMetrics 失败: "+err.Error())
		return
	}
	model.Success(c, result)
}

// QueryRange exposes a bounded set of history windows for dashboard charts.
func (h *MonitoringHandler) QueryRange(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	query := strings.TrimSpace(c.Query("query"))
	if query == "" || len(query) > maxPromQLLength {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "PromQL 查询不能为空且不能超过 2048 个字符")
		return
	}
	rangeSpec, ok := monitoringRanges[c.DefaultQuery("range", "6h")]
	if !ok {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "时间范围仅支持 1h、6h、24h 或 7d")
		return
	}
	status := K8s.VictoriaMetricsStatus()
	if !monitoringDataStoreAvailable(status) {
		model.Error(c, http.StatusConflict, model.CodeConflict, "VictoriaMetrics 存储实例尚未就绪")
		return
	}
	end := time.Now().UTC()
	result, err := h.query(c.Request.Context(), "/api/v1/query_range", url.Values{
		"query": []string{query},
		"start": []string{strconv.FormatInt(end.Add(-rangeSpec.window).Unix(), 10)},
		"end":   []string{strconv.FormatInt(end.Unix(), 10)},
		"step":  []string{strconv.FormatInt(int64(rangeSpec.step.Seconds()), 10)},
	})
	if err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "查询 VictoriaMetrics 历史指标失败: "+err.Error())
		return
	}
	model.Success(c, result)
}

// Dashboard batches the four default node trend queries into one browser
// request while preserving concurrent reads against VictoriaMetrics.
func (h *MonitoringHandler) Dashboard(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	rangeName := c.DefaultQuery("range", "6h")
	rangeSpec, ok := monitoringRanges[rangeName]
	if !ok {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "时间范围仅支持 1h、6h、24h 或 7d")
		return
	}
	if status := K8s.VictoriaMetricsStatus(); !monitoringDataStoreAvailable(status) {
		model.Error(c, http.StatusConflict, model.CodeConflict, "VictoriaMetrics 存储实例尚未就绪")
		return
	}

	end := time.Now().UTC()
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	type dashboardResult struct {
		key  string
		data interface{}
		err  error
	}
	results := make(chan dashboardResult, len(monitoringDashboardQueries))
	var group sync.WaitGroup
	for _, dashboardQuery := range monitoringDashboardQueries {
		group.Add(1)
		go func(item monitoringDashboardQuery) {
			defer group.Done()
			data, err := h.query(ctx, "/api/v1/query_range", monitoringRangeValues(item.query, rangeSpec, end))
			results <- dashboardResult{key: item.key, data: data, err: err}
		}(dashboardQuery)
	}
	go func() {
		group.Wait()
		close(results)
	}()

	trends := make(map[string]interface{}, len(monitoringDashboardQueries))
	for result := range results {
		if result.err != nil {
			cancel()
			model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "查询 VictoriaMetrics 趋势指标失败: "+result.err.Error())
			return
		}
		trends[result.key] = result.data
	}
	model.Success(c, gin.H{"range": rangeName, "trends": trends})
}

// DiskGrowth ranks positive filesystem growth from the metrics already
// collected by the managed VictoriaMetrics instance. The browser selects only
// a bounded window and optional node; all PromQL is controlled here.
func (h *MonitoringHandler) DiskGrowth(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	rangeName := c.DefaultQuery("range", "6h")
	rangeSpec, ok := monitoringRanges[rangeName]
	if !ok {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "时间范围仅支持 1h、6h、24h 或 7d")
		return
	}
	node := strings.TrimSpace(c.Query("node"))
	if node != "" && len(validation.IsDNS1123Subdomain(node)) > 0 {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "节点名称无效")
		return
	}
	if status := K8s.VictoriaMetricsStatus(); !monitoringDataStoreAvailable(status) {
		model.Error(c, http.StatusConflict, model.CodeConflict, "VictoriaMetrics 存储实例尚未就绪")
		return
	}

	queries := diskGrowthQueries(monitoringPromQLWindow(rangeSpec.window), node)
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	type diskGrowthResult struct {
		key  string
		data interface{}
		err  error
	}
	results := make(chan diskGrowthResult, len(queries))
	var group sync.WaitGroup
	for _, item := range queries {
		group.Add(1)
		go func(item diskGrowthQuery) {
			defer group.Done()
			data, err := h.query(ctx, "/api/v1/query", url.Values{"query": []string{item.query}})
			results <- diskGrowthResult{key: item.key, data: data, err: err}
		}(item)
	}
	go func() {
		group.Wait()
		close(results)
	}()

	raw := make(map[string]interface{}, len(queries))
	warnings := make(map[string]string)
	for result := range results {
		if result.err != nil {
			warnings[result.key] = result.err.Error()
			continue
		}
		raw[result.key] = result.data
	}
	consumers, err := pvcConsumers(c.Request.Context())
	if err != nil {
		warnings["pvcs"] = "读取 PVC 当前使用者失败: " + err.Error()
		consumers = map[string][]string{}
	}
	data := gin.H{
		"range":      rangeName,
		"node":       node,
		"mounts":     normalizeMountGrowth(raw["mounts"]),
		"pvcs":       normalizePVCGrowth(raw["pvcs"], consumers),
		"containers": normalizeContainerGrowth(raw["containers"]),
	}
	if len(warnings) > 0 {
		data["warnings"] = warnings
	}
	model.Success(c, data)
}

func monitoringPromQLWindow(window time.Duration) string {
	switch window {
	case time.Hour:
		return "1h"
	case 6 * time.Hour:
		return "6h"
	case 24 * time.Hour:
		return "24h"
	case 7 * 24 * time.Hour:
		return "7d"
	default:
		return "6h"
	}
}

func diskGrowthQueries(window, node string) []diskGrowthQuery {
	nodeMatcher := ""
	if node != "" {
		nodeMatcher = ",node=" + strconv.Quote(node)
	}
	return []diskGrowthQuery{
		{key: "mounts", query: fmt.Sprintf(`topk(12, max by (node, mountpoint) (clamp_min(-delta(node_filesystem_avail_bytes{fstype!~"tmpfs|overlay",mountpoint!~"/etc/(hosts|hostname|resolv[.]conf)"%s}[%s]), 0)) and on (node, mountpoint) node_filesystem_avail_bytes{fstype!~"tmpfs|overlay",mountpoint!~"/etc/(hosts|hostname|resolv[.]conf)"%s})`, nodeMatcher, window, nodeMatcher)},
		{key: "pvcs", query: fmt.Sprintf(`topk(12, max by (node, namespace, persistentvolumeclaim) (clamp_min(delta(kubelet_volume_stats_used_bytes{%s}[%s]), 0)))`, strings.TrimPrefix(nodeMatcher, ","), window)},
		{key: "containers", query: fmt.Sprintf(`topk(12, max by (node, namespace, pod, container) (clamp_min(delta(container_fs_usage_bytes{container!="",pod!=""%s}[%s]), 0)))`, nodeMatcher, window)},
	}
}

func pvcConsumers(ctx context.Context) (map[string][]string, error) {
	pods, err := K8s.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	consumers := make(map[string][]string)
	for _, pod := range pods.Items {
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim == nil || volume.PersistentVolumeClaim.ClaimName == "" {
				continue
			}
			key := pod.Namespace + "/" + volume.PersistentVolumeClaim.ClaimName
			consumers[key] = append(consumers[key], pod.Name)
		}
	}
	for key, names := range consumers {
		sort.Strings(names)
		consumers[key] = names
	}
	return consumers, nil
}

func normalizeMountGrowth(data interface{}) []diskGrowthItem {
	items := make([]diskGrowthItem, 0)
	for _, sample := range monitoringVectorSamples(data) {
		if sample.value <= 0 {
			continue
		}
		items = append(items, diskGrowthItem{Node: sample.label("node"), MountPoint: sample.label("mountpoint"), GrowthBytes: sample.value})
	}
	return sortDiskGrowthItems(items)
}

func normalizePVCGrowth(data interface{}, consumers map[string][]string) []diskGrowthItem {
	items := make([]diskGrowthItem, 0)
	for _, sample := range monitoringVectorSamples(data) {
		if sample.value <= 0 {
			continue
		}
		namespace := sample.label("namespace")
		claim := sample.label("persistentvolumeclaim")
		items = append(items, diskGrowthItem{Node: sample.label("node"), Namespace: namespace, PVC: claim, GrowthBytes: sample.value, Consumers: consumers[namespace+"/"+claim]})
	}
	return sortDiskGrowthItems(items)
}

func normalizeContainerGrowth(data interface{}) []diskGrowthItem {
	items := make([]diskGrowthItem, 0)
	for _, sample := range monitoringVectorSamples(data) {
		if sample.value <= 0 {
			continue
		}
		items = append(items, diskGrowthItem{Node: sample.label("node"), Namespace: sample.label("namespace"), Pod: sample.label("pod"), Container: sample.label("container"), GrowthBytes: sample.value})
	}
	return sortDiskGrowthItems(items)
}

func sortDiskGrowthItems(items []diskGrowthItem) []diskGrowthItem {
	sort.SliceStable(items, func(left, right int) bool {
		if items[left].GrowthBytes != items[right].GrowthBytes {
			return items[left].GrowthBytes > items[right].GrowthBytes
		}
		return diskGrowthItemKey(items[left]) < diskGrowthItemKey(items[right])
	})
	return items
}

func diskGrowthItemKey(item diskGrowthItem) string {
	return strings.Join([]string{item.Node, item.MountPoint, item.Namespace, item.PVC, item.Pod, item.Container}, "\x00")
}

type monitoringVectorSample struct {
	metric map[string]interface{}
	value  float64
}

func (sample monitoringVectorSample) label(name string) string {
	value, _ := sample.metric[name].(string)
	return value
}

func monitoringVectorSamples(data interface{}) []monitoringVectorSample {
	payload, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	results, ok := payload["result"].([]interface{})
	if !ok {
		return nil
	}
	samples := make([]monitoringVectorSample, 0, len(results))
	for _, raw := range results {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		metric, ok := item["metric"].(map[string]interface{})
		if !ok {
			continue
		}
		value, ok := item["value"].([]interface{})
		if !ok || len(value) < 2 {
			continue
		}
		growth, err := strconv.ParseFloat(fmt.Sprint(value[1]), 64)
		if err != nil {
			continue
		}
		samples = append(samples, monitoringVectorSample{metric: metric, value: growth})
	}
	return samples
}

func monitoringRangeValues(query string, rangeSpec struct {
	window time.Duration
	step   time.Duration
}, end time.Time) url.Values {
	return url.Values{
		"query": []string{query},
		"start": []string{strconv.FormatInt(end.Add(-rangeSpec.window).Unix(), 10)},
		"end":   []string{strconv.FormatInt(end.Unix(), 10)},
		"step":  []string{strconv.FormatInt(int64(rangeSpec.step.Seconds()), 10)},
	}
}

func (h *MonitoringHandler) Targets(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	status := K8s.VictoriaMetricsStatus()
	if !monitoringDataStoreAvailable(status) {
		model.Error(c, http.StatusConflict, model.CodeConflict, "VictoriaMetrics 存储实例尚未就绪")
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
