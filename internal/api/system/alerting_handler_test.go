package system

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	alertingservice "github.com/cylism/cylism-manager/internal/service/observability/alerting"
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
	group.PUT("/automation-policy", handler.UpdateAutomationPolicy)
	group.GET("/automation-events", handler.ListAutomationEvents)
	router.POST("/api/monitoring/alerts/notify", handler.Notify)
	return router
}

func TestAlertingNotifyRejectsMissingOrInvalidToken(t *testing.T) {
	original := k8sClient
	k8sClient = alertingReadyK8s("relay-token")
	defer func() { k8sClient = original }()

	handler, sender := newTestAlertingHandler()
	sender.feishu = func(context.Context, string, alertingservice.AlertNotification) error {
		t.Fatal("notification must not be forwarded without a valid token")
		return nil
	}
	response := serve(setupAlertingRouter(handler), newJSONRequest(http.MethodPost, "/api/monitoring/alerts/notify", gin.H{"alerts": []gin.H{}}))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized callback, got %s", response.Body.String())
	}
}

func TestAlertingNotifyForwardsAuthorizedAlertBatch(t *testing.T) {
	original := k8sClient
	k8sClient = alertingReadyK8s("relay-token")
	defer func() { k8sClient = original }()

	called := false
	handler, sender := newTestAlertingHandler()
	sender.feishu = func(_ context.Context, url string, payload alertingservice.AlertNotification) error {
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
	original := k8sClient
	k8sClient = alertingReadyK8s("relay-token")
	defer func() { k8sClient = original }()

	handler, sender := newTestAlertingHandler()
	sender.feishu = func(_ context.Context, url string, payload alertingservice.AlertNotification) error {
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
	handler, _ := newTestAlertingHandler("https://alerts.example.com/platform")
	if handler.platformURL != "https://alerts.example.com/#/monitoring?tab=alerts" {
		t.Fatalf("unexpected configured platform URL: %q", handler.platformURL)
	}
}

func TestAlertingTestNotificationSendsSMTPEmail(t *testing.T) {
	original := k8sClient
	k8sClient = alertingReadyK8s("relay-token")
	defer func() { k8sClient = original }()
	secret, err := k8sClient.Clientset.CoreV1().Secrets("monitoring").Get(t.Context(), "cylism-alerting-secret", metav1.GetOptions{})
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
	if _, err := k8sClient.Clientset.CoreV1().Secrets("monitoring").Update(t.Context(), secret, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	handler, sender := newTestAlertingHandler()
	sender.feishu = func(context.Context, string, alertingservice.AlertNotification) error {
		t.Fatal("email-only test must not send a Feishu notification")
		return nil
	}
	sender.email = func(_ context.Context, config alertingservice.EmailConfig, payload alertingservice.AlertNotification) error {
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
	feishu, err := json.Marshal(alertingservice.BuildFeishuCard(alertingservice.AlertNotification{Status: payload.Status, Alerts: payload.Alerts}, payload.PlatformURL))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"告警规则", "当前值", "92.4%", "85%", "查看平台告警", "https://cylism.example.com/#/monitoring?tab=alerts"} {
		if !strings.Contains(string(feishu), expected) {
			t.Fatalf("expected %q in Feishu card: %s", expected, feishu)
		}
	}
	for _, expected := range []string{"告警规则", "当前值", "92.4%", "查看平台告警", "https://cylism.example.com/#/monitoring?tab=alerts"} {
		if !strings.Contains(alertingservice.BuildEmailHTML(alertingservice.AlertNotification{Status: payload.Status, Alerts: payload.Alerts}, payload.PlatformURL), expected) {
			t.Fatalf("expected %q in HTML email", expected)
		}
	}
}

func TestAlertEmailMIMEWrapsLongBodyLines(t *testing.T) {
	payload := alertingservice.AlertNotification{Status: "firing", Alerts: []alertingservice.Alert{{Annotations: map[string]string{"summary": strings.Repeat("alert content ", 300)}}}}
	message := alertingservice.BuildEmailMIME("alerts@example.com", "ops@example.com", payload, "")
	if !strings.Contains(message, "Content-Transfer-Encoding: quoted-printable") {
		t.Fatalf("expected quoted-printable body encoding: %s", message)
	}
	if alertingservice.MaximumSMTPLineLength(message) > 998 {
		t.Fatalf("SMTP line exceeds 998 bytes: %d", alertingservice.MaximumSMTPLineLength(message))
	}
}

func TestAlertingOverviewSurfacesAlertmanagerFailure(t *testing.T) {
	original := k8sClient
	k8sClient = alertingReadyK8s("relay-token")
	defer func() { k8sClient = original }()

	handler, _ := newTestAlertingHandler()
	handler.WithAlertmanager(func(context.Context, string, string, interface{}, interface{}) error {
		return errors.New("connection refused")
	}, func(context.Context) bool { return true })
	response := serve(setupAlertingRouter(handler), newJSONRequest(http.MethodGet, "/api/monitoring/alerts/overview", nil))
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "Alertmanager") {
		t.Fatalf("expected Alertmanager error, got %s", response.Body.String())
	}
}

func TestAlertingSilenceHandlersDelegateToWorkflowClient(t *testing.T) {
	original := k8sClient
	k8sClient = alertingReadyK8s("relay-token")
	defer func() { k8sClient = original }()

	requests := make([]string, 0, 3)
	handler, _ := newTestAlertingHandler()
	handler.WithAlertmanager(func(_ context.Context, method, path string, input interface{}, output interface{}) error {
		requests = append(requests, method+" "+path)
		switch method {
		case http.MethodGet:
			*output.(*[]alertingservice.Silence) = []alertingservice.Silence{{ID: "silence-1"}}
		case http.MethodPost:
			request, ok := input.(alertingservice.Silence)
			if !ok || request.Comment != "maintenance" || len(request.Matchers) != 1 {
				t.Fatalf("silence request was not delegated correctly: %#v", input)
			}
			*output.(*alertingservice.SilenceResult) = alertingservice.SilenceResult{SilenceID: "silence-2"}
		}
		return nil
	}, func(context.Context) bool { return true })
	router := setupAlertingRouter(handler)
	if response := serve(router, newJSONRequest(http.MethodGet, "/api/monitoring/alerts/silences", nil)); response.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", response.Code, response.Body.String())
	}
	if response := serve(router, newJSONRequest(http.MethodPost, "/api/monitoring/alerts/silences", gin.H{"duration_minutes": 30, "comment": "maintenance", "matchers": []gin.H{{"name": "alertname", "value": "NodeDown", "is_equal": true}}})); response.Code != http.StatusOK {
		t.Fatalf("create status = %d: %s", response.Code, response.Body.String())
	}
	if response := serve(router, newJSONRequest(http.MethodDelete, "/api/monitoring/alerts/silences/silence-2", nil)); response.Code != http.StatusOK {
		t.Fatalf("delete status = %d: %s", response.Code, response.Body.String())
	}
	if strings.Join(requests, ",") != "GET /api/v2/silences,POST /api/v2/silences,DELETE /api/v2/silence/silence-2" {
		t.Fatalf("unexpected delegated Alertmanager requests: %#v", requests)
	}
}

func TestAlertingOverviewIncludesRecentResolvedWebhookAlerts(t *testing.T) {
	original := k8sClient
	k8sClient = alertingReadyK8s("relay-token")
	defer func() { k8sClient = original }()

	handler, sender := newTestAlertingHandler()
	sender.feishu = func(context.Context, string, alertingservice.AlertNotification) error { return nil }
	handler.WithAlertmanager(func(_ context.Context, method, _ string, _ interface{}, output interface{}) error {
		if method == http.MethodGet {
			*output.(*[]alertmanagerAlert) = []alertmanagerAlert{}
		}
		return nil
	}, func(context.Context) bool { return true })
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

func TestUpdateAutomationPolicyImmediatelySyncsCurrentFiringAlerts(t *testing.T) {
	original := k8sClient
	k8sClient = alertingReadyK8s("relay-token")
	defer func() { k8sClient = original }()

	store := &memoryAlertAutomationStore{runtime: &model.RuntimeInstance{ID: 7, RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged}, events: map[string]*model.AlertEvent{}}
	dispatched := make(chan *model.AlertEvent, 1)
	handler, _ := newTestAlertingHandler()
	handler.WithAutomation(store, alertDispatcherFunc(func(_ context.Context, event *model.AlertEvent) { dispatched <- event }))
	handler.WithAlertmanager(func(_ context.Context, method, path string, _ interface{}, output interface{}) error {
		if method != http.MethodGet || path != "/api/v2/alerts" {
			t.Fatalf("unexpected Alertmanager request: %s %s", method, path)
		}
		*output.(*[]alertmanagerAlert) = []alertmanagerAlert{
			{Fingerprint: "firing-1", Status: alertStatus{State: "active"}, Labels: map[string]string{"alertname": "NodeDiskHigh", "severity": "warning", "node": "node-a"}},
			{Fingerprint: "resolved-1", Status: alertStatus{State: "resolved"}, Labels: map[string]string{"alertname": "NodeDiskHigh", "severity": "warning"}},
		}
		return nil
	}, func(context.Context) bool { return true })

	response := serve(setupAlertingRouter(handler), newJSONRequest(http.MethodPut, "/api/monitoring/alerts/automation-policy", gin.H{
		"runtime_id": 7, "enabled": true, "minimum_severity": "warning", "mode": "report_only", "cooldown_minutes": 30,
	}))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"synced":1`) {
		t.Fatalf("expected one immediately synced alert, got %d %s", response.Code, response.Body.String())
	}
	select {
	case event := <-dispatched:
		if event.Fingerprint != "firing-1" || event.Status != model.AlertEventAnalyzing {
			t.Fatalf("unexpected dispatched event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("expected current firing alert to be dispatched")
	}
}

func TestListAutomationEventsDelegatesToWorkflowStore(t *testing.T) {
	original := k8sClient
	k8sClient = alertingReadyK8s("relay-token")
	defer func() { k8sClient = original }()

	store := &memoryAlertAutomationStore{events: map[string]*model.AlertEvent{
		"event-1": {ID: 1, Fingerprint: "event-1", AlertName: "NodeDown"},
	}}
	handler, _ := newTestAlertingHandler()
	handler.WithAutomation(store, nil)
	response := serve(setupAlertingRouter(handler), newJSONRequest(http.MethodGet, "/api/monitoring/alerts/automation-events", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "NodeDown") {
		t.Fatalf("expected automation events from workflow store, got %d %s", response.Code, response.Body.String())
	}
}

func TestAlertAutomationPromptDistinguishesLifecycleAndAlertState(t *testing.T) {
	prompt := alertAutomationPrompt(&model.AlertEvent{ID: 7, NodeName: "node-a"}, model.AlertAutomationApproval)
	for _, expected := range []string{"automation_status", "alert_state", `alert_state="firing"`, "DiskPressure", "KubeletHasNoDiskPressure"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected %q in automation prompt: %s", expected, prompt)
		}
	}
}

type memoryAlertAutomationStore struct {
	policy  *model.AlertAutomationPolicy
	runtime *model.RuntimeInstance
	events  map[string]*model.AlertEvent
}

func (s *memoryAlertAutomationStore) UpsertAlertEvent(event *model.AlertEvent) (*model.AlertEvent, error) {
	if existing, ok := s.events[event.Fingerprint]; ok {
		return existing, nil
	}
	event.ID = uint(len(s.events) + 1)
	s.events[event.Fingerprint] = event
	return event, nil
}

func (s *memoryAlertAutomationStore) GetAlertAutomationPolicy() (*model.AlertAutomationPolicy, error) {
	if s.policy == nil {
		return nil, errors.New("policy not found")
	}
	return s.policy, nil
}

func (s *memoryAlertAutomationStore) SaveAlertAutomationPolicy(policy *model.AlertAutomationPolicy) error {
	copy := *policy
	s.policy = &copy
	return nil
}

func (s *memoryAlertAutomationStore) ListAlertEvents(int) ([]model.AlertEvent, error) {
	result := make([]model.AlertEvent, 0, len(s.events))
	for _, event := range s.events {
		result = append(result, *event)
	}
	return result, nil
}

func (s *memoryAlertAutomationStore) UpdateAlertEvent(event *model.AlertEvent) error {
	s.events[event.Fingerprint] = event
	return nil
}

func (s *memoryAlertAutomationStore) GetRuntime(uint) (*model.RuntimeInstance, error) {
	return s.runtime, nil
}

type alertDispatcherFunc func(context.Context, *model.AlertEvent)

func (f alertDispatcherFunc) Dispatch(ctx context.Context, event *model.AlertEvent) { f(ctx, event) }

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
