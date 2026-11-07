package database

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func TestInstanceUsageReadsOnlyMySQLAndRetainsPodFactsWithoutMetrics(t *testing.T) {
	ctx := context.Background()
	available := true
	pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: ResourceName(2) + "-0", Namespace: Namespace, Labels: map[string]string{k8s.ManagedByLabel: k8s.ManagedByValue, "app.kubernetes.io/name": ResourceName(2)}}, Spec: corev1.PodSpec{NodeName: "worker", Containers: []corev1.Container{{Name: "mysql", Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("2Gi")}}}}}, Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "mysql", Ready: true, RestartCount: 2, LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled"}}}}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/namespaces/"+Namespace+"/pods/"+pod.Name {
			json.NewEncoder(w).Encode(pod)
			return
		}
		if r.URL.Path == "/apis/metrics.k8s.io/v1beta1/namespaces/"+Namespace+"/pods" {
			if !available {
				w.WriteHeader(503)
				return
			}
			fmt.Fprint(w, `{"items":[{"metadata":{"name":"cylism-mysql-2-0"},"containers":[{"name":"mysql","usage":{"cpu":"80m","memory":"600Mi"}},{"name":"sidecar","usage":{"cpu":"1","memory":"1Gi"}}]}]}`)
			return
		}
		t.Errorf("unexpected request %s", r.URL.Path)
		w.WriteHeader(404)
	}))
	defer server.Close()
	client, err := k8s.NewClientFromRestConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	usage, err := ReadInstanceUsage(ctx, client, &model.DatabaseInstance{ID: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !usage.MetricsAvailable || *usage.CPUMilli != 80 || *usage.MemoryBytes != 600<<20 || usage.MemoryLimitBytes != 2<<30 || usage.Restarts != 2 || usage.LastTermination != "OOMKilled" || !usage.Ready {
		t.Fatalf("bad sample: %+v", usage)
	}
	available = false
	usage, err = ReadInstanceUsage(ctx, client, &model.DatabaseInstance{ID: 2})
	if err != nil || usage.MetricsAvailable || usage.CPUMilli != nil || usage.MemoryBytes != nil || usage.Restarts != 2 {
		t.Fatalf("metrics failure became zero: %+v %v", usage, err)
	}
	pod.Labels[k8s.ManagedByLabel] = "other"
	if _, err := ReadInstanceUsage(ctx, client, &model.DatabaseInstance{ID: 2}); err == nil {
		t.Fatal("read unowned pod")
	}
}
