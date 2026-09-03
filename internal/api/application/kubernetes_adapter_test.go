package applicationapi

import (
	"context"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestNewKubernetesAdapterNilClient(t *testing.T) {
	if deps := NewKubernetesAdapter(nil); deps != nil {
		t.Fatal("nil Kubernetes client should produce unavailable dependencies")
	}
	if client := NewNamespaceClient(nil); client != nil {
		t.Fatal("nil Kubernetes client should produce no namespace client")
	}
}

func TestKubernetesAdapterExposeNarrowCapabilities(t *testing.T) {
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()}
	deps := NewKubernetesAdapter(client)
	if deps == nil || !deps.KubernetesAvailable() {
		t.Fatal("expected available Kubernetes dependency port")
	}
	ns, err := deps.CreateNamespace(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "application-test"}})
	if err != nil || ns.Name != "application-test" {
		t.Fatalf("namespace capability failed: %#v, %v", ns, err)
	}
	if got, err := deps.GetNamespace(context.Background(), "application-test"); err != nil || got.Name != "application-test" {
		t.Fatalf("namespace read capability failed: %#v, %v", got, err)
	}
}
