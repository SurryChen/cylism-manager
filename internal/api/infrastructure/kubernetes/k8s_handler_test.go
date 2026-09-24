package kubernetes

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

type resourceReferenceFake struct{ references []model.ResourceReference }

func (f resourceReferenceFake) ListResourceReferences(namespace, sourceType, sourceName string) ([]model.ResourceReference, error) {
	return f.references, nil
}

func setupK8sTestRouter(clients ...*k8sclient.Client) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var client *k8sclient.Client
	if len(clients) > 0 {
		client = clients[0]
	}
	h := NewK8sHandlerWithAdapter(NewK8sResourceAdapter(client), nil, nil, nil)
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

func TestDashboardUsesLightweightClusterLists(t *testing.T) {
	replicas := int32(2)
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: map[string]string{"node-role.kubernetes.io/control-plane": ""}},
			Status: corev1.NodeStatus{
				NodeInfo:   corev1.NodeSystemInfo{KubeletVersion: "v1.32.1+k3s1"},
				Capacity:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("4"), corev1.ResourceMemory: resource.MustParse("8Gi")},
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			},
		},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "ready", Namespace: "default"}, Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: "default"}, Status: corev1.PodStatus{Phase: corev1.PodPending}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "ready", Namespace: "default"}, Spec: appsv1.DeploymentSpec{Replicas: &replicas}, Status: appsv1.DeploymentStatus{ReadyReplicas: 2}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: "default"}, Spec: appsv1.DeploymentSpec{Replicas: &replicas}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "service-a", Namespace: "default"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "service-b", Namespace: "default"}},
	)}

	response := serve(setupK8sTestRouter(client), httptest.NewRequest(http.MethodGet, "/api/k8s/dashboard", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}

	var payload struct {
		Data struct {
			NodesTotal        int    `json:"nodes_total"`
			NodesReady        int    `json:"nodes_ready"`
			NodesNotReady     int    `json:"nodes_not_ready"`
			ControlPlaneNodes int    `json:"control_plane_nodes"`
			WorkerNodes       int    `json:"worker_nodes"`
			CPUCoresTotal     int64  `json:"cpu_cores_total"`
			MemoryMBTotal     int64  `json:"memory_mb_total"`
			PodsTotal         int    `json:"pods_total"`
			PodsReady         int    `json:"pods_ready"`
			DeploymentsTotal  int    `json:"deployments_total"`
			DeploymentsReady  int    `json:"deployments_ready"`
			ServicesTotal     int    `json:"services_total"`
			Namespaces        int    `json:"namespaces"`
			Version           string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if got := payload.Data; got.NodesTotal != 1 || got.NodesReady != 1 || got.NodesNotReady != 0 || got.ControlPlaneNodes != 1 || got.WorkerNodes != 0 || got.CPUCoresTotal != 4 || got.MemoryMBTotal != 8192 || got.PodsTotal != 2 || got.PodsReady != 1 || got.DeploymentsTotal != 2 || got.DeploymentsReady != 1 || got.ServicesTotal != 2 || got.Namespaces != 1 || got.Version != "v1.32.1+k3s1" {
		t.Fatalf("unexpected dashboard data: %#v", got)
	}

	for _, action := range client.Clientset.(*k8sfake.Clientset).Actions() {
		if action.GetResource().Resource == "endpointslices" || action.GetResource().Resource == "endpoints" {
			t.Fatalf("dashboard must not query service endpoints: %#v", action)
		}
	}
}

func TestLightweightServiceListSkipsEndpointLookups(t *testing.T) {
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"}, Spec: corev1.ServiceSpec{ClusterIP: "10.43.0.10"}},
	)}

	response := serve(setupK8sTestRouter(client), httptest.NewRequest(http.MethodGet, "/api/k8s/services?endpoint_count=false", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "endpoint_count") {
		t.Fatalf("lightweight response must not include endpoint_count: %s", response.Body.String())
	}
	for _, action := range client.Clientset.(*k8sfake.Clientset).Actions() {
		if action.GetResource().Resource == "endpointslices" || action.GetResource().Resource == "endpoints" {
			t.Fatalf("lightweight service list must not query endpoints: %#v", action)
		}
	}
}

func TestDashboardUsesPodReadyAndCachesClusterSummary(t *testing.T) {
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}, Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.32.1"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-b"}, Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.33.0"}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "running-not-ready", Namespace: "default"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
	)}
	r := setupK8sTestRouter(client)
	first := serve(r, httptest.NewRequest(http.MethodGet, "/api/k8s/dashboard", nil))
	if first.Code != http.StatusOK || strings.Contains(first.Body.String(), `"pods_ready":1`) {
		t.Fatalf("running pod without Ready condition must not count: %s", first.Body.String())
	}
	var firstPayload struct {
		Data struct {
			Version    string `json:"version"`
			Consistent bool   `json:"version_consistent"`
		} `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstPayload); err != nil {
		t.Fatal(err)
	}
	if firstPayload.Data.Version != "多版本" || firstPayload.Data.Consistent {
		t.Fatalf("expected mixed node versions: %#v", firstPayload.Data)
	}
	actionsAfterFirst := len(client.Clientset.(*k8sfake.Clientset).Actions())
	second := serve(r, httptest.NewRequest(http.MethodGet, "/api/k8s/dashboard", nil))
	if second.Code != http.StatusOK {
		t.Fatalf("second request failed: %s", second.Body.String())
	}
	if got := len(client.Clientset.(*k8sfake.Clientset).Actions()); got != actionsAfterFirst {
		t.Fatalf("expected cache hit, actions grew from %d to %d", actionsAfterFirst, got)
	}
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

func TestK8sHandlerProtectsReferencedResourceThroughRepository(t *testing.T) {
	handler := NewK8sHandlerWithAdapter(nil, resourceReferenceFake{references: []model.ResourceReference{{ApplicationID: 1, Kind: "template", Name: "default"}}}, nil, nil)
	referenced, err := handler.resourceIsReferenced("default", "secret", "api-token")
	if err != nil || !referenced {
		t.Fatalf("expected reference protection, referenced=%v err=%v", referenced, err)
	}
}
