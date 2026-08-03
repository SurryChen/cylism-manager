package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
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
	group.DELETE("", handler.Uninstall)
	group.GET("/query", handler.Query)
	group.GET("/query-range", handler.QueryRange)
	group.GET("/dashboard", handler.Dashboard)
	group.GET("/targets", handler.Targets)
	return router
}

func TestMonitoringRangeQueryUsesBoundedRange(t *testing.T) {
	original := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "cylism-victoria-metrics", Namespace: "monitoring"}, Status: appsv1.DeploymentStatus{AvailableReplicas: 1}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "cylism-node-exporter", Namespace: "monitoring"}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 1, NumberAvailable: 1}},
	)}
	defer func() { K8s = original }()

	handler := NewMonitoringHandler()
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

func TestMonitoringRangeQueryRejectsUnknownRange(t *testing.T) {
	original := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()}
	defer func() { K8s = original }()

	response := serve(setupMonitoringRouter(NewMonitoringHandler()), newJSONRequest(http.MethodGet, "/api/monitoring/query-range?query=up&range=30d", nil))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "时间范围") {
		t.Fatalf("unexpected range validation response: %s", response.Body.String())
	}
}

func TestMonitoringDashboardReturnsAllTrendSeries(t *testing.T) {
	original := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "cylism-victoria-metrics", Namespace: "monitoring"}, Status: appsv1.DeploymentStatus{AvailableReplicas: 1}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "cylism-node-exporter", Namespace: "monitoring"}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 1, NumberAvailable: 1}},
	)}
	defer func() { K8s = original }()

	handler := NewMonitoringHandler()
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

func TestMonitoringInstallCreatesHostPathInstance(t *testing.T) {
	original := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}})}
	defer func() { K8s = original }()

	response := serve(setupMonitoringRouter(NewMonitoringHandler()), newJSONRequest(http.MethodPost, "/api/monitoring/install", gin.H{"node_name": "node-a", "data_path": "/data/victoria-metrics", "retention_days": 14}))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"node_name":"node-a"`) {
		t.Fatalf("unexpected install response: %s", response.Body.String())
	}
}

func TestMonitoringQueryRequiresReadyInstance(t *testing.T) {
	original := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()}
	defer func() { K8s = original }()

	response := serve(setupMonitoringRouter(NewMonitoringHandler()), newJSONRequest(http.MethodGet, "/api/monitoring/query?query=up", nil))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "尚未就绪") {
		t.Fatalf("unexpected query response: %s", response.Body.String())
	}
}

func TestMonitoringQueryReturnsMetricData(t *testing.T) {
	original := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "cylism-victoria-metrics", Namespace: "monitoring"}, Status: appsv1.DeploymentStatus{AvailableReplicas: 1}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "cylism-node-exporter", Namespace: "monitoring"}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 1, NumberAvailable: 1}},
	)}
	defer func() { K8s = original }()
	handler := NewMonitoringHandler()
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
