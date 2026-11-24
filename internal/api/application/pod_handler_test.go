package applicationapi

import (
	"net/http"
	"strings"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestApplicationPodsOnlyExposeManagedPodsAndMetricsCanBeUnavailable(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	router, store := setupApplicationRouter(&k8sclient.Client{Clientset: clientset})
	app := createApplicationForReleaseRuntimeTest(t, store)
	for _, pod := range []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "managed", Namespace: app.Environment.Namespace, Labels: map[string]string{applicationservice.ManagedByLabel: applicationservice.ManagedByValue, applicationservice.ApplicationNameLabel: app.Name}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}}, Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "main", Ready: true}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: app.Environment.Namespace, Labels: map[string]string{applicationservice.ManagedByLabel: applicationservice.ManagedByValue, applicationservice.ApplicationNameLabel: "other-app"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "unmanaged", Namespace: app.Environment.Namespace, Labels: map[string]string{applicationservice.ApplicationNameLabel: app.Name}}},
	} {
		if _, err := clientset.CoreV1().Pods(app.Environment.Namespace).Create(t.Context(), pod, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	response := serve(router, newJSONRequest(http.MethodGet, "/api/applications/1/pods", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"managed"`) || !strings.Contains(response.Body.String(), `"metrics_available":false`) || strings.Contains(response.Body.String(), `"name":"other"`) || strings.Contains(response.Body.String(), `"name":"unmanaged"`) {
		t.Fatalf("unexpected application pods: %d %s", response.Code, response.Body.String())
	}
	for _, path := range []string{
		"/api/applications/1/pods/other/logs?container=main",
		"/api/applications/1/pods/unmanaged/logs?container=main",
	} {
		response = serve(router, newJSONRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("unrelated pod logs exposed: %d %s", response.Code, response.Body.String())
		}
	}
	response = serve(router, newJSONRequest(http.MethodGet, "/api/applications/1/pods/managed/logs?container=other", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected container accepted: %d %s", response.Code, response.Body.String())
	}
}
