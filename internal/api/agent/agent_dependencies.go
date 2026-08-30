package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
)

const (
	coreDNSNamespace = "kube-system"
	coreDNSConfigMap = "coredns"
)

var coreDNSForwardPattern = regexp.MustCompile(`(?m)^(\s*forward\s+\.\s+)([^\n{]+)(\{[^\n]*\})?\s*$`)

func forwardTargets(corefile string) []string {
	matches := coreDNSForwardPattern.FindStringSubmatch(corefile)
	if len(matches) < 3 {
		return []string{}
	}
	return strings.Fields(strings.TrimSpace(matches[2]))
}

func policyPayload(policy *model.ClusterDNSPolicy) any {
	if policy == nil {
		return nil
	}
	var resolvers []string
	_ = json.Unmarshal([]byte(policy.Resolvers), &resolvers)
	return map[string]any{"revision": policy.Revision, "resolvers": resolvers, "created_at": policy.CreatedAt}
}

func coreDNSPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

var monitoringRanges = map[string]struct {
	window time.Duration
	step   time.Duration
}{
	"1h":  {window: time.Hour, step: 30 * time.Second},
	"6h":  {window: 6 * time.Hour, step: 2 * time.Minute},
	"24h": {window: 24 * time.Hour, step: 5 * time.Minute},
	"7d":  {window: 7 * 24 * time.Hour, step: 30 * time.Minute},
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
	GrowthBytes float64  `json:"growth_bytes"`
	Consumers   []string `json:"consumers,omitempty"`
}

type monitoringVectorSample struct {
	metric map[string]interface{}
	value  float64
}

func (sample monitoringVectorSample) label(name string) string {
	value, _ := sample.metric[name].(string)
	return value
}

func monitoringDataStoreAvailable(status *k8sclient.VictoriaMetricsStatus) bool {
	return status != nil && status.ReadyReplicas > 0
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
	}
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

func normalizeMountGrowth(data interface{}) []diskGrowthItem {
	items := make([]diskGrowthItem, 0)
	for _, sample := range monitoringVectorSamples(data) {
		if sample.value > 0 {
			items = append(items, diskGrowthItem{Node: sample.label("node"), MountPoint: sample.label("mountpoint"), GrowthBytes: sample.value})
		}
	}
	sort.SliceStable(items, func(left, right int) bool { return items[left].GrowthBytes > items[right].GrowthBytes })
	return items
}

func queryVictoriaMetrics(ctx context.Context, path string, values url.Values) (interface{}, error) {
	requestURL := k8sclient.VictoriaMetricsServiceURL() + path
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
		Status string      `json:"status"`
		Data   interface{} `json:"data"`
		Error  string      `json:"error"`
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
