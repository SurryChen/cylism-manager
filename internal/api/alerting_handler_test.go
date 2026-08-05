package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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
		if url != "https://open.feishu.cn/open-apis/bot/v2/hook/example" || len(payload.Alerts) != 1 || payload.PlatformURL != "https://cylism.crazycoding.top/#/monitoring?tab=alerts" || payload.Alerts[0].Labels["node"] != "示例节点" || payload.Alerts[0].Annotations["current_value"] != "92.4%" || payload.Alerts[0].Annotations["threshold"] != "85%" || payload.Alerts[0].Annotations["duration"] != "10 分钟" {
			t.Fatalf("unexpected test notification: %q %#v", url, payload)
		}
		return nil
	}
	response := serve(setupAlertingRouter(handler), newJSONRequest(http.MethodPost, "/api/monitoring/alerts/test-notification?channel=feishu", nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "open.feishu.cn") {
		t.Fatalf("test response must be successful and redacted: %s", response.Body.String())
	}
}

func TestNewAlertingHandlerUsesConfiguredPlatformURL(t *testing.T) {
	handler := NewAlertingHandler("https://alerts.example.com/platform")
	if handler.platformURL != "https://alerts.example.com/#/monitoring?tab=alerts" {
		t.Fatalf("unexpected configured platform URL: %q", handler.platformURL)
	}
}

func TestAlertingTestNotificationSendsSMTPEmail(t *testing.T) {
	original := K8s
	K8s = alertingReadyK8s("relay-token")
	defer func() { K8s = original }()
	secret, err := K8s.Clientset.CoreV1().Secrets("monitoring").Get(t.Context(), "cylism-alerting-secret", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	secret.Data["email-smtp-host"] = []byte("smtp.example.com")
	secret.Data["email-smtp-port"] = []byte("587")
	secret.Data["email-username"] = []byte("alerts")
	secret.Data["email-password"] = []byte("smtp-password")
	secret.Data["email-from"] = []byte("alerts@example.com")
	secret.Data["email-to"] = []byte("ops@example.com")
	secret.Data["email-tls-mode"] = []byte("starttls")
	if _, err := K8s.Clientset.CoreV1().Secrets("monitoring").Update(t.Context(), secret, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	handler := NewAlertingHandler()
	handler.notify = func(context.Context, string, alertmanagerNotification) error {
		t.Fatal("email-only test must not send a Feishu notification")
		return nil
	}
	handler.emailNotify = func(_ context.Context, config k8sclient.EmailConfig, payload alertmanagerNotification) error {
		if config.SMTPHost != "smtp.example.com" || config.Password != "smtp-password" || len(payload.Alerts) != 1 {
			t.Fatalf("unexpected email notification: %#v %#v", config, payload)
		}
		return nil
	}
	response := serve(setupAlertingRouter(handler), newJSONRequest(http.MethodPost, "/api/monitoring/alerts/test-notification?channel=email", nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "smtp-password") {
		t.Fatalf("expected redacted successful SMTP test: %s", response.Body.String())
	}
}

func TestAlertNotificationMessagesIncludeContextAndPlatformLink(t *testing.T) {
	payload := alertmanagerNotification{
		Status:      "firing",
		PlatformURL: "https://cylism.example.com/#/monitoring?tab=alerts",
		Alerts: []alertmanagerAlert{{
			Status:      alertStatus{State: "firing"},
			Labels:      map[string]string{"alertname": "NodeDiskHigh", "node": "node-a", "severity": "warning"},
			Annotations: map[string]string{"summary": "节点根磁盘空间不足", "rule_name": "节点根磁盘使用率过高", "current_value": "92.4%", "threshold": "85%", "duration": "15 分钟"},
			StartsAt:    time.Date(2026, time.August, 5, 10, 30, 0, 0, time.UTC),
		}},
	}
	feishu, err := json.Marshal(feishuMessage(payload))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"告警规则", "当前值", "92.4%", "85%", "查看平台告警", "https://cylism.example.com/#/monitoring?tab=alerts"} {
		if !strings.Contains(string(feishu), expected) {
			t.Fatalf("expected %q in Feishu card: %s", expected, feishu)
		}
	}
	for _, expected := range []string{"告警规则", "当前值", "92.4%", "查看平台告警", "https://cylism.example.com/#/monitoring?tab=alerts"} {
		if !strings.Contains(emailHTMLMessage(payload), expected) {
			t.Fatalf("expected %q in HTML email", expected)
		}
	}
}

func TestAlertEmailMIMEWrapsLongBodyLines(t *testing.T) {
	message := alertEmailMIME("alerts@example.com", "ops@example.com", alertmanagerNotification{Status: "firing"}, strings.Repeat("alert content ", 300), "<div>"+strings.Repeat("alert content ", 300)+"</div>")
	if !strings.Contains(message, "Content-Transfer-Encoding: quoted-printable") {
		t.Fatalf("expected quoted-printable body encoding: %s", message)
	}
	if maximumSMTPLineLength(message) > 998 {
		t.Fatalf("SMTP line exceeds 998 bytes: %d", maximumSMTPLineLength(message))
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
