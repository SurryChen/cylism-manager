package k8s

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestRuntimeReaderScopesReadsAndSelectors(t *testing.T) {
	client := &Client{Clientset: fake.NewSimpleClientset(
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "demo", Labels: map[string]string{"managed": "true"}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "other", Labels: map[string]string{"managed": "true"}}},
	)}
	ctx := context.Background()
	services, err := client.ListRuntimeServices(ctx, "demo")
	if err != nil || len(services) != 1 || services[0].Name != "api" {
		t.Fatalf("unexpected runtime services: %#v, %v", services, err)
	}
	pods, err := client.ListRuntimePods(ctx, "demo", "managed=true")
	if err != nil || len(pods) != 1 || pods[0].Name != "api-1" {
		t.Fatalf("unexpected filtered runtime pods: %#v, %v", pods, err)
	}
}
