package k8s

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
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

func TestRuntimePodMetricsUsesLiveMetricsAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apis/metrics.k8s.io/v1beta1/namespaces/demo/pods" {
			t.Errorf("unexpected metrics path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"api-1"},"containers":[{"name":"mysql","usage":{"cpu":"50m","memory":"64Mi"}},{"name":"sidecar","usage":{"cpu":"25m","memory":"32Mi"}}]}]}`))
	}))
	defer server.Close()
	client, err := NewClientFromRestConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := client.ListRuntimePodMetrics(t.Context(), "demo")
	if err != nil || metrics["api-1"].CPUMilli != 75 || metrics["api-1"].MemoryBytes != 96*1024*1024 {
		t.Fatalf("unexpected pod metrics: %#v, %v", metrics, err)
	}
	containers := metrics["api-1"].Containers
	if len(containers) != 2 || containers[0].Name != "mysql" || containers[0].CPUMilli != 50 || containers[0].MemoryBytes != 64*1024*1024 || containers[1].Name != "sidecar" || containers[1].CPUMilli != 25 || containers[1].MemoryBytes != 32*1024*1024 {
		t.Fatalf("unexpected container metrics: %#v", containers)
	}
}
