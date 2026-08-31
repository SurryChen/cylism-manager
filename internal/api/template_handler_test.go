package api

import (
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"net/http"
	"strings"
	"testing"
)

func TestApplicationHandlerReleasesFromSelectedDeploymentTemplateByVersion(t *testing.T) {
	r, s := setupApplicationRouter()
	if err := s.CreateProject(&model.Project{Name: "commerce", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateEnvironment(&model.Environment{ProjectID: 1, Name: "production", Namespace: "commerce-prod"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateApplication(&model.Application{ProjectID: 1, EnvironmentID: 1, Name: "order-api", WorkloadKind: "deployment", CreatedBy: 1}); err != nil {
		t.Fatal(err)
	}
	spec := gin.H{
		"image": "order-api", "container_port": 8080, "replicas": 1,
		"resources": gin.H{"requests_cpu": "100m", "requests_memory": "128Mi", "limits_cpu": "500m", "limits_memory": "512Mi"},
		"health":    gin.H{"readiness_enabled": false, "readiness_type": "http", "readiness_path": "/healthz", "liveness_enabled": false, "liveness_type": "http", "liveness_path": "/healthz"},
		"service":   gin.H{"port": 80, "target_port": 8080},
	}
	template := gin.H{"name": "标准上线", "enabled": true, "spec": spec}
	save := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/deployment-templates", template))
	if save.Code != http.StatusOK {
		t.Fatalf("save template status = %d: %s", save.Code, save.Body.String())
	}
	loaded := serve(r, newJSONRequest(http.MethodGet, "/api/applications/1/deployment-templates", nil))
	if loaded.Code != http.StatusOK || !strings.Contains(loaded.Body.String(), "标准上线") || !strings.Contains(loaded.Body.String(), "\"revision\":1") {
		t.Fatalf("unexpected template response: %s", loaded.Body.String())
	}
	canary := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/deployment-templates", gin.H{"name": "灰度配置", "enabled": true, "spec": spec}))
	if canary.Code != http.StatusOK {
		t.Fatalf("create second template status = %d: %s", canary.Code, canary.Body.String())
	}
	canaryID := responseID(t, canary.Body.Bytes())
	setDefault := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/deployment-templates/2/default", nil))
	if setDefault.Code != http.StatusOK {
		t.Fatalf("set default template status = %d: %s", setDefault.Code, setDefault.Body.String())
	}

	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "commerce-prod"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}})}
	defer func() { K8s = originalK8s }()
	release := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/releases", gin.H{"template_id": canaryID, "version": "1.2.3"}))
	if release.Code != http.StatusOK || !strings.Contains(release.Body.String(), "order-api:1.2.3") || !strings.Contains(release.Body.String(), "\"template_id\":2") {
		t.Fatalf("unexpected version release: %d %s", release.Code, release.Body.String())
	}
}
