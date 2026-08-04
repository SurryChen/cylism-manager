package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func setupAlertingRouter(handler *AlertingHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/monitoring/alerts")
	group.GET("/status", handler.Status)
	group.POST("/install", handler.Install)
	group.PUT("/config", handler.Update)
	group.DELETE("", handler.Uninstall)
	group.GET("/overview", handler.Overview)
	group.GET("/silences", handler.ListSilences)
	group.POST("/silences", handler.CreateSilence)
	group.DELETE("/silences/:id", handler.DeleteSilence)
	group.POST("/test-notification", handler.TestNotification)
	router.POST("/api/monitoring/alerts/notify", handler.Notify)
	return router
}

func TestAlertingNotifyRejectsMissingOrInvalidToken(t *testing.T) {
	original := K8s
	K8s = alertingReadyK8s("relay-token")
	defer func() { K8s = original }()

	handler := NewAlertingHandler()
	handler.notify = func(context.Context, string, alertmanagerNotification) error {
		t.Fatal("notification must not be forwarded without a valid token")
		return nil
	}
	response := serve(setupAlertingRouter(handler), newJSONRequest(http.MethodPost, "/api/monitoring/alerts/notify", gin.H{"alerts": []gin.H{}}))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized callback, got %s", response.Body.String())
	}
}

func TestAlertingNotifyForwardsAuthorizedAlertBatch(t *testing.T) {
	original := K8s
	K8s = alertingReadyK8s("relay-token")
	defer func() { K8s = original }()

	called := false
	handler := NewAlertingHandler()
	handler.notify = func(_ context.Context, url string, payload alertmanagerNotification) error {
		called = true
		if url == "" || len(payload.Alerts) != 1 || payload.Alerts[0].Labels["alertname"] != "NodeDown" {
			t.Fatalf("unexpected forwarded payload: %#v %q", payload, url)
		}
		return nil
	}
	request := newJSONRequest(http.MethodPost, "/api/monitoring/alerts/notify", gin.H{"alerts": []gin.H{{"status": "firing", "labels": gin.H{"alertname": "NodeDown"}}}})
	request.Header.Set("Authorization", "Bearer relay-token")
	response := serve(setupAlertingRouter(handler), request)
	if response.Code != http.StatusOK || !called {
		t.Fatalf("expected forwarded callback, got %d %s", response.Code, response.Body.String())
	}
}

func TestAlertingTestNotificationDoesNotExposeWebhook(t *testing.T) {
	original := K8s
	K8s = alertingReadyK8s("relay-token")
	defer func() { K8s = original }()

	handler := NewAlertingHandler()
	handler.notify = func(_ context.Context, url string, payload alertmanagerNotification) error {
		if url != "https://open.feishu.cn/open-apis/bot/v2/hook/example" || len(payload.Alerts) != 1 {
			t.Fatalf("unexpected test notification: %q %#v", url, payload)
		}
		return nil
	}
	response := serve(setupAlertingRouter(handler), newJSONRequest(http.MethodPost, "/api/monitoring/alerts/test-notification", nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "open.feishu.cn") {
		t.Fatalf("test response must be successful and redacted: %s", response.Body.String())
	}
}

func TestAlertingOverviewSurfacesAlertmanagerFailure(t *testing.T) {
	original := K8s
	K8s = alertingReadyK8s("relay-token")
	defer func() { K8s = original }()

	handler := NewAlertingHandler()
	handler.alertmanager = func(context.Context, string, string, interface{}, interface{}) error {
		return errors.New("connection refused")
	}
	response := serve(setupAlertingRouter(handler), newJSONRequest(http.MethodGet, "/api/monitoring/alerts/overview", nil))
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "Alertmanager") {
		t.Fatalf("expected Alertmanager error, got %s", response.Body.String())
	}
}

func TestAlertingOverviewIncludesRecentResolvedWebhookAlerts(t *testing.T) {
	original := K8s
	K8s = alertingReadyK8s("relay-token")
	defer func() { K8s = original }()

	handler := NewAlertingHandler()
	handler.notify = func(context.Context, string, alertmanagerNotification) error { return nil }
	handler.alertmanager = func(_ context.Context, method, _ string, _ interface{}, output interface{}) error {
		if method == http.MethodGet {
			*output.(*[]alertmanagerAlert) = []alertmanagerAlert{}
		}
		return nil
	}
	callback := newJSONRequest(http.MethodPost, "/api/monitoring/alerts/notify", gin.H{"status": "resolved", "alerts": []gin.H{{"status": "resolved", "fingerprint": "resolved-1", "labels": gin.H{"alertname": "NodeDown"}, "annotations": gin.H{"summary": "节点已恢复"}}}})
	callback.Header.Set("Authorization", "Bearer relay-token")
	if response := serve(setupAlertingRouter(handler), callback); response.Code != http.StatusOK {
		t.Fatalf("unexpected callback response: %s", response.Body.String())
	}
	response := serve(setupAlertingRouter(handler), newJSONRequest(http.MethodGet, "/api/monitoring/alerts/overview", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "节点已恢复") {
		t.Fatalf("expected recent resolved alert, got %s", response.Body.String())
	}
}

func alertingReadyK8s(token string) *k8sclient.Client {
	return &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "cylism-victoria-metrics", Namespace: "monitoring"}, Status: appsv1.DeploymentStatus{AvailableReplicas: 1}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "cylism-node-exporter", Namespace: "monitoring"}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 1, NumberAvailable: 1}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "cylism-alertmanager", Namespace: "monitoring"}, Status: appsv1.DeploymentStatus{AvailableReplicas: 1}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "cylism-vmalert", Namespace: "monitoring"}, Status: appsv1.DeploymentStatus{AvailableReplicas: 1}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "cylism-kube-state-metrics", Namespace: "monitoring"}, Status: appsv1.DeploymentStatus{AvailableReplicas: 1}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cylism-alerting-config", Namespace: "monitoring"}, Data: map[string]string{"settings.json": `{"node_name":"node-a","rules":[]}`}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "cylism-alerting-secret", Namespace: "monitoring"}, Data: map[string][]byte{"relay-token": []byte(token), "feishu-webhook-url": []byte("https://open.feishu.cn/open-apis/bot/v2/hook/example")}},
	)}
}
