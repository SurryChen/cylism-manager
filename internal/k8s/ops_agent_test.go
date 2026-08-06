package k8s

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestOpsAgentConfigUsesResponsesModelName(t *testing.T) {
	client := &Client{Clientset: fake.NewSimpleClientset()}
	if err := client.upsertOpsAgentConfig(OpsAgentConfig{Model: "gpt-4o-mini", BaseURL: "https://responses.example.test/v1"}); err != nil {
		t.Fatal(err)
	}

	configMap, err := client.Clientset.CoreV1().ConfigMaps(opsAgentNamespace).Get(client.Ctx(), opsAgentName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := configMap.Data["OPS_AGENT_MODEL"]; got != "gpt-4o-mini" {
		t.Fatalf("OPS_AGENT_MODEL = %q, want bare Responses API model name", got)
	}
	if got := configMap.Data["OPENAI_BASE_URL"]; got != "https://responses.example.test/v1" {
		t.Fatalf("OPENAI_BASE_URL = %q, want configured Responses API endpoint", got)
	}
}

func TestOpsAgentStatusWaitsForTheUpdatedDeploymentTemplate(t *testing.T) {
	replicas := int32(1)
	client := &Client{Clientset: fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: opsAgentName, Namespace: opsAgentNamespace, Generation: 2},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, UpdatedReplicas: 1, AvailableReplicas: 1},
	})}

	status := client.OpsAgentStatus()
	if status.State != "installing" {
		t.Fatalf("state = %q, want installing while the new template is unobserved", status.State)
	}
	if status.DesiredReplicas != 1 || status.UpdatedReplicas != 1 || status.ReadyReplicas != 1 {
		t.Fatalf("unexpected replica state: %#v", status)
	}

	deployment, err := client.Clientset.AppsV1().Deployments(opsAgentNamespace).Get(client.Ctx(), opsAgentName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	deployment.Status.ObservedGeneration = 2
	if _, err := client.Clientset.AppsV1().Deployments(opsAgentNamespace).UpdateStatus(client.Ctx(), deployment, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if status = client.OpsAgentStatus(); status.State != "ready" {
		t.Fatalf("state = %q, want ready after the updated template is available", status.State)
	}
}

func TestOpsAgentPVCMigratesToInfrastructureLabels(t *testing.T) {
	client := &Client{Clientset: fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: opsAgentName + "-audit", Namespace: opsAgentNamespace, Labels: opsAgentLabels()},
	})}
	if err := client.upsertOpsAgentPVC(OpsAgentConfig{Storage: "1Gi"}); err != nil {
		t.Fatal(err)
	}
	claim, err := client.Clientset.CoreV1().PersistentVolumeClaims(opsAgentNamespace).Get(client.Ctx(), opsAgentName+"-audit", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if claim.Labels[InfrastructureLabel] != InfrastructureOpsAgent || claim.Labels["cylism.io/component"] != "assistant" {
		t.Fatalf("assistant audit PVC was not migrated to infrastructure labels: %#v", claim.Labels)
	}
}
