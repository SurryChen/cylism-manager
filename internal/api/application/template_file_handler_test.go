package applicationapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

func TestTemplateFileInspectionIsScopedAuditedAndUncached(t *testing.T) {
	r, s := setupApplicationRouter()
	createApplicationForReleaseRuntimeTest(t, s)
	key := []byte("01234567890123456789012345678901")
	h := newTestApplicationHandler(s, key, nil).WithAudit(s)
	r.POST("/inspect/:id/:templateID", func(c *gin.Context) { c.Set("user_id", uint(7)); h.InspectDeploymentTemplateFile(c) })
	r.POST("/anonymous/:id/:templateID", h.InspectDeploymentTemplateFile)
	r.POST("/delegated/:id/:templateID", func(c *gin.Context) {
		c.Set("user_id", uint(7))
		c.Set("delegation", true)
		h.InspectDeploymentTemplateFile(c)
	})
	content := "# config\nMYSQL_HOST=mysql.svc\nMYSQL_PASSWORD=never-expose-this\n"
	spec := templateFileTestSpec(content)
	created := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/deployment-templates", gin.H{"name": "default", "enabled": true, "spec": spec}))
	if created.Code != http.StatusOK {
		t.Fatal(created.Body.String())
	}
	body := gin.H{"key": "env", "revision": 1, "mode": "fields"}
	inspected := serve(r, newJSONRequest(http.MethodPost, "/inspect/1/1", body))
	if inspected.Code != http.StatusOK || strings.Contains(inspected.Body.String(), "mysql.svc") || strings.Contains(inspected.Body.String(), "never-expose-this") || inspected.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("inspection failed or leaked secret: %d", inspected.Code)
	}
	body["mode"], body["line"], body["name"] = "field", 2, "MYSQL_PASSWORD"
	selected := serve(r, newJSONRequest(http.MethodPost, "/inspect/1/1", body))
	if selected.Code != http.StatusOK || !strings.Contains(selected.Body.String(), "never-expose-this") || strings.Contains(selected.Body.String(), "mysql.svc") || strings.Contains(selected.Body.String(), "MYSQL_HOST") || selected.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("selected field reveal must return only that assignment")
	}
	for _, identity := range []gin.H{{"name": "MYSQL_HOST", "line": 2}, {"name": "MYSQL_HOST"}, {"name": "MYSQL_HOST", "line": -1}} {
		invalid := gin.H{"key": "env", "revision": 1, "mode": "field", "name": identity["name"], "line": identity["line"]}
		response := serve(r, newJSONRequest(http.MethodPost, "/inspect/1/1", invalid))
		if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "mysql.svc") || strings.Contains(response.Body.String(), "never-expose-this") {
			t.Fatal("invalid field identity must not disclose contents")
		}
	}
	body["mode"] = "content"
	revealed := serve(r, newJSONRequest(http.MethodPost, "/inspect/1/1", body))
	if revealed.Code != http.StatusOK || !strings.Contains(revealed.Body.String(), "never-expose-this") {
		t.Fatal("explicit reveal failed")
	}
	logs, count, err := s.ListAuditLogsFiltered(model.AuditLogFilter{Action: "application.template_file.read", Limit: 10})
	if err != nil || count != 3 {
		t.Fatalf("file reads not audited: count=%d err=%v", count, err)
	}
	encoded, _ := json.Marshal(logs)
	if strings.Contains(string(encoded), "never-expose-this") || strings.Contains(string(encoded), "mysql.svc") {
		t.Fatal("audit must never include file values")
	}
	for path, status := range map[string]int{"/anonymous/1/1": 401, "/delegated/1/1": 403, "/inspect/2/1": 404} {
		response := serve(r, newJSONRequest(http.MethodPost, path, body))
		if response.Code != status || strings.Contains(response.Body.String(), "never-expose-this") {
			t.Fatalf("scope guard failed for %s: %d", path, response.Code)
		}
	}
	body["key"] = "not-mounted"
	if response := serve(r, newJSONRequest(http.MethodPost, "/inspect/1/1", body)); response.Code != 404 {
		t.Fatal("unmounted key should not be readable")
	}
	body["key"], body["revision"] = "env", 99
	if response := serve(r, newJSONRequest(http.MethodPost, "/inspect/1/1", body)); response.Code != 409 {
		t.Fatal("stale file read should conflict")
	}
	h.audit = failingTemplateFileAudit{}
	body["revision"], body["mode"] = 1, "field"
	response := serve(r, newJSONRequest(http.MethodPost, "/inspect/1/1", body))
	if response.Code != 500 || strings.Contains(response.Body.String(), "never-expose-this") {
		t.Fatal("audit failure must prevent disclosure")
	}
}

type failingTemplateFileAudit struct{}

func (failingTemplateFileAudit) CreateAuditLog(*model.AuditLog) error {
	return errors.New("audit unavailable")
}

func templateFileTestSpec(content string) gin.H {
	return gin.H{"image": "order-api", "container_port": 8080, "replicas": 1,
		"resources": gin.H{"requests_cpu": "100m", "requests_memory": "128Mi", "limits_cpu": "500m", "limits_memory": "512Mi"},
		"service":   gin.H{"port": 80, "target_port": 8080}, "secrets": gin.H{"env": content}, "secret_file_keys": []string{"env"},
		"file_mounts": []gin.H{{"source_type": "application_secret", "key": "env", "mount_path": "/app/secrets/env"}}}
}

func TestTemplateFilePartialUpdateKeepsHiddenValuesAndChecksRevision(t *testing.T) {
	r, s := setupApplicationRouter()
	createApplicationForReleaseRuntimeTest(t, s)
	original := "# keep\nMYSQL_HOST=old.svc\nMYSQL_PASSWORD='hidden#value'\n"
	spec := templateFileTestSpec(original)
	created := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/deployment-templates", gin.H{"name": "default", "enabled": true, "spec": spec}))
	if created.Code != 200 {
		t.Fatal(created.Body.String())
	}
	spec["secrets"] = gin.H{"env": ""}
	request := gin.H{"name": "default", "enabled": true, "revision": 1, "spec": spec, "secret_file_edits": []gin.H{{"key": "env", "changes": []gin.H{{"line": 1, "name": "MYSQL_HOST", "value": "new.svc"}}}}}
	updated := serve(r, newJSONRequest(http.MethodPut, "/api/applications/1/deployment-templates/1", request))
	if updated.Code != 200 || strings.Contains(updated.Body.String(), "hidden#value") || strings.Contains(updated.Body.String(), "new.svc") {
		t.Fatalf("partial save failed or leaked content: %d", updated.Code)
	}
	stored, _ := s.GetApplicationDeploymentTemplate(1, 1)
	h := newTestApplicationHandler(s, []byte("01234567890123456789012345678901"), nil)
	values, err := h.decryptTemplateSecrets(stored)
	if err != nil || values["env"] != strings.Replace(original, "old.svc", "new.svc", 1) || strings.Contains(stored.EncryptedSecrets, "hidden#value") || strings.Contains(stored.Spec, "hidden#value") {
		t.Fatal("partial update did not preserve encrypted original content")
	}
	if response := serve(r, newJSONRequest(http.MethodPut, "/api/applications/1/deployment-templates/1", request)); response.Code != 409 {
		t.Fatal("stale partial update must conflict")
	}
	request["revision"] = 0
	if response := serve(r, newJSONRequest(http.MethodPut, "/api/applications/1/deployment-templates/1", request)); response.Code != 400 {
		t.Fatal("revision is required for partial update")
	}
	request["revision"] = 2
	spec["secrets"] = gin.H{"env": "RAW=conflicting"}
	if response := serve(r, newJSONRequest(http.MethodPut, "/api/applications/1/deployment-templates/1", request)); response.Code != 400 {
		t.Fatal("raw and partial replacement cannot be mixed")
	}
}
