package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func setupLoggingRouter(handler *LoggingHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/monitoring/logs")
	group.GET("/status", handler.Status)
	group.POST("/install", handler.Install)
	group.PUT("/config", handler.Update)
	group.DELETE("", handler.Uninstall)
	group.GET("/filters", handler.Filters)
	group.POST("/query", handler.Query)
	return router
}

func TestLoggingQueryBuildsBoundedStructuredSelector(t *testing.T) {
	original := K8s
	K8s = loggingReadyK8s()
	defer func() { K8s = original }()

	handler := NewLoggingHandler()
	handler.now = func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }
	handler.query = func(_ context.Context, path string, values url.Values) (*lokiQueryResponse, error) {
		if path != "/loki/api/v1/query_range" {
			t.Fatalf("unexpected Loki path: %q", path)
		}
		if got := values.Get("query"); got != `{namespace="project-demo",pod="api-123",container="api"} |= "error"` {
			t.Fatalf("unexpected LogQL selector: %q", got)
		}
		if start, _ := strconv.ParseInt(values.Get("start"), 10, 64); start != 1_699_996_400_000_000_000 || values.Get("end") != "1700000000000000000" || values.Get("limit") != "100" || values.Get("direction") != "BACKWARD" {
			t.Fatalf("unexpected bounds: %#v", values)
		}
		return &lokiQueryResponse{Status: "success", Data: lokiQueryData{ResultType: "streams", Result: []lokiStream{{Stream: map[string]string{"namespace": "project-demo", "pod": "api-123", "container": "api"}, Values: [][]string{{"1700000000000000000", "error happened"}}}}}}, nil
	}

	response := serve(setupLoggingRouter(handler), newJSONRequest(http.MethodPost, "/api/monitoring/logs/query", gin.H{"range": "1h", "namespace": "project-demo", "pod": "api-123", "container": "api", "keyword": "error", "limit": 100}))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "error happened") || !strings.Contains(response.Body.String(), "has_more") {
		t.Fatalf("unexpected log query response: %d %s", response.Code, response.Body.String())
	}
}

func TestLoggingQuerySupportsExactTimeAndBooleanKeywordBranches(t *testing.T) {
	original := K8s
	K8s = loggingReadyK8s()
	defer func() { K8s = original }()

	handler := NewLoggingHandler()
	queries := make([]string, 0, 2)
	handler.query = func(_ context.Context, _ string, values url.Values) (*lokiQueryResponse, error) {
		queries = append(queries, values.Get("query"))
		if values.Get("start") != "1700000000000000000" || values.Get("end") != "1700000005000000000" {
			t.Fatalf("expected exact UTC bounds, got %#v", values)
		}
		return &lokiQueryResponse{Status: "success", Data: lokiQueryData{ResultType: "streams", Result: []lokiStream{{Stream: map[string]string{"namespace": "project-demo"}, Values: [][]string{{"1700000003000000000", "matched log"}}}}}}, nil
	}

	response := serve(setupLoggingRouter(handler), newJSONRequest(http.MethodPost, "/api/monitoring/logs/query", gin.H{
		"namespace":  "project-demo",
		"start_time": "2023-11-14T22:13:20Z",
		"end_time":   "2023-11-14T22:13:25Z",
		"keyword":    `"fetch failed" AND timeout OR "connection refused"`,
		"limit":      100,
	}))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "matched log") {
		t.Fatalf("unexpected log query response: %d %s", response.Code, response.Body.String())
	}
	want := []string{
		`{namespace="project-demo"} |= "fetch failed" |= "timeout"`,
		`{namespace="project-demo"} |= "connection refused"`,
	}
	if strings.Join(queries, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected query branches: %#v", queries)
	}
}

func TestBuildLogQLQueriesSupportsQuotedEscapesAndRejectsInvalidExpression(t *testing.T) {
	queries, err := buildLogQLQueries(logQueryRequest{Namespace: "default", Keyword: `"api\/v1" AND "said \"ready\"" OR health`})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`{namespace="default"} |= "api/v1" |= "said \"ready\""`,
		`{namespace="default"} |= "health"`,
	}
	if strings.Join(queries, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected parsed expression: %#v", queries)
	}
	if _, err := buildLogQLQueries(logQueryRequest{Keyword: `health AND`}); err == nil || !strings.Contains(err.Error(), "表达式") {
		t.Fatalf("expected expression validation error, got %v", err)
	}
}

func TestLoggingQueryRejectsRawLogQLAndOversizedRange(t *testing.T) {
	original := K8s
	K8s = loggingReadyK8s()
	defer func() { K8s = original }()

	response := serve(setupLoggingRouter(NewLoggingHandler()), newJSONRequest(http.MethodPost, "/api/monitoring/logs/query", gin.H{"range": "7d", "logql": "{job=~\".*\"}"}))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "24 小时") {
		t.Fatalf("expected bounded range error, got %d %s", response.Code, response.Body.String())
	}
}

func TestBuildLogQLAddsNamespaceMatcherForUnfilteredQuery(t *testing.T) {
	query, err := buildLogQL(logQueryRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if query != `{namespace=~".+"}` {
		t.Fatalf("expected non-empty namespace matcher for all-log query, got %q", query)
	}
}

func TestLoggingInstallAndFiltersExposeNoLokiEndpoint(t *testing.T) {
	original := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api-123", Namespace: "project-demo", Labels: map[string]string{"app.kubernetes.io/name": "api"}}, Spec: corev1.PodSpec{NodeName: "node-a", Containers: []corev1.Container{{Name: "api"}}}},
	)}
	defer func() { K8s = original }()

	handler := NewLoggingHandler()
	response := serve(setupLoggingRouter(handler), newJSONRequest(http.MethodPost, "/api/monitoring/logs/install", gin.H{"node_name": "node-a", "storage": "10Gi", "retention_days": 14}))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "loki.monitoring.svc") {
		t.Fatalf("unexpected install response: %d %s", response.Code, response.Body.String())
	}
	response = serve(setupLoggingRouter(handler), newJSONRequest(http.MethodGet, "/api/monitoring/logs/filters", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "project-demo") || !strings.Contains(response.Body.String(), "api-123") {
		t.Fatalf("unexpected filters response: %d %s", response.Code, response.Body.String())
	}
}

func loggingReadyK8s() *k8sclient.Client {
	return &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "cylism-loki", Namespace: "monitoring"}, Status: appsv1.StatefulSetStatus{ReadyReplicas: 1}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "cylism-alloy", Namespace: "monitoring"}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 1, NumberAvailable: 1}},
	)}
}
