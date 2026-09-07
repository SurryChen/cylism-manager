package system

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	monitoringservice "github.com/cylism/cylism-manager/internal/service/observability/monitoring"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func setupMonitoringRouter(handler *MonitoringHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/monitoring")
	group.GET("/status", handler.Status)
	group.POST("/install", handler.Install)
	group.POST("/storage-migration", handler.MigrateLegacyStorage)
	group.DELETE("", handler.Uninstall)
	group.GET("/query", handler.Query)
	group.GET("/query-range", handler.QueryRange)
	group.GET("/dashboard", handler.Dashboard)
	group.GET("/disk-growth", handler.DiskGrowth)
	group.GET("/targets", handler.Targets)
	return router
}

func newTestMonitoringHandler() *MonitoringHandler {
	client := k8sClient
	return NewMonitoringHandler(MonitoringDependencies{
		Component: client,
		Status: monitoringservice.StatusFunc(func(ctx context.Context) bool {
			return client != nil && client.VictoriaMetricsStatusContext(ctx).ReadyReplicas > 0
		}),
		Consumers: monitoringservice.PVCConsumerReader{Pods: client},
	})
}

func TestMonitoringRangeQueryUsesBoundedRange(t *testing.T) {
	original := k8sClient
	k8sClient = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "cylism-victoria-metrics", Namespace: "monitoring"}, Status: appsv1.DeploymentStatus{AvailableReplicas: 1}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "cylism-node-exporter", Namespace: "monitoring"}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 1, NumberAvailable: 1}},
	)}
	defer func() { k8sClient = original }()

	handler := newTestMonitoringHandler()
	handler.query = func(_ context.Context, path string, values url.Values) (interface{}, error) {
		if path != "/api/v1/query_range" || values.Get("query") != "up" || values.Get("step") != "30" {
			t.Fatalf("unexpected range query: %s %#v", path, values)
		}
		start, _ := strconv.ParseInt(values.Get("start"), 10, 64)
		end, _ := strconv.ParseInt(values.Get("end"), 10, 64)
		if end-start != 3600 {
			t.Fatalf("expected one hour range, got %d seconds", end-start)
		}
		return map[string]interface{}{"resultType": "matrix", "result": []interface{}{}}, nil
	}

	response := serve(setupMonitoringRouter(handler), newJSONRequest(http.MethodGet, "/api/monitoring/query-range?query=up&range=1h", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "matrix") {
		t.Fatalf("unexpected range response: %s", response.Body.String())
	}
}

func TestMonitoringQueryAndTargetsDelegateToInjectedTransport(t *testing.T) {
	original := k8sClient
	k8sClient = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "cylism-victoria-metrics", Namespace: "monitoring"}, Status: appsv1.DeploymentStatus{AvailableReplicas: 1}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "cylism-node-exporter", Namespace: "monitoring"}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 1, NumberAvailable: 1}},
	)}
	defer func() { k8sClient = original }()

	paths := make([]string, 0, 2)
	handler := newTestMonitoringHandler().WithQuery(func(_ context.Context, path string, values url.Values) (interface{}, error) {
		paths = append(paths, path)
		if path == "/api/v1/query" && values.Get("query") != "up" {
			t.Fatalf("query expression was not delegated: %#v", values)
		}
		return map[string]interface{}{"status": "success"}, nil
	})
	router := setupMonitoringRouter(handler)
	if response := serve(router, newJSONRequest(http.MethodGet, "/api/monitoring/query?query=up", nil)); response.Code != http.StatusOK {
		t.Fatalf("query status = %d: %s", response.Code, response.Body.String())
	}
	if response := serve(router, newJSONRequest(http.MethodGet, "/api/monitoring/targets", nil)); response.Code != http.StatusOK {
		t.Fatalf("targets status = %d: %s", response.Code, response.Body.String())
	}
	if strings.Join(paths, ",") != "/api/v1/query,/api/v1/targets" {
		t.Fatalf("unexpected delegated paths: %#v", paths)
	}
}

func TestMonitoringRangeQueryRejectsUnknownRange(t *testing.T) {
	original := k8sClient
	k8sClient = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()}
	defer func() { k8sClient = original }()

	response := serve(setupMonitoringRouter(newTestMonitoringHandler()), newJSONRequest(http.MethodGet, "/api/monitoring/query-range?query=up&range=30d", nil))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "时间范围") {
		t.Fatalf("unexpected range validation response: %s", response.Body.String())
	}
}

func TestMonitoringDashboardReturnsAllTrendSeries(t *testing.T) {
	original := k8sClient
	k8sClient = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "cylism-victoria-metrics", Namespace: "monitoring"}, Status: appsv1.DeploymentStatus{AvailableReplicas: 1}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "cylism-node-exporter", Namespace: "monitoring"}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 1, NumberAvailable: 1}},
	)}
	defer func() { k8sClient = original }()

	handler := newTestMonitoringHandler()
	handler.query = func(_ context.Context, path string, values url.Values) (interface{}, error) {
		if path != "/api/v1/query_range" || values.Get("query") == "" || values.Get("step") != "120" {
			t.Fatalf("unexpected dashboard query: %s %#v", path, values)
		}
		return map[string]interface{}{"resultType": "matrix", "result": []interface{}{}}, nil
	}

	response := serve(setupMonitoringRouter(handler), newJSONRequest(http.MethodGet, "/api/monitoring/dashboard?range=6h", nil))
	for _, key := range []string{"cpu", "memory", "disk", "network"} {
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"`+key+`"`) {
			t.Fatalf("unexpected dashboard response: %s", response.Body.String())
		}
	}
}

func TestMonitoringDiskGrowthReturnsRankingsAndPVCConsumers(t *testing.T) {
	original := k8sClient
	k8sClient = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "cylism-victoria-metrics", Namespace: "monitoring"}, Status: appsv1.DeploymentStatus{AvailableReplicas: 1}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "cylism-node-exporter", Namespace: "monitoring"}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 1, NumberAvailable: 1}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "app-1", Namespace: "project-demo"}, Spec: corev1.PodSpec{Volumes: []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data"}}}}}},
	)}
	defer func() { k8sClient = original }()

	var mu sync.Mutex
	queries := make([]string, 0, 3)
	handler := newTestMonitoringHandler()
	handler.query = func(_ context.Context, path string, values url.Values) (interface{}, error) {
		if path != "/api/v1/query" {
			t.Fatalf("unexpected metric endpoint: %s", path)
		}
		query := values.Get("query")
		mu.Lock()
		queries = append(queries, query)
		mu.Unlock()
		switch {
		case strings.Contains(query, "node_filesystem_avail_bytes"):
			return map[string]interface{}{"resultType": "vector", "result": []interface{}{map[string]interface{}{"metric": map[string]interface{}{"node": "node-a", "mountpoint": "/var/lib"}, "value": []interface{}{float64(1), "1048576"}}}}, nil
		case strings.Contains(query, "kubelet_volume_stats_used_bytes"):
			return map[string]interface{}{"resultType": "vector", "result": []interface{}{map[string]interface{}{"metric": map[string]interface{}{"node": "node-a", "namespace": "project-demo", "persistentvolumeclaim": "data"}, "value": []interface{}{float64(1), "2097152"}}}}, nil
		default:
			t.Fatalf("unexpected disk growth query: %s", query)
			return nil, nil
		}
	}

	response := serve(setupMonitoringRouter(handler), newJSONRequest(http.MethodGet, "/api/monitoring/disk-growth?range=6h&node=node-a", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected disk growth response: %s", response.Body.String())
	}
	var payload struct {
		Data struct {
			Range  string `json:"range"`
			Node   string `json:"node"`
			Mounts []struct {
				GrowthBytes float64 `json:"growth_bytes"`
			} `json:"mounts"`
			PVCs []struct {
				Consumers []string `json:"consumers"`
			} `json:"pvcs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Range != "6h" || payload.Data.Node != "node-a" || len(payload.Data.Mounts) != 1 || payload.Data.Mounts[0].GrowthBytes != 1048576 || len(payload.Data.PVCs) != 1 || !slicesEqual(payload.Data.PVCs[0].Consumers, []string{"app-1"}) || strings.Contains(response.Body.String(), `"containers"`) {
		t.Fatalf("unexpected disk growth payload: %s", response.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(queries) != 2 {
		t.Fatalf("expected two fixed queries, got %#v", queries)
	}
	for _, query := range queries {
		if strings.Contains(query, "container_fs_usage_bytes") {
			t.Fatalf("container writable-layer query must not run: %s", query)
		}
		if !strings.Contains(query, `node="node-a"`) {
			t.Fatalf("node filter missing from query: %s", query)
		}
	}
}

func TestDiskGrowthMountQueryAvoidsInvalidPromQLStringEscapes(t *testing.T) {
	queries := monitoringservice.DiskGrowthQueries("6h", "")
	if len(queries) == 0 || strings.Contains(queries[0].Query, `\.`) || !strings.Contains(queries[0].Query, "resolv[.]conf") || !strings.Contains(queries[0].Query, "and on (node, mountpoint) node_filesystem_avail_bytes") {
		t.Fatalf("unexpected mount query: %#v", queries)
	}
}

func TestDiskGrowthDoesNotQueryUnsupportedContainerWritableLayerMetrics(t *testing.T) {
	queries := monitoringservice.DiskGrowthQueries("6h", "")
	if len(queries) != 2 {
		t.Fatalf("unexpected disk growth queries: %#v", queries)
	}
	for _, query := range queries {
		if strings.Contains(query.Query, "container_fs_usage_bytes") {
			t.Fatalf("container writable-layer query must not be configured: %#v", queries)
		}
	}
}

func TestMonitoringDiskGrowthReturnsPartialResultsWhenOneQueryFails(t *testing.T) {
	original := k8sClient
	k8sClient = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "cylism-victoria-metrics", Namespace: "monitoring"}, Status: appsv1.DeploymentStatus{AvailableReplicas: 1}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "cylism-node-exporter", Namespace: "monitoring"}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 1, NumberAvailable: 1}},
	)}
	defer func() { k8sClient = original }()

	handler := newTestMonitoringHandler()
	handler.query = func(_ context.Context, _ string, values url.Values) (interface{}, error) {
		if strings.Contains(values.Get("query"), "kubelet_volume_stats_used_bytes") {
			return nil, errors.New("VictoriaMetrics 返回 422 Unprocessable Entity")
		}
		return map[string]interface{}{"resultType": "vector", "result": []interface{}{}}, nil
	}

	response := serve(setupMonitoringRouter(handler), newJSONRequest(http.MethodGet, "/api/monitoring/disk-growth?range=6h", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"warnings":{"pvcs":"VictoriaMetrics 返回 422 Unprocessable Entity"}`) {
		t.Fatalf("expected partial disk diagnostics response, got %s", response.Body.String())
	}
}

func TestMonitoringDiskGrowthRejectsUnsupportedRangeAndUnreadyInstance(t *testing.T) {
	original := k8sClient
	k8sClient = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()}
	defer func() { k8sClient = original }()

	handler := newTestMonitoringHandler()
	handler.query = func(_ context.Context, _ string, _ url.Values) (interface{}, error) {
		t.Fatal("metric query must not run when validation or readiness fails")
		return nil, nil
	}
	response := serve(setupMonitoringRouter(handler), newJSONRequest(http.MethodGet, "/api/monitoring/disk-growth?range=30d", nil))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "时间范围") {
		t.Fatalf("unexpected range validation response: %s", response.Body.String())
	}
	response = serve(setupMonitoringRouter(handler), newJSONRequest(http.MethodGet, "/api/monitoring/disk-growth?range=1h", nil))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "尚未就绪") {
		t.Fatalf("unexpected readiness response: %s", response.Body.String())
	}
}

func TestNormalizeMountGrowthSortsLargestFirst(t *testing.T) {
	items := monitoringservice.NormalizeMountGrowth(map[string]interface{}{"result": []interface{}{
		map[string]interface{}{"metric": map[string]interface{}{"node": "node-a", "mountpoint": "/small"}, "value": []interface{}{float64(1), "7340032"}},
		map[string]interface{}{"metric": map[string]interface{}{"node": "node-a", "mountpoint": "/large"}, "value": []interface{}{float64(1), "775946240"}},
	}})
	if len(items) != 2 || items[0].MountPoint != "/large" || items[0].GrowthBytes != 775946240 {
		t.Fatalf("expected largest growth first, got %#v", items)
	}
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestMonitoringInstallCreatesPVCInstance(t *testing.T) {
	original := k8sClient
	k8sClient = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}})}
	defer func() { k8sClient = original }()

	response := serve(setupMonitoringRouter(newTestMonitoringHandler()), newJSONRequest(http.MethodPost, "/api/monitoring/install", gin.H{"node_name": "node-a", "storage": "10Gi", "retention_days": 14}))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"node_name":"node-a"`) {
		t.Fatalf("unexpected install response: %s", response.Body.String())
	}
}

func TestMonitoringQueryRequiresReadyInstance(t *testing.T) {
	original := k8sClient
	k8sClient = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()}
	defer func() { k8sClient = original }()

	response := serve(setupMonitoringRouter(newTestMonitoringHandler()), newJSONRequest(http.MethodGet, "/api/monitoring/query?query=up", nil))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "尚未就绪") {
		t.Fatalf("unexpected query response: %s", response.Body.String())
	}
}

func TestMonitoringQueryReturnsMetricData(t *testing.T) {
	original := k8sClient
	k8sClient = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "cylism-victoria-metrics", Namespace: "monitoring"}, Status: appsv1.DeploymentStatus{AvailableReplicas: 1}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "cylism-node-exporter", Namespace: "monitoring"}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 1, NumberAvailable: 1}},
	)}
	defer func() { k8sClient = original }()
	handler := newTestMonitoringHandler()
	handler.query = func(_ context.Context, path string, values url.Values) (interface{}, error) {
		if path != "/api/v1/query" || values.Get("query") != "up" {
			t.Fatalf("unexpected query: %s %#v", path, values)
		}
		return map[string]interface{}{"resultType": "vector", "result": []interface{}{}}, nil
	}

	response := serve(setupMonitoringRouter(handler), newJSONRequest(http.MethodGet, "/api/monitoring/query?query=up", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "resultType") {
		t.Fatalf("unexpected query response: %s", response.Body.String())
	}
}

func TestMonitoringQueryAllowsPartialNodeExporterCoverage(t *testing.T) {
	original := k8sClient
	k8sClient = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "cylism-victoria-metrics", Namespace: "monitoring"}, Status: appsv1.DeploymentStatus{AvailableReplicas: 1}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "cylism-node-exporter", Namespace: "monitoring"}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 2, NumberAvailable: 1}},
	)}
	defer func() { k8sClient = original }()

	handler := newTestMonitoringHandler()
	handler.query = func(_ context.Context, _ string, _ url.Values) (interface{}, error) {
		return map[string]interface{}{"resultType": "vector", "result": []interface{}{}}, nil
	}

	response := serve(setupMonitoringRouter(handler), newJSONRequest(http.MethodGet, "/api/monitoring/query?query=up", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected query to remain available with partial exporter coverage, got %s", response.Body.String())
	}
}
