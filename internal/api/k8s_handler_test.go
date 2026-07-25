package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupK8sTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewK8sHandler()
	g := r.Group("/api/k8s")
	{
		g.GET("/dashboard", h.Dashboard)
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
