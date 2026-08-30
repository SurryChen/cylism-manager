package infrastructure

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func setupK8sTestRouter(clients ...*k8sclient.Client) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var client *k8sclient.Client
	if len(clients) > 0 {
		client = clients[0]
	}
	h := NewK8sHandlerWithClient(client, nil)
	g := r.Group("/api/k8s")
	{
		g.GET("/dashboard", h.Dashboard)
		g.GET("/namespaces", h.ListNamespaces)
		g.GET("/namespace-names", h.ListNamespaceNames)
		g.POST("/namespaces", h.CreateNamespace)
		g.PATCH("/namespaces/:name", h.UpdateNamespace)
		g.DELETE("/namespaces/:name", h.DeleteNamespace)
		g.GET("/pods", h.ListPods)
		g.GET("/pods/:namespace/:name/terminal", h.PodTerminal)
		g.GET("/deployments", h.ListDeployments)
		g.GET("/deployments/:namespace/:name", h.GetDeployment)
		g.GET("/deployments/:namespace/:name/pods", h.ListDeploymentPods)
		g.GET("/deployments/:namespace/:name/revisions", h.ListDeploymentRevisions)
		g.PATCH("/deployments/:namespace/:name/scale", h.ScaleDeployment)
		g.PATCH("/deployments/:namespace/:name/image", h.UpdateDeploymentImage)
		g.POST("/deployments/:namespace/:name/rollback", h.RollbackDeployment)
		g.GET("/statefulsets", h.ListStatefulSets)
		g.GET("/statefulsets/:namespace/:name", h.GetStatefulSet)
		g.PATCH("/statefulsets/:namespace/:name/scale", h.ScaleStatefulSet)
		g.GET("/daemonsets", h.ListDaemonSets)
		g.GET("/daemonsets/:namespace/:name", h.GetDaemonSet)
		g.GET("/services", h.ListServicesV2)
		g.GET("/services/:namespace/:name", h.GetService)
		g.GET("/services/:namespace/:name/endpoints", h.GetServiceEndpoints)
		g.DELETE("/services/:namespace/:name", h.DeleteService)
		g.GET("/configmaps", h.ListConfigMaps)
		g.POST("/configmaps", h.CreateConfigMap)
		g.GET("/configmaps/:namespace/:name", h.GetConfigMap)
		g.PUT("/configmaps/:namespace/:name", h.UpdateConfigMap)
		g.DELETE("/configmaps/:namespace/:name", h.DeleteConfigMap)
		g.GET("/secrets", h.ListSecrets)
		g.POST("/secrets", h.CreateOpaqueSecret)
		g.GET("/secrets/:namespace/:name", h.GetSecret)
		g.PUT("/secrets/:namespace/:name", h.UpdateOpaqueSecret)
		g.DELETE("/secrets/:namespace/:name", h.DeleteOpaqueSecret)
		g.GET("/ingresses", h.ListIngresses)
		g.GET("/ingresses/:namespace/:name", h.GetIngress)
		g.POST("/ingresses", h.CreateIngress)
		g.DELETE("/ingresses/:namespace/:name", h.DeleteIngress)
		g.GET("/ingress-controller", h.GetIngressController)
	}
	return r
}

func jsonBody(obj interface{}) io.Reader {
	b, _ := json.Marshal(obj)
	return strings.NewReader(string(b))
}

// noK8sGet/Post 等辅助：无 K8s 时，所有 handler 必须返回 200
func doNoK8s(t *testing.T, method, path string, body io.Reader) {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	r := setupK8sTestRouter(nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("%s %s: expected 200, got %d. body=%s", method, path, w.Code, w.Body.String())
	}
}

func TestNoK8s_AllEndpoints(t *testing.T) {
	func() {
		doNoK8s(t, "GET", "/api/k8s/dashboard", nil)
		doNoK8s(t, "GET", "/api/k8s/namespaces", nil)
		doNoK8s(t, "GET", "/api/k8s/namespace-names", nil)
		doNoK8s(t, "POST", "/api/k8s/namespaces", jsonBody(map[string]any{"name": "demo"}))
		doNoK8s(t, "PATCH", "/api/k8s/namespaces/demo", jsonBody(map[string]any{"labels": map[string]string{"team": "ops"}}))
		doNoK8s(t, "DELETE", "/api/k8s/namespaces/demo", nil)
		doNoK8s(t, "GET", "/api/k8s/pods", nil)
		doNoK8s(t, "GET", "/api/k8s/pods/default/web/terminal", nil)
		doNoK8s(t, "GET", "/api/k8s/deployments", nil)
		doNoK8s(t, "GET", "/api/k8s/deployments/default/web", nil)
		doNoK8s(t, "GET", "/api/k8s/deployments/default/web/pods", nil)
		doNoK8s(t, "GET", "/api/k8s/deployments/default/web/revisions", nil)
		doNoK8s(t, "PATCH", "/api/k8s/deployments/default/web/scale", jsonBody(map[string]int{"replicas": 3}))
		doNoK8s(t, "PATCH", "/api/k8s/deployments/default/web/image", jsonBody(map[string]string{"container": "app", "image": "nginx:1"}))
		doNoK8s(t, "POST", "/api/k8s/deployments/default/web/rollback", jsonBody(map[string]int{"revision": 2}))
		doNoK8s(t, "GET", "/api/k8s/statefulsets", nil)
		doNoK8s(t, "GET", "/api/k8s/statefulsets/default/db", nil)
		doNoK8s(t, "PATCH", "/api/k8s/statefulsets/default/db/scale", jsonBody(map[string]int{"replicas": 2}))
		doNoK8s(t, "GET", "/api/k8s/daemonsets", nil)
		doNoK8s(t, "GET", "/api/k8s/daemonsets/kube-system/traefik", nil)
		doNoK8s(t, "GET", "/api/k8s/services", nil)
		doNoK8s(t, "GET", "/api/k8s/services/default/web/endpoints", nil)
		doNoK8s(t, "DELETE", "/api/k8s/services/default/web", nil)
		doNoK8s(t, "GET", "/api/k8s/configmaps", nil)
		doNoK8s(t, "POST", "/api/k8s/configmaps", jsonBody(map[string]any{"namespace": "default", "name": "app-config", "data": map[string]string{"config.yaml": "value"}}))
		doNoK8s(t, "GET", "/api/k8s/configmaps/default/app", nil)
		doNoK8s(t, "PUT", "/api/k8s/configmaps/default/app", jsonBody(map[string]any{"data": map[string]string{"config.yaml": "value"}}))
		doNoK8s(t, "DELETE", "/api/k8s/configmaps/default/app", nil)
		doNoK8s(t, "GET", "/api/k8s/secrets", nil)
		doNoK8s(t, "POST", "/api/k8s/secrets", jsonBody(map[string]any{"namespace": "default", "name": "app-secret", "data": map[string]string{"token": "value"}}))
		doNoK8s(t, "GET", "/api/k8s/secrets/default/db-pass", nil)
		doNoK8s(t, "PUT", "/api/k8s/secrets/default/db-pass", jsonBody(map[string]any{"data": map[string]string{"token": ""}}))
		doNoK8s(t, "DELETE", "/api/k8s/secrets/default/db-pass", nil)
		doNoK8s(t, "GET", "/api/k8s/ingresses", nil)
		doNoK8s(t, "GET", "/api/k8s/ingresses/default/web", nil)
		doNoK8s(t, "POST", "/api/k8s/ingresses", jsonBody(map[string]string{"namespace": "default", "name": "test", "host": "example.com", "service_name": "web", "service_port": "http"}))
		doNoK8s(t, "DELETE", "/api/k8s/ingresses/default/web", nil)
		doNoK8s(t, "GET", "/api/k8s/ingress-controller", nil)
	}()
}

func TestListNamespaceNamesDoesNotLoadResourceSummaries(t *testing.T) {
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "monitoring"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}},
	)}

	response := serve(setupK8sTestRouter(client), httptest.NewRequest(http.MethodGet, "/api/k8s/namespace-names", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"monitoring"`) {
		t.Fatalf("unexpected namespace name response: %s", response.Body.String())
	}
	for _, action := range client.Clientset.(*k8sfake.Clientset).Actions() {
		if action.GetResource().Resource != "namespaces" {
			t.Fatalf("unexpected resource lookup: %#v", action)
		}
	}
}

func TestListPodsIncludesContainers(t *testing.T) {
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api-0", Namespace: "default"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}, {Name: "metrics"}}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	})}

	r := setupK8sTestRouter(client)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/k8s/pods", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var response struct {
		Data []struct {
			Name       string   `json:"name"`
			Containers []string `json:"containers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data) != 1 || response.Data[0].Name != "api-0" {
		t.Fatalf("unexpected Pod response: %#v", response.Data)
	}
	if got := response.Data[0].Containers; len(got) != 2 || got[0] != "app" || got[1] != "metrics" {
		t.Fatalf("expected Pod containers, got %#v", got)
	}
}
