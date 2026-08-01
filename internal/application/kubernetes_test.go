package application

import (
	"context"
	"strings"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestKubernetesApplierPreflightRequiresActiveNamespace(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	applier := NewKubernetesApplier(&k8sclient.Client{Clientset: clientset})
	application := ApplicationContext{Namespace: "dev"}
	if err := applier.Preflight(context.Background(), application, ReleaseSpec{Endpoint: EndpointSpec{Exposure: ExposureCluster}}); err == nil || !strings.Contains(err.Error(), "命名空间") {
		t.Fatalf("expected missing namespace preflight failure, got %v", err)
	}
	if _, err := clientset.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dev"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := applier.Preflight(context.Background(), application, ReleaseSpec{Endpoint: EndpointSpec{Exposure: ExposureCluster}}); err != nil {
		t.Fatalf("expected active namespace preflight to pass: %v", err)
	}
}
