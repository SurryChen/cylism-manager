package k8s

import (
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestInstallAlertingCreatesSelectedNodeResources(t *testing.T) {
	client := alertingReadyClient()

	status, err := client.InstallAlerting(AlertingConfig{
		NodeName:           "node-b",
		FeishuWebhookURL:   "https://open.feishu.cn/open-apis/bot/v2/hook/example",
		NotificationPolicy: AlertNotificationPolicy{GroupWaitSeconds: 45, GroupIntervalMinutes: 8, RepeatIntervalMinutes: 360},
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.State != AlertingStateInstalling || status.NodeName != "node-b" || !status.NotificationConfigured || !status.FeishuConfigured {
		t.Fatalf("unexpected alerting status: %#v", status)
	}

	alertmanager, err := client.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Get(t.Context(), alertmanagerName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected alertmanager deployment: %v", err)
	}
	if alertmanager.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] != "node-b" {
		t.Fatalf("expected alertmanager node selector, got %#v", alertmanager.Spec.Template.Spec.NodeSelector)
	}
	if len(alertmanager.Spec.Template.Spec.Volumes) == 0 || alertmanager.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim == nil {
		t.Fatalf("expected alertmanager PVC volume: %#v", alertmanager.Spec.Template.Spec.Volumes)
	}
	alertmanagerPVC, err := client.Clientset.CoreV1().PersistentVolumeClaims(victoriaMetricsNamespace).Get(t.Context(), alertmanagerName+"-data", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected alertmanager PVC: %v", err)
	}
	if alertmanagerPVC.Labels[InfrastructureLabel] != InfrastructureAlertmanager || alertmanagerPVC.Labels[ManagedByLabel] != ManagedByValue {
		t.Fatalf("expected alertmanager infrastructure labels: %#v", alertmanagerPVC.Labels)
	}
	vmalert, err := client.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Get(t.Context(), vmalertName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected vmalert deployment: %v", err)
	}
	if vmalert.Spec.Template.Annotations["cylism.io/rules-config-hash"] == "" {
		t.Fatalf("expected vmalert rules checksum annotation: %#v", vmalert.Spec.Template.Annotations)
	}
	kubeStateMetrics, err := client.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Get(t.Context(), kubeStateMetricsName, metav1.GetOptions{})
	if err != nil || !strings.Contains(strings.Join(kubeStateMetrics.Spec.Template.Spec.Containers[0].Args, " "), "--resources=nodes,pods,deployments,statefulsets,daemonsets") {
		t.Fatalf("expected least-privilege kube-state-metrics resource selection: %#v, %v", kubeStateMetrics, err)
	}
	rules, err := client.Clientset.CoreV1().ConfigMaps(victoriaMetricsNamespace).Get(t.Context(), alertingRulesConfigName, metav1.GetOptions{})
	if err != nil || !strings.Contains(rules.Data["alerts.yml"], "NodeCPUHigh") {
		t.Fatalf("expected default rules config: %#v, %v", rules, err)
	}
	secret, err := client.Clientset.CoreV1().Secrets(victoriaMetricsNamespace).Get(t.Context(), alertingSecretName, metav1.GetOptions{})
	if err != nil || len(secret.Data["relay-token"]) < 24 || string(secret.Data["feishu-webhook-url"]) == "" {
		t.Fatalf("expected protected notification secret: %#v, %v", secret, err)
	}
}

func TestInstallAlertingRequiresReadyVictoriaMetrics(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}},
	})}

	_, err := client.InstallAlerting(AlertingConfig{NodeName: "node-a"})
	if err == nil || !strings.Contains(err.Error(), "VictoriaMetrics") {
		t.Fatalf("expected VictoriaMetrics readiness error, got %v", err)
	}
}

func TestInstallAlertingStoresSMTPSettingsOnlyInSecret(t *testing.T) {
	client := alertingReadyClient()
	_, err := client.InstallAlerting(AlertingConfig{NodeName: "node-b", Email: EmailConfig{Enabled: true, SMTPHost: "smtp.example.com", SMTPPort: 587, Username: "alerts", Password: "smtp-password", From: "alerts@example.com", To: "ops@example.com", TLSMode: "starttls"}})
	if err != nil {
		t.Fatal(err)
	}
	secret, err := client.Clientset.CoreV1().Secrets(victoriaMetricsNamespace).Get(t.Context(), alertingSecretName, metav1.GetOptions{})
	if err != nil || string(secret.Data["email-password"]) != "smtp-password" || string(secret.Data["email-to"]) != "ops@example.com" {
		t.Fatalf("expected SMTP settings in Secret: %#v, %v", secret, err)
	}
	settings, err := client.Clientset.CoreV1().ConfigMaps(victoriaMetricsNamespace).Get(t.Context(), alertingConfigName, metav1.GetOptions{})
	if err != nil || strings.Contains(settings.Data["settings.json"], "smtp-password") {
		t.Fatalf("SMTP password must not be stored in ConfigMap: %#v, %v", settings, err)
	}
}

func TestRenderAlertmanagerConfigUsesNotificationPolicy(t *testing.T) {
	config := renderAlertmanagerConfig(AlertNotificationPolicy{GroupWaitSeconds: 45, GroupIntervalMinutes: 8, RepeatIntervalMinutes: 360})
	for _, expected := range []string{"group_wait: 45s", "group_interval: 8m", "repeat_interval: 360m"} {
		if !strings.Contains(config, expected) {
			t.Fatalf("expected %q in Alertmanager config: %s", expected, config)
		}
	}
}

func TestRenderAlertRulesIncludesNotificationContext(t *testing.T) {
	rules := defaultAlertRules()
	config := renderAlertRules(rules)
	for _, expected := range []string{"rule_name:", "current_value: \"{{ $value }}%\"", "current_value: \"{{ $value }} 次/10分钟\"", "threshold: \"85.00%\"", "threshold: \"3 次/10分钟\""} {
		if !strings.Contains(config, expected) {
			t.Fatalf("expected %q in alert rules: %s", expected, config)
		}
	}
}

func TestNormalizeAlertNotificationPolicyUsesLegacyDefaultsAndRejectsInvalidRange(t *testing.T) {
	policy, err := normalizeAlertNotificationPolicy(AlertNotificationPolicy{})
	if err != nil || policy != defaultAlertNotificationPolicy() {
		t.Fatalf("expected legacy defaults, got %#v, %v", policy, err)
	}
	_, err = normalizeAlertNotificationPolicy(AlertNotificationPolicy{GroupWaitSeconds: 1})
	if err == nil || !strings.Contains(err.Error(), "首次通知等待时间") {
		t.Fatalf("expected group wait validation error, got %v", err)
	}
}

func TestNormalizeAlertingConfigRejectsZeroThreshold(t *testing.T) {
	_, err := normalizeAlertingConfig(AlertingConfig{NodeName: "node-a", Rules: []AlertRuleConfig{{ID: "node-cpu-high", Enabled: true, Threshold: 0, DurationMinutes: 15}}})
	if err == nil || !strings.Contains(err.Error(), "阈值必须大于 0") {
		t.Fatalf("expected zero threshold validation error, got %v", err)
	}
}

func TestUpsertAlertingSecretCreatesWhenGetReturnsNotFoundWithEmptyObject(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	clientset.PrependReactor("get", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &corev1.Secret{}, apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, alertingSecretName)
	})
	client := &Client{Clientset: clientset}

	if err := client.upsertAlertingSecret("", EmailConfig{}); err != nil {
		t.Fatalf("expected Secret creation after NotFound, got %v", err)
	}

	actions := clientset.Actions()
	if len(actions) != 2 || actions[1].GetVerb() != "create" {
		t.Fatalf("expected Get followed by Create, got %#v", actions)
	}
	created := actions[1].(k8stesting.CreateAction).GetObject().(*corev1.Secret)
	if len(created.Data["relay-token"]) < 24 {
		t.Fatalf("expected generated relay token, got %#v", created.Data)
	}
}

func alertingReadyClient() *Client {
	return &Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-b"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsName, Namespace: victoriaMetricsNamespace}, Status: appsv1.DeploymentStatus{AvailableReplicas: 1}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: nodeExporterName, Namespace: victoriaMetricsNamespace}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 1, NumberAvailable: 1}},
	)}
}
