package applicationapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	crypto "github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/service/application"
	"github.com/cylism/cylism-manager/internal/service/auth"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestIntegrationConfigFilesAreGenericScopedEncryptedAndVersioned(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	app := createApplicationForReleaseRuntimeTest(t, s)
	if err := app.SetCapabilities([]string{"file-editor"}); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateApplication(app); err != nil {
		t.Fatal(err)
	}
	key := []byte("01234567890123456789012345678901")
	values, _ := json.Marshal(map[string]string{"service.ini": "opaque-secret-v1", "tls.key": "unrelated-private-key", "disabled.txt": "disabled-value"})
	encrypted, err := crypto.Encrypt(key, string(values))
	if err != nil {
		t.Fatal(err)
	}
	spec := application.ReleaseSpec{Image: "docker.io/example/app", ContainerPort: 8080, Replicas: 1,
		Resources: application.ResourceSpec{RequestsCPU: "10m", RequestsMemory: "16Mi", LimitsCPU: "100m", LimitsMemory: "64Mi"},
		Config:    map[string]string{"settings.txt": "original"}, ConfigManagedKeys: []string{"settings.txt"},
		Secrets: map[string]string{"service.ini": "", "tls.key": "", "disabled.txt": ""}, SecretManagedKeys: []string{"service.ini"}, SecretsDisabled: []string{"disabled.txt"},
		FileMounts: []application.FileMountSpec{{SourceType: application.FileMountSourceApplicationSecret, Key: "service.ini", MountPath: "/etc/app/service.ini"}, {SourceType: application.FileMountSourceApplicationSecret, Key: "tls.key", MountPath: "/etc/app/tls.key"}, {SourceType: application.FileMountSourceApplicationConfig, Key: "settings.txt", MountPath: "/etc/config/settings.txt"}},
		Service:    application.ServiceSpec{Port: 8080, TargetPort: 8080, Protocol: application.ServiceProtocolTCP}}
	raw, _ := json.Marshal(spec)
	template := &model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "default", Enabled: true, Spec: string(raw), EncryptedSecrets: encrypted}
	if err = s.CreateApplicationDeploymentTemplate(template, true); err != nil {
		t.Fatal(err)
	}
	h := newTestApplicationHandler(s, key, nil).WithAudit(s)
	claims := &auth.DelegationClaims{UserID: 1, ProjectID: app.ProjectID, EnvironmentIDs: []uint{app.EnvironmentID}, ApplicationIDs: []uint{app.ID}, Capability: "file-editor", Actions: []string{"application:read", "secret:read", "secret:write", "configmap:read", "configmap:write"}}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("delegation", claims); c.Set("user_id", uint(1)); c.Next() })
	base := fmt.Sprintf("/api/integrations/applications/%d/config-files", app.ID)
	r.GET("/api/integrations/applications/:id/config-files", h.IntegrationListConfigFiles)
	r.GET("/api/integrations/applications/:id/config-files/:source/:key", h.IntegrationGetConfigFile)
	r.PUT("/api/integrations/applications/:id/config-files/:source/:key", h.IntegrationReplaceConfigFile)
	r.GET("/api/integrations/applications/:id/network", h.IntegrationApplicationNetwork)
	request := func(method, path string, body any) int {
		response := serve(r, newJSONRequest(method, path, body))
		return response.Code
	}
	listed := serve(r, newJSONRequest("GET", base, nil))
	if listed.Code != 200 || strings.Contains(listed.Body.String(), "opaque-secret") || strings.Contains(listed.Body.String(), "tls.key") || !strings.Contains(listed.Body.String(), "service.ini") {
		t.Fatalf("invalid file list: %s", listed.Body.String())
	}
	got := serve(r, newJSONRequest("GET", base+"/secret/service.ini", nil))
	if got.Code != 200 || !strings.Contains(got.Body.String(), "opaque-secret-v1") || strings.Contains(got.Body.String(), "unrelated-private-key") || got.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("invalid read: %d %s", got.Code, got.Body.String())
	}
	events, total, err := s.ListAuditLogsFiltered(model.AuditLogFilter{Action: "application.config_file.read", Limit: 20})
	if err != nil || total != 1 || strings.Contains(events[0].Detail, "opaque-secret") {
		t.Fatal("missing or unsafe file audit")
	}
	for _, test := range []struct {
		name   string
		change func()
	}{
		{"project", func() { claims.ProjectID++ }}, {"environment", func() { claims.EnvironmentIDs = []uint{app.EnvironmentID + 1} }}, {"application", func() { claims.ApplicationIDs = []uint{app.ID + 1} }}, {"capability", func() { claims.Capability = "other" }}, {"action", func() { claims.Actions = []string{"application:read"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			old := *claims
			defer func() { *claims = old }()
			test.change()
			if code := request("GET", base+"/secret/service.ini", nil); code != 403 {
				t.Fatalf("scope not enforced: %d", code)
			}
		})
	}
	h.WithAudit(nil)
	if code := request("GET", base+"/secret/service.ini", nil); code == 200 {
		t.Fatal("read bypassed unavailable audit")
	}
	h.WithAudit(s)
	for _, path := range []string{"/secret/tls.key", "/secret/disabled.txt", "/secret/missing", "/configmap/service.ini"} {
		if code := request("GET", base+path, nil); code != 404 {
			t.Fatalf("exposed unmanaged file %s: %d", path, code)
		}
	}
	oldActions := claims.Actions
	claims.Actions = []string{"application:read", "configmap:read", "configmap:write"}
	deniedList := serve(r, newJSONRequest("GET", base, nil))
	if strings.Contains(deniedList.Body.String(), "service.ini") {
		t.Fatal("Secret metadata exposed without secret:read")
	}
	if code := request("PUT", base+"/secret/service.ini", gin.H{"content": "unauthorized", "expected_revision": 1}); code != 403 {
		t.Fatal("Secret write allowed without independent action")
	}
	claims.Actions = oldActions
	put := func(content any, revision uint, restart bool) int {
		return request("PUT", base+"/secret/service.ini", gin.H{"content": content, "expected_revision": revision, "restart": restart})
	}
	if put(nil, 1, false) != 400 || put("changed", 1, true) != 403 || put(strings.Repeat("x", (4<<20)+1), 1, false) != 413 {
		t.Fatal("invalid mutation accepted")
	}
	if code := put("opaque-secret-v2", 1, false); code != 200 {
		t.Fatalf("write failed: %d", code)
	}
	if code := put("stale", 1, false); code != 409 {
		t.Fatalf("stale write accepted: %d", code)
	}
	stored, _ := s.GetDefaultApplicationDeploymentTemplate(app.ID)
	decrypted, err := h.decryptTemplateSecrets(stored)
	if err != nil || decrypted["service.ini"] != "opaque-secret-v2" || decrypted["tls.key"] != "unrelated-private-key" || decrypted["disabled.txt"] != "disabled-value" || strings.Contains(stored.Spec, "opaque-secret-v2") {
		t.Fatal("Secret preservation or encryption failed")
	}
	if code := request("PUT", base+"/configmap/settings.txt", gin.H{"content": "", "expected_revision": 2}); code != 200 {
		t.Fatalf("empty ConfigMap write failed: %d", code)
	}
	if code := put("", 3, false); code == 200 {
		t.Fatal("blank Secret write silently accepted")
	}
	network := serve(r, newJSONRequest("GET", fmt.Sprintf("/api/integrations/applications/%d/network", app.ID), nil))
	if network.Code != http.StatusOK || !strings.Contains(network.Body.String(), "public_host") {
		t.Fatalf("generic network unavailable: %s", network.Body.String())
	}
}
