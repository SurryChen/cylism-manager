package api

import (
	"context"
	"net/http"
	"net/url"
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
	group.GET("/targets", handler.Targets)
	return router
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
