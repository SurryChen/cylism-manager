package monitoring

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type QueryFunc func(context.Context, string, url.Values) (interface{}, error)
type DashboardQuery struct{ Key, Query string }

var DefaultDashboardQueries = []DashboardQuery{{"cpu", `100 - (avg by (node, instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)`}, {"memory", `100 * (1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)`}, {"disk", `max by (node, instance) (100 * (1 - node_filesystem_avail_bytes{mountpoint="/",fstype!~"tmpfs|overlay"} / node_filesystem_size_bytes{mountpoint="/",fstype!~"tmpfs|overlay"}))`}, {"network", `sum by (node, instance) (rate(node_network_receive_bytes_total{device!~"lo|veth.*"}[5m])) / 1024 / 1024`}}

func Dashboard(ctx context.Context, name string, end time.Time, query QueryFunc) (map[string]interface{}, error) {
	spec, ok := ResolveRange(name)
	if !ok {
		return nil, fmt.Errorf("时间范围仅支持 1h、6h、24h 或 7d")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		key  string
		data interface{}
		err  error
	}
	ch := make(chan result, len(DefaultDashboardQueries))
	for _, item := range DefaultDashboardQueries {
		go func(item DashboardQuery) {
			data, err := query(ctx, "/api/v1/query_range", RangeValues(item.Query, spec, end))
			ch <- result{item.Key, data, err}
		}(item)
	}
	out := make(map[string]interface{}, len(DefaultDashboardQueries))
	for range DefaultDashboardQueries {
		r := <-ch
		if r.err != nil {
			cancel()
			return nil, r.err
		}
		out[r.key] = r.data
	}
	return out, nil
}

type GrowthItem struct {
	Node        string   `json:"node,omitempty"`
	MountPoint  string   `json:"mount_point,omitempty"`
	Namespace   string   `json:"namespace,omitempty"`
	PVC         string   `json:"pvc,omitempty"`
	GrowthBytes float64  `json:"growth_bytes"`
	Consumers   []string `json:"consumers,omitempty"`
}
type vectorSample struct {
	metric map[string]interface{}
	value  float64
}

func NormalizeMountGrowth(data interface{}) []GrowthItem {
	items := make([]GrowthItem, 0)
	for _, sample := range vectorSamples(data) {
		if sample.value > 0 {
			items = append(items, GrowthItem{Node: label(sample, "node"), MountPoint: label(sample, "mountpoint"), GrowthBytes: sample.value})
		}
	}
	return sortGrowth(items)
}
func NormalizePVCGrowth(data interface{}, consumers map[string][]string) []GrowthItem {
	items := make([]GrowthItem, 0)
	for _, sample := range vectorSamples(data) {
		if sample.value <= 0 {
			continue
		}
		ns, pvc := label(sample, "namespace"), label(sample, "persistentvolumeclaim")
		items = append(items, GrowthItem{Node: label(sample, "node"), Namespace: ns, PVC: pvc, GrowthBytes: sample.value, Consumers: consumers[ns+"/"+pvc]})
	}
	return sortGrowth(items)
}
func vectorSamples(data interface{}) []vectorSample {
	payload, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	raw, ok := payload["result"].([]interface{})
	if !ok {
		return nil
	}
	out := make([]vectorSample, 0, len(raw))
	for _, item := range raw {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		metric, ok := obj["metric"].(map[string]interface{})
		if !ok {
			continue
		}
		values, ok := obj["value"].([]interface{})
		if !ok || len(values) < 2 {
			continue
		}
		value, err := strconv.ParseFloat(fmt.Sprint(values[1]), 64)
		if err == nil {
			out = append(out, vectorSample{metric: metric, value: value})
		}
	}
	return out
}
func label(sample vectorSample, name string) string {
	value, _ := sample.metric[name].(string)
	return value
}
func sortGrowth(items []GrowthItem) []GrowthItem {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].GrowthBytes != items[j].GrowthBytes {
			return items[i].GrowthBytes > items[j].GrowthBytes
		}
		return growthKey(items[i]) < growthKey(items[j])
	})
	return items
}
func growthKey(item GrowthItem) string {
	return strings.Join([]string{item.Node, item.MountPoint, item.Namespace, item.PVC}, "\x00")
}

type RangeSpec struct{ Window, Step time.Duration }

var Ranges = map[string]RangeSpec{
	"1h": {Window: time.Hour, Step: 30 * time.Second}, "6h": {Window: 6 * time.Hour, Step: 2 * time.Minute},
	"24h": {Window: 24 * time.Hour, Step: 5 * time.Minute}, "7d": {Window: 7 * 24 * time.Hour, Step: 30 * time.Minute},
}

func ResolveRange(name string) (RangeSpec, bool) {
	spec, ok := Ranges[name]
	return spec, ok
}
func RangeValues(query string, spec RangeSpec, end time.Time) url.Values {
	return url.Values{
		"query": []string{query},
		"start": []string{strconv.FormatInt(end.Add(-spec.Window).Unix(), 10)},
		"end":   []string{strconv.FormatInt(end.Unix(), 10)},
		"step":  []string{strconv.FormatInt(int64(spec.Step.Seconds()), 10)},
	}
}
func PromQLWindow(window time.Duration) string {
	for name, spec := range Ranges {
		if spec.Window == window {
			return name
		}
	}
	return "6h"
}
func DiskGrowthQueries(window, node string) []struct{ Key, Query string } {
	n := ""
	if node != "" {
		n = ",node=" + strconv.Quote(node)
	}
	mount := fmt.Sprintf(`topk(12, max by (node, mountpoint) (clamp_min(-delta(node_filesystem_avail_bytes{fstype!~"tmpfs|overlay",mountpoint!~"/etc/(hosts|hostname|resolv[.]conf)"%s}[%s]), 0)) and on (node, mountpoint) node_filesystem_avail_bytes{fstype!~"tmpfs|overlay",mountpoint!~"/etc/(hosts|hostname|resolv[.]conf)"%s})`, n, window, n)
	pvc := fmt.Sprintf(`topk(12, max by (node, namespace, persistentvolumeclaim) (clamp_min(delta(kubelet_volume_stats_used_bytes{%s}[%s]), 0)))`, strings.TrimPrefix(n, ","), window)
	return []struct{ Key, Query string }{{Key: "mounts", Query: mount}, {Key: "pvcs", Query: pvc}}
}
