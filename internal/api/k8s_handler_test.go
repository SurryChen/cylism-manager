package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func setupK8sTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewK8sHandler()
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
		g.GET("/configmaps/:namespace/:name", h.GetConfigMap)
		g.GET("/secrets", h.ListSecrets)
		g.GET("/secrets/:namespace/:name", h.GetSecret)
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

func withNoK8s(t *testing.T, fn func()) {
	t.Helper()
	orig := K8s
	K8s = nil
	defer func() { K8s = orig }()
	fn()
}

// noK8sGet/Post 等辅助：无 K8s 时，所有 handler 必须返回 200
func doNoK8s(t *testing.T, method, path string, body io.Reader) {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	r := setupK8sTestRouter()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("%s %s: expected 200, got %d. body=%s", method, path, w.Code, w.Body.String())
	}
}

func TestNoK8s_AllEndpoints(t *testing.T) {
	withNoK8s(t, func() {
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
		doNoK8s(t, "GET", "/api/k8s/configmaps/default/app", nil)
		doNoK8s(t, "GET", "/api/k8s/secrets", nil)
		doNoK8s(t, "GET", "/api/k8s/secrets/default/db-pass", nil)
		doNoK8s(t, "GET", "/api/k8s/ingresses", nil)
		doNoK8s(t, "GET", "/api/k8s/ingresses/default/web", nil)
		doNoK8s(t, "POST", "/api/k8s/ingresses", jsonBody(map[string]string{"namespace": "default", "name": "test", "host": "example.com", "service_name": "web", "service_port": "http"}))
		doNoK8s(t, "DELETE", "/api/k8s/ingresses/default/web", nil)
		doNoK8s(t, "GET", "/api/k8s/ingress-controller", nil)
	})
}

func TestListNamespaceNamesDoesNotLoadResourceSummaries(t *testing.T) {
	original := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "monitoring"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}},
	)}
	defer func() { K8s = original }()

	response := serve(setupK8sTestRouter(), httptest.NewRequest(http.MethodGet, "/api/k8s/namespace-names", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"monitoring"`) {
		t.Fatalf("unexpected namespace name response: %s", response.Body.String())
	}
	for _, action := range K8s.Clientset.(*k8sfake.Clientset).Actions() {
		if action.GetResource().Resource != "namespaces" {
			t.Fatalf("unexpected resource lookup: %#v", action)
		}
	}
}

func TestListPodsIncludesContainers(t *testing.T) {
	original := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api-0", Namespace: "default"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}, {Name: "metrics"}}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	})}
	defer func() { K8s = original }()

	r := setupK8sTestRouter()
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

func TestPersistentVolumeClaimsAreScopedToEnvironment(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateProject(&model.Project{Name: "knowledge", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateEnvironment(&model.Environment{ProjectID: 1, Name: "production", Namespace: "project-knowledge-prod"}); err != nil {
		t.Fatal(err)
	}
	original := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "project-knowledge-prod"}})}
	defer func() { K8s = original }()

	r := gin.New()
	h := NewK8sHandler(st)
	g := r.Group("/api/k8s")
	g.POST("/persistent-volume-claims", h.CreatePersistentVolumeClaim)
	g.GET("/persistent-volume-claims", h.ListPersistentVolumeClaims)

	create := serve(r, newJSONRequest(http.MethodPost, "/api/k8s/persistent-volume-claims", map[string]any{"environment_id": 1, "name": "karakeep-data", "storage": "5Gi"}))
	if create.Code != http.StatusOK || !strings.Contains(create.Body.String(), "karakeep-data") {
		t.Fatalf("unexpected create response: %d %s", create.Code, create.Body.String())
	}
	listed := serve(r, httptest.NewRequest(http.MethodGet, "/api/k8s/persistent-volume-claims?environment_id=1", nil))
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), "karakeep-data") {
		t.Fatalf("unexpected list response: %d %s", listed.Code, listed.Body.String())
	}
}

func TestPersistentVolumeClaimsListClusterInventoryWithoutEnvironment(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateProject(&model.Project{Name: "knowledge", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateEnvironment(&model.Environment{ProjectID: 1, Name: "production", Namespace: "project-knowledge-prod"}); err != nil {
		t.Fatal(err)
	}
	original := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "manual-data", Namespace: "default"}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "app-data", Namespace: "project-knowledge-prod", Labels: map[string]string{k8sclient.ManagedByLabel: k8sclient.ManagedByValue, k8sclient.EnvironmentLabel: k8sclient.EnvironmentLabelValue(1)}}},
	)}
	defer func() { K8s = original }()

	r := gin.New()
	h := NewK8sHandler(st)
	r.GET("/api/k8s/persistent-volume-claims", h.ListPersistentVolumeClaims)
	response := serve(r, httptest.NewRequest(http.MethodGet, "/api/k8s/persistent-volume-claims", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "manual-data") || !strings.Contains(response.Body.String(), "project_name") {
		t.Fatalf("unexpected cluster PVC inventory: %d %s", response.Code, response.Body.String())
	}
}

func TestPersistentVolumeClaimCanBeCreatedInNamespaceWithoutEnvironment(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	original := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}})}
	defer func() { K8s = original }()

	r := gin.New()
	h := NewK8sHandler(st)
	r.POST("/api/k8s/persistent-volume-claims", h.CreatePersistentVolumeClaim)
	response := serve(r, newJSONRequest(http.MethodPost, "/api/k8s/persistent-volume-claims", gin.H{"namespace": "default", "name": "shared-data", "storage": "2Gi"}))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "shared-data") {
		t.Fatalf("unexpected independent PVC create response: %d %s", response.Code, response.Body.String())
	}
	claim, err := K8s.Clientset.CoreV1().PersistentVolumeClaims("default").Get(context.Background(), "shared-data", metav1.GetOptions{})
	if err != nil || claim.Labels[k8sclient.ManagedByLabel] != k8sclient.ManagedByValue || claim.Labels[k8sclient.EnvironmentLabel] != "" {
		t.Fatalf("expected platform-managed PVC without application environment, claim=%#v err=%v", claim, err)
	}
}

func TestDeleteHostDirectoryPVCImportBackupAcceptsEnvironmentIDFromBody(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateProject(&model.Project{Name: "knowledge", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateEnvironment(&model.Environment{ProjectID: 1, Name: "production", Namespace: "project-knowledge-prod"}); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	h := NewK8sHandler(st)
	r.DELETE("/api/k8s/persistent-volume-claims/:name/imports/:id/backup", h.DeleteHostDirectoryPVCImportBackup)
	response := serve(r, newJSONRequest(http.MethodDelete, "/api/k8s/persistent-volume-claims/karakeep-data/imports/3/backup", gin.H{"environment_id": 1}))
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "目录导入记录不存在") {
		t.Fatalf("expected body environment ID to be accepted, got %d %s", response.Code, response.Body.String())
	}
}
