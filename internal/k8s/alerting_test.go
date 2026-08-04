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
		NodeName:         "node-b",
		FeishuWebhookURL: "https://open.feishu.cn/open-apis/bot/v2/hook/example",
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.State != AlertingStateInstalling || status.NodeName != "node-b" || !status.NotificationConfigured {
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
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(victoriaMetricsNamespace).Get(t.Context(), alertmanagerName+"-data", metav1.GetOptions{}); err != nil {
		t.Fatalf("expected alertmanager PVC: %v", err)
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

func TestUpsertAlertingSecretCreatesWhenGetReturnsNotFoundWithEmptyObject(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	clientset.PrependReactor("get", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &corev1.Secret{}, apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, alertingSecretName)
	})
	client := &Client{Clientset: clientset}

	if err := client.upsertAlertingSecret(""); err != nil {
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
