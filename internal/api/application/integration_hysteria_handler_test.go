package applicationapi

import (
	"encoding/json"
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

func TestIntegrationHysteriaSecretUsersAreScopedAndVersioned(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	app := createApplicationForReleaseRuntimeTest(t, s)
	if err := app.SetCapabilities([]string{"hysteria2"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateApplication(app); err != nil {
		t.Fatal(err)
	}
	key := []byte("01234567890123456789012345678901")
	config := "listen: :8443\nauth:\n  type: userpass\n  userpass:\n    alice: alice-secret-password\ntrafficStats:\n  listen: :9999\n  secret: traffic-secret\n"
	values, _ := json.Marshal(map[string]string{"config.yaml": config, "tls.key": "private-key"})
	encrypted, err := crypto.Encrypt(key, string(values))
	if err != nil {
		t.Fatal(err)
	}
	spec := application.ReleaseSpec{
		Image: "ghcr.io/example/hysteria", ContainerPort: 8443, Replicas: 1,
		Resources:         application.ResourceSpec{RequestsCPU: "100m", RequestsMemory: "128Mi", LimitsCPU: "500m", LimitsMemory: "512Mi"},
		Secrets:           map[string]string{"config.yaml": "", "tls.key": ""},
		SecretManagedKeys: []string{"config.yaml"},
		FileMounts:        []application.FileMountSpec{{SourceType: application.FileMountSourceApplicationSecret, Key: "config.yaml", MountPath: "/etc/hysteria/config.yaml"}},
		Service:           application.ServiceSpec{Port: 8443, TargetPort: 8443, Protocol: application.ServiceProtocolUDP},
	}
	snapshot, _ := json.Marshal(spec)
	template := &model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "default", Enabled: true, Spec: string(snapshot), EncryptedSecrets: encrypted}
	if err := s.CreateApplicationDeploymentTemplate(template, true); err != nil {
		t.Fatal(err)
	}
	h := newTestApplicationHandler(s, key, nil)
	claims := &auth.DelegationClaims{UserID: 1, ProjectID: app.ProjectID, EnvironmentIDs: []uint{app.EnvironmentID}, ApplicationIDs: []uint{app.ID}, Capability: "hysteria2", Actions: []string{"hysteria:read", "hysteria:write"}}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("delegation", claims); c.Set("user_id", uint(1)); c.Next() })
	r.GET("/api/integrations/applications/:id/hysteria/config", h.IntegrationHysteriaConfig)
	r.GET("/api/integrations/applications/:id/hysteria/users/:username/credential", h.IntegrationHysteriaCredential)
	r.PUT("/api/integrations/applications/:id/hysteria/users/:username", h.IntegrationUpdateHysteriaUser)
	base := "/api/integrations/applications/1/hysteria"

	list := serve(r, newJSONRequest(http.MethodGet, base+"/config", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"users":["alice"]`) || strings.Contains(list.Body.String(), "alice-secret-password") || strings.Contains(list.Body.String(), "private-key") {
		t.Fatalf("unexpected redacted summary: %d %s", list.Code, list.Body.String())
	}
	credential := serve(r, newJSONRequest(http.MethodGet, base+"/users/alice/credential", nil))
	if credential.Code != http.StatusOK || !strings.Contains(credential.Body.String(), "alice-secret-password") {
		t.Fatalf("single user credential unavailable: %d %s", credential.Code, credential.Body.String())
	}
	claims.Capability = "other"
	wrongCapability := serve(r, newJSONRequest(http.MethodGet, base+"/users/alice/credential", nil))
	if wrongCapability.Code != http.StatusForbidden {
		t.Fatalf("credential read with unrelated capability = %d", wrongCapability.Code)
	}
	claims.Capability = "hysteria2"
	claims.Actions = []string{"hysteria:write"}
	denied := serve(r, newJSONRequest(http.MethodGet, base+"/users/alice/credential", nil))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("credential read without action = %d", denied.Code)
	}
	claims.Actions = []string{"hysteria:read", "hysteria:write"}
	added := serve(r, newJSONRequest(http.MethodPut, base+"/users/bob", gin.H{"operation": "add", "password": "bob-secret-password", "expected_revision": 1}))
	if added.Code != http.StatusOK || !strings.Contains(added.Body.String(), `"revision":2`) || strings.Contains(added.Body.String(), "bob-secret-password") {
		t.Fatalf("unexpected mutation response: %d %s", added.Code, added.Body.String())
	}
	stale := serve(r, newJSONRequest(http.MethodPut, base+"/users/carol", gin.H{"operation": "add", "password": "carol-password", "expected_revision": 1}))
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale revision = %d %s", stale.Code, stale.Body.String())
	}
	stored, err := s.GetDefaultApplicationDeploymentTemplate(app.ID)
	if err != nil {
		t.Fatal(err)
	}
	updatedSecrets, err := h.decryptTemplateSecrets(stored)
	if err != nil || !strings.Contains(updatedSecrets["config.yaml"], "bob-secret-password") || !strings.Contains(updatedSecrets["config.yaml"], "traffic-secret") || updatedSecrets["tls.key"] != "private-key" {
		t.Fatalf("secret update lost unrelated content: err=%v", err)
	}
	var updatedSpec application.ReleaseSpec
	if err := json.Unmarshal([]byte(stored.Spec), &updatedSpec); err != nil || updatedSpec.Secrets["tls.key"] != "" {
		t.Fatalf("template lost other Secret placeholders: spec=%#v err=%v", updatedSpec.Secrets, err)
	}
	updatedSpec.SecretManagedKeys = nil
	updatedSnapshot, _ := json.Marshal(updatedSpec)
	stored.Spec = string(updatedSnapshot)
	if err := s.UpdateApplicationDeploymentTemplate(stored); err != nil {
		t.Fatal(err)
	}
	unmanaged := serve(r, newJSONRequest(http.MethodGet, base+"/config", nil))
	if unmanaged.Code == http.StatusOK {
		t.Fatalf("unmanaged Secret was exposed: %s", unmanaged.Body.String())
	}
}
