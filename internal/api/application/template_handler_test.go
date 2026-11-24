package applicationapi

import (
	"context"
	"encoding/json"
	"fmt"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	crypto "github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/service/application"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"net/http"
	"strings"
	"testing"
)

func TestTemplateSensitiveDeletionPreservesReferencesAndRollsBack(t *testing.T) {
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()}
	r, s := setupApplicationRouter(client)
	app := createApplicationForReleaseRuntimeTest(t, s)
	_, err := client.Clientset.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: app.Environment.Namespace},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("01234567890123456789012345678901")
	h := newTestApplicationHandler(s, key, NewKubernetesAdapter(client))
	r.PUT("/template-edits/:id/:templateID", func(c *gin.Context) {
		c.Set("user_id", uint(7))
		c.Set("username", "operator")
		h.UpdateDeploymentTemplate(c)
	})
	r.GET("/template-edits/:id/:templateID", func(c *gin.Context) {
		c.Set("user_id", uint(7))
		h.GetDeploymentTemplate(c)
	})
	r.POST("/template-edits/:id/preflight", h.PreflightRelease)
	const removed = "account-password-client-secret"
	const remaining = "account-client-secret"
	mount := func(key, path string) gin.H {
		return gin.H{"source_type": "application_secret", "key": key, "mount_path": path}
	}
	spec := gin.H{
		"image": "order-api", "container_port": 8080, "replicas": 1,
		"resources":           gin.H{"requests_cpu": "100m", "requests_memory": "128Mi", "limits_cpu": "500m", "limits_memory": "512Mi"},
		"service":             gin.H{"port": 80, "target_port": 8080},
		"secrets":             gin.H{removed: "fixture-delete", remaining: "fixture-keep", "encryption.key": "fixture-encryption", "MYSQL_DSN": "fixture-dsn"},
		"secret_file_keys":    []string{removed, remaining},
		"secret_managed_keys": []string{removed},
		"file_mounts":         []gin.H{mount(removed, "/a/key"), mount(removed, "/b/key"), mount(remaining, "/c/key")},
	}
	created := serve(r, newJSONRequest(http.MethodPost, fmt.Sprintf("/api/applications/%d/deployment-templates", app.ID), gin.H{"name": "default", "default_version": "v1", "enabled": true, "spec": spec}))
	if created.Code != http.StatusOK {
		t.Fatal(created.Body.String())
	}
	template, err := s.GetDefaultApplicationDeploymentTemplate(app.ID)
	if err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/template-edits/%d/%d", app.ID, template.ID)
	spec["secrets"] = gin.H{removed: "", remaining: "", "encryption.key": "", "MYSQL_DSN": ""}
	spec["file_mounts"] = []gin.H{mount(removed, "/b/key"), mount(remaining, "/c/key")}
	save := func(revision uint, deletes ...string) int {
		response := serve(r, newJSONRequest(http.MethodPut, path, gin.H{"name": "default", "default_version": "v1", "enabled": true, "revision": revision, "spec": spec, "delete_secret_keys": deletes}))
		return response.Code
	}
	if code := save(template.Revision, removed); code == http.StatusOK {
		t.Fatal("deleted a still-mounted value")
	}
	if code := save(template.Revision); code != http.StatusOK {
		t.Fatalf("saving remaining mount: %d", code)
	}
	live, err := s.GetDefaultApplicationDeploymentTemplate(app.ID)
	if err != nil {
		t.Fatal(err)
	}
	values, err := h.decryptTemplateSecrets(live)
	if err != nil || values[removed] != "fixture-delete" {
		t.Fatal("removing one of two mounts changed the saved value")
	}
	spec["file_mounts"] = []gin.H{mount(remaining, "/c/key")}
	spec["secret_file_keys"] = []string{remaining}
	if code := save(live.Revision, removed); code != http.StatusBadRequest {
		t.Fatalf("deleting a value with an environment reference: %d", code)
	}
	spec["secret_file_keys"] = []string{removed, remaining}
	spec["secret_managed_keys"] = []string{}
	if code := save(live.Revision); code != http.StatusOK {
		t.Fatalf("unmounting the last reference while keeping its value: %d", code)
	}
	kept, err := s.GetDefaultApplicationDeploymentTemplate(app.ID)
	if err != nil {
		t.Fatal(err)
	}
	keptValues, err := h.decryptTemplateSecrets(kept)
	if err != nil || keptValues[removed] != "fixture-delete" {
		t.Fatal("unmounting with retain removed the saved value")
	}
	keptSpec, err := h.releaseWorkflow().PrepareTemplateSpec(app, kept, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := keptSpec.Secrets[removed]; exists {
		t.Fatal("an unmounted retained value was included in deployment")
	}
	spec["file_mounts"] = []gin.H{mount(removed, "/b/key"), mount(remaining, "/c/key")}
	spec["secret_managed_keys"] = []string{removed}
	if code := save(kept.Revision); code != http.StatusOK {
		t.Fatalf("remounting the retained value: %d", code)
	}
	live, err = s.GetDefaultApplicationDeploymentTemplate(app.ID)
	if err != nil {
		t.Fatal(err)
	}
	values["legacy-unreferenced"] = "fixture-legacy"
	payload, _ := json.Marshal(values)
	ciphertext, err := crypto.Encrypt(key, string(payload))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DB().Model(live).Update("encrypted_secrets", ciphertext).Error; err != nil {
		t.Fatal(err)
	}
	readPath := path + "?include_unused_secret_keys=true"
	read := serve(r, newJSONRequest(http.MethodGet, readPath, nil))
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"unused_secret_keys":["legacy-unreferenced"]`) || strings.Contains(read.Body.String(), "fixture-") {
		t.Fatal("cleanup must expose only unreferenced names")
	}
	ordinaryRead := serve(r, newJSONRequest(http.MethodGet, fmt.Sprintf("/api/applications/%d/deployment-templates/%d?include_unused_secret_keys=true", app.ID, template.ID), nil))
	if ordinaryRead.Code != http.StatusForbidden || strings.Contains(ordinaryRead.Body.String(), "legacy-unreferenced") {
		t.Fatal("cleanup metadata requires a platform manager")
	}
	before, _ := s.GetDefaultApplicationDeploymentTemplate(app.ID)
	prepared, err := h.releaseWorkflow().PrepareTemplateSpec(app, before, "")
	if err != nil || prepared.Secrets["legacy-unreferenced"] != "" {
		t.Fatal("legacy orphan leaked into release preparation")
	}
	spec["file_mounts"] = []gin.H{mount(remaining, "/c/key")}
	spec["secret_file_keys"] = []string{remaining}
	spec["secret_managed_keys"] = []string{}
	delete(spec["secrets"].(gin.H), removed)
	if code := save(template.Revision, removed); code != http.StatusConflict {
		t.Fatalf("stale edit: %d", code)
	}
	for _, failure := range []struct{ table, operation string }{{"application_deployment_templates", "UPDATE"}, {"application_managed_files", "UPDATE"}, {"audit_logs", "INSERT"}} {
		if err := s.DB().Exec(fmt.Sprintf("CREATE TRIGGER fail_edit BEFORE %s ON %s BEGIN SELECT RAISE(ABORT, 'fixture write failure'); END", failure.operation, failure.table)).Error; err != nil {
			t.Fatal(err)
		}
		if code := save(live.Revision, removed, "legacy-unreferenced"); code != http.StatusInternalServerError {
			t.Fatalf("%s failure: %d", failure.table, code)
		}
		after, err := s.GetDefaultApplicationDeploymentTemplate(app.ID)
		if err != nil || after.Revision != before.Revision || after.Spec != before.Spec || after.EncryptedSecrets != before.EncryptedSecrets {
			t.Fatalf("%s failure changed the template", failure.table)
		}
		files, err := s.ListApplicationManagedFiles(app.ID)
		if err != nil || len(files) != 1 || files[0].Key != removed {
			t.Fatalf("%s failure changed managed files", failure.table)
		}
		if err := s.DB().Exec("DROP TRIGGER fail_edit").Error; err != nil {
			t.Fatal(err)
		}
	}
	if code := save(live.Revision, removed, "legacy-unreferenced"); code != http.StatusOK {
		t.Fatalf("deletion: %d", code)
	}
	final, err := s.GetDefaultApplicationDeploymentTemplate(app.ID)
	if err != nil {
		t.Fatal(err)
	}
	values, err = h.decryptTemplateSecrets(final)
	if err != nil || len(values) != 3 || values[remaining] != "fixture-keep" || values["encryption.key"] != "fixture-encryption" || values["MYSQL_DSN"] != "fixture-dsn" {
		t.Fatal("deletion did not preserve exactly the active values")
	}
	read = serve(r, newJSONRequest(http.MethodGet, path, nil))
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), remaining) || strings.Contains(read.Body.String(), removed) || strings.Contains(read.Body.String(), "legacy-unreferenced") {
		t.Fatal("deleted keys still appear when reopening the template")
	}
	cleanup := serve(r, newJSONRequest(http.MethodGet, readPath, nil))
	if cleanup.Code != http.StatusOK || !strings.Contains(cleanup.Body.String(), `"unused_secret_keys":[]`) {
		t.Fatal("deleted keys still appear in the cleanup list")
	}
	preflight := serve(r, newJSONRequest(http.MethodPost, fmt.Sprintf("/template-edits/%d/preflight", app.ID), gin.H{"template_id": template.ID, "version": "v1"}))
	result := preflight.Body.String()
	if preflight.Code != http.StatusOK || !strings.Contains(result, `"ready":true`) || !strings.Contains(result, remaining) ||
		strings.Contains(result, removed) || strings.Contains(result, "legacy-unreferenced") || strings.Contains(result, "fixture-") {
		t.Fatalf("preflight did not report only active names: %s", result)
	}
	prepared, err = h.releaseWorkflow().PrepareTemplateSpec(app, final, "")
	if err != nil {
		t.Fatal(err)
	}
	resources, err := application.RenderResources(applicationContextFor(app), prepared)
	if err != nil || len(resources.Secret.StringData) != 3 || resources.Secret.StringData[removed] != "" {
		t.Fatal("deleted keys remained in the rendered release")
	}
	files, err := s.ListApplicationManagedFiles(app.ID)
	if err != nil || len(files) != 0 {
		t.Fatal("deleted key retained an enabled managed file binding")
	}
	logs, total, err := s.ListAuditLogs("application", "application.template.delete-sensitive-keys", "", 10, 0)
	if err != nil || total != 1 || logs[0].UserID != 7 || !strings.Contains(logs[0].Detail, removed) || !strings.Contains(logs[0].Detail, "legacy-unreferenced") || strings.Contains(logs[0].Detail, "fixture-") {
		t.Fatal("audit must record the operator and deleted names only")
	}
}

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
