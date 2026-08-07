package runtime

import (
	"context"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestApplyCreatesRuntimeResourcesAndReusesPVC(t *testing.T) {
	client := &k8s.Client{Clientset: fake.NewSimpleClientset()}
	manager := NewKubernetesManager(client)
	instance := &model.RuntimeInstance{ID: 7, Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, Image: "example/nanobot:latest", Namespace: DefaultNamespace, Port: 8080, HealthPath: "/health", PVCName: "nanobot-main-data", Storage: "10Gi", Config: `{"gateway":"enabled"}`}
	if err := manager.Apply(context.Background(), instance, "secret"); err != nil {
		t.Fatalf("apply runtime: %v", err)
	}
	if instance.EndpointURL == "" || instance.SecretName == "" {
		t.Fatalf("expected endpoint and secret name, got %#v", instance)
	}
	if _, err := client.Clientset.CoreV1().Namespaces().Get(context.Background(), DefaultNamespace, metav1.GetOptions{}); err != nil {
		t.Fatalf("namespace not created: %v", err)
	}
	if _, err := client.Clientset.AppsV1().Deployments(DefaultNamespace).Get(context.Background(), instance.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("deployment not created: %v", err)
	}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(DefaultNamespace).Get(context.Background(), instance.PVCName, metav1.GetOptions{}); err != nil {
		t.Fatalf("pvc not created: %v", err)
	}
	if err := manager.Apply(context.Background(), instance, "secret-2"); err != nil {
		t.Fatalf("reapply runtime: %v", err)
	}
	claim, err := client.Clientset.CoreV1().PersistentVolumeClaims(DefaultNamespace).Get(context.Background(), instance.PVCName, metav1.GetOptions{})
	if err != nil || claim.Spec.Resources.Requests.Storage().String() != "10Gi" {
		t.Fatalf("unexpected pvc after reapply: %v %#v", err, claim)
	}
}

func TestApplyRejectsForeignPVC(t *testing.T) {
	client := &k8s.Client{Clientset: fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: DefaultNamespace}})}
	manager := NewKubernetesManager(client)
	instance := &model.RuntimeInstance{ID: 1, Name: "nanobot", RuntimeType: model.RuntimeTypeNanobot, Image: "example/nanobot", Namespace: DefaultNamespace, PVCName: "data", Storage: "1Gi"}
	if err := manager.Apply(context.Background(), instance, ""); err == nil {
		t.Fatal("expected foreign PVC to be rejected")
	}
}

func TestDeleteRejectsForeignPVCWhenDataDeletionIsRequested(t *testing.T) {
	client := &k8s.Client{Clientset: fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: DefaultNamespace, Labels: map[string]string{k8s.ManagedByLabel: k8s.ManagedByValue, RuntimeIDLabel: "99"}}})}
	manager := NewKubernetesManager(client)
	instance := &model.RuntimeInstance{ID: 1, Name: "nanobot", Namespace: DefaultNamespace, PVCName: "data"}
	if err := manager.Delete(context.Background(), instance, true); err == nil {
		t.Fatal("expected foreign PVC deletion to be rejected")
	}
}
