package applicationapi

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
	clientset := k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "commerce-prod"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}})
	r, s := setupApplicationRouter(&k8sclient.Client{Clientset: clientset})
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
	canary := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/deployment-templates", gin.H{"name": "灰度配置", "enabled": true, "default_version": "1.2.3", "spec": spec}))
	if canary.Code != http.StatusOK {
		t.Fatalf("create second template status = %d: %s", canary.Code, canary.Body.String())
	}
	if !strings.Contains(canary.Body.String(), "\"default_version\":\"1.2.3\"") {
		t.Fatalf("default version missing from template response: %s", canary.Body.String())
	}
	canaryID := responseID(t, canary.Body.Bytes())
	setDefault := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/deployment-templates/2/default", nil))
	if setDefault.Code != http.StatusOK {
		t.Fatalf("set default template status = %d: %s", setDefault.Code, setDefault.Body.String())
	}

	missingVersion := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/releases", gin.H{"template_id": 1}))
	if missingVersion.Code == http.StatusOK || !strings.Contains(missingVersion.Body.String(), "请填写版本号") {
		t.Fatalf("missing version should fail: %d %s", missingVersion.Code, missingVersion.Body.String())
	}
	release := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/releases", gin.H{"template_id": canaryID, "version": "1.2.4"}))
	if release.Code != http.StatusOK || !strings.Contains(release.Body.String(), "order-api:1.2.4") || !strings.Contains(release.Body.String(), "\"template_id\":2") {
		t.Fatalf("unexpected version release: %d %s", release.Code, release.Body.String())
	}
}

func TestFirstDisabledDeploymentTemplateRemainsDisabledAndNotDefault(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "commerce-prod"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}})
	r, s := setupApplicationRouter(&k8sclient.Client{Clientset: clientset})
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
		"health":    gin.H{"readiness_enabled": false},
		"service":   gin.H{"port": 80, "target_port": 8080},
	}
	created := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/deployment-templates", gin.H{"name": "待切换", "enabled": false, "spec": spec}))
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), `"enabled":false`) || !strings.Contains(created.Body.String(), `"is_default":false`) {
		t.Fatalf("disabled template was enabled or made default: %d %s", created.Code, created.Body.String())
	}
	app, err := s.GetApplication(1)
	if err != nil || app.DefaultDeploymentTemplateID != nil {
		t.Fatalf("disabled template became default: app=%+v err=%v", app, err)
	}
	blocked := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/releases", gin.H{"template_id": responseID(t, created.Body.Bytes()), "version": "1.0.0"}))
	if blocked.Code == http.StatusOK {
		t.Fatalf("disabled template was released: %s", blocked.Body.String())
	}
}

func TestApplicationHandlerUsesTemplateDefaultVersionWhenReleaseVersionIsEmpty(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "commerce-prod"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}})
	r, s := setupApplicationRouter(&k8sclient.Client{Clientset: clientset})
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
	template := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/deployment-templates", gin.H{"name": "标准上线", "enabled": true, "default_version": "2.3.4", "spec": spec}))
	if template.Code != http.StatusOK {
		t.Fatalf("create template: %d %s", template.Code, template.Body.String())
	}
	release := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/releases", gin.H{"template_id": responseID(t, template.Body.Bytes())}))
	if release.Code != http.StatusOK || !strings.Contains(release.Body.String(), "order-api:2.3.4") {
		t.Fatalf("unexpected default version release: %d %s", release.Code, release.Body.String())
	}
}

func TestTemplateInlineSensitiveFilesSurviveBlankEdit(t *testing.T) {
	r, s := setupApplicationRouter()
	createApplicationForReleaseRuntimeTest(t, s)
	spec := gin.H{
		"image": "order-api", "container_port": 8080, "replicas": 1,
		"resources": gin.H{"requests_cpu": "100m", "requests_memory": "128Mi", "limits_cpu": "500m", "limits_memory": "512Mi"},
		"service":   gin.H{"port": 80, "target_port": 8080},
		"config":    gin.H{"config.yaml": "port: 8080"}, "config_file_keys": []string{"config.yaml"},
		"secrets": gin.H{"env": "MYSQL_DSN=private"}, "secret_file_keys": []string{"env"},
		"file_mounts": []gin.H{{"source_type": "application_secret", "key": "env", "mount_path": "/app/secrets/env"}},
	}
	created := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/deployment-templates", gin.H{"name": "default", "enabled": true, "spec": spec}))
	if created.Code != http.StatusOK || strings.Contains(created.Body.String(), "MYSQL_DSN=private") || !strings.Contains(created.Body.String(), `"secret_file_keys":["env"]`) {
		t.Fatalf("inline file save failed or leaked contents: %d %s", created.Code, created.Body.String())
	}
	spec["secrets"] = gin.H{"env": ""}
	updated := serve(r, newJSONRequest(http.MethodPut, "/api/applications/1/deployment-templates/1", gin.H{"name": "default", "enabled": true, "revision": 1, "spec": spec}))
	if updated.Code != http.StatusOK {
		t.Fatalf("blank edit failed: %d %s", updated.Code, updated.Body.String())
	}
	template, err := s.GetApplicationDeploymentTemplate(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(template.Spec, "MYSQL_DSN=private") || !strings.Contains(template.Spec, `"config_file_keys":["config.yaml"]`) {
		t.Fatal("stored snapshot lost metadata or leaked Secret")
	}
	h := newTestApplicationHandler(s, []byte("01234567890123456789012345678901"), nil)
	values, err := h.decryptTemplateSecrets(template)
	if err != nil || values["env"] != "MYSQL_DSN=private" {
		t.Fatalf("blank edit did not retain encrypted content: %v", err)
	}
	spec["secrets"] = gin.H{"env": "", "new-file": ""}
	invalid := serve(r, newJSONRequest(http.MethodPut, "/api/applications/1/deployment-templates/1", gin.H{"name": "default", "enabled": true, "revision": 2, "spec": spec}))
	if invalid.Code == http.StatusOK || !strings.Contains(invalid.Body.String(), "new-file") {
		t.Fatal("new sensitive file must require content")
	}
}
