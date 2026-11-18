package applicationapi

import (
	"crypto/ecdh"
	"encoding/base64"
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

func TestIntegrationVlessUsersAreScopedRedactedAndVersioned(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	app := createApplicationForReleaseRuntimeTest(t, s)
	if err := app.SetCapabilities([]string{"vless", "hysteria2"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateApplication(app); err != nil {
		t.Fatal(err)
	}
	key := []byte("01234567890123456789012345678901")
	privateBytes := []byte("12345678901234567890123456789012")
	private := base64.RawURLEncoding.EncodeToString(privateBytes)
	privateKey, err := ecdh.X25519().NewPrivateKey(privateBytes)
	if err != nil {
		t.Fatal(err)
	}
	public := base64.RawURLEncoding.EncodeToString(privateKey.PublicKey().Bytes())
	const aliceID = "aabbccdd-1111-4111-8111-111111111111"
	const bobID = "aabbccdd-2222-4222-8222-222222222222"
	const resetID = "aabbccdd-3333-4333-8333-333333333333"
	config := fmt.Sprintf(`{"log":{"level":"warn"},"inbounds":[{"type":"vless","tag":"proxy","listen_port":8443,"users":[{"name":"alice","uuid":%q,"flow":"xtls-rprx-vision"}],"tls":{"enabled":true,"server_name":"www.cloudflare.com","reality":{"enabled":true,"private_key":%q,"short_id":["a1b2"],"handshake":{"server":"www.cloudflare.com","server_port":443}}}}],"route":{"final":"direct"}}`, aliceID, private)
	values, _ := json.Marshal(map[string]string{"config.json": config, "other.key": "unrelated-secret"})
	encrypted, err := crypto.Encrypt(key, string(values))
	if err != nil {
		t.Fatal(err)
	}
	spec := application.ReleaseSpec{
		Image: "ghcr.io/sagernet/sing-box", ContainerPort: 8443, Replicas: 1,
		Resources: application.ResourceSpec{RequestsCPU: "50m", RequestsMemory: "32Mi", LimitsCPU: "500m", LimitsMemory: "128Mi"},
		Secrets:   map[string]string{"config.json": "", "other.key": ""}, SecretManagedKeys: []string{"config.json"},
		FileMounts: []application.FileMountSpec{{SourceType: application.FileMountSourceApplicationSecret, Key: "config.json", MountPath: "/etc/sing-box/config.json"}},
		Service:    application.ServiceSpec{Port: 8443, TargetPort: 8443, Protocol: application.ServiceProtocolTCP},
	}
	snapshot, _ := json.Marshal(spec)
	template := &model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "default", Enabled: true, Spec: string(snapshot), EncryptedSecrets: encrypted}
	if err := s.CreateApplicationDeploymentTemplate(template, true); err != nil {
		t.Fatal(err)
	}
	h := newTestApplicationHandler(s, key, nil)
	claims := &auth.DelegationClaims{UserID: 1, ProjectID: app.ProjectID, EnvironmentIDs: []uint{app.EnvironmentID}, ApplicationIDs: []uint{app.ID}, Capability: "vless", Actions: []string{"vless:read", "vless:write"}}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("delegation", claims); c.Set("user_id", uint(1)); c.Next() })
	r.GET("/api/integrations/applications/:id/vless/config", h.IntegrationVlessConfig)
	r.GET("/api/integrations/applications/:id/vless/users/:username/credential", h.IntegrationVlessCredential)
	r.PUT("/api/integrations/applications/:id/vless/users/:username", h.IntegrationUpdateVlessUser)
	base := fmt.Sprintf("/api/integrations/applications/%d/vless", app.ID)

	list := serve(r, newJSONRequest(http.MethodGet, base+"/config", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"users":["alice"]`) || !strings.Contains(list.Body.String(), public) || strings.Contains(list.Body.String(), aliceID) || strings.Contains(list.Body.String(), private) || strings.Contains(list.Body.String(), "unrelated-secret") || list.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("invalid public summary: %d %s", list.Code, list.Body.String())
	}
	credential := serve(r, newJSONRequest(http.MethodGet, base+"/users/alice/credential", nil))
	if credential.Code != http.StatusOK || !strings.Contains(credential.Body.String(), aliceID) || strings.Contains(credential.Body.String(), private) {
		t.Fatalf("invalid single credential: %d %s", credential.Code, credential.Body.String())
	}
	for _, scope := range []string{"capability", "project", "environment", "application", "action"} {
		t.Run(scope, func(t *testing.T) {
			previous := *claims
			defer func() { *claims = previous }()
			switch scope {
			case "capability":
				claims.Capability = "hysteria2"
			case "project":
				claims.ProjectID = app.ProjectID + 1
			case "environment":
				claims.EnvironmentIDs = []uint{app.EnvironmentID + 1}
			case "application":
				claims.ApplicationIDs = []uint{app.ID + 1}
			case "action":
				claims.Actions = []string{"vless:write"}
			}
			denied := serve(r, newJSONRequest(http.MethodGet, base+"/users/alice/credential", nil))
			if denied.Code != http.StatusForbidden {
				t.Fatalf("credential read with wrong %s: %d", scope, denied.Code)
			}
		})
	}
	mutate := func(name, operation, uuid string, revision uint, restart bool) int {
		res := serve(r, newJSONRequest(http.MethodPut, base+"/users/"+name, gin.H{"operation": operation, "uuid": uuid, "expected_revision": revision, "restart": restart}))
		if strings.Contains(res.Body.String(), private) || strings.Contains(res.Body.String(), bobID) {
			t.Fatal("mutation exposed secret")
		}
		return res.Code
	}
	if got := mutate("bob", "add", bobID, 1, true); got != http.StatusForbidden {
		t.Fatalf("restart without action=%d", got)
	}
	if got := mutate("bob", "add", "invalid", 1, false); got != http.StatusBadRequest {
		t.Fatalf("invalid UUID=%d", got)
	}
	if got := mutate("bob", "add", bobID, 1, false); got != http.StatusOK {
		t.Fatalf("add=%d", got)
	}
	if got := mutate("carol", "add", resetID, 1, false); got != http.StatusConflict {
		t.Fatalf("stale revision=%d", got)
	}
	if got := mutate("carol", "add", strings.ToUpper(bobID), 2, false); got != http.StatusConflict {
		t.Fatalf("duplicate UUID=%d", got)
	}
	if got := mutate("bob", "replace", resetID, 2, false); got != http.StatusOK {
		t.Fatalf("replace=%d", got)
	}
	stored, err := s.GetDefaultApplicationDeploymentTemplate(app.ID)
	if err != nil {
		t.Fatal(err)
	}
	secrets, err := h.decryptTemplateSecrets(stored)
	if err != nil || !strings.Contains(secrets["config.json"], resetID) || !strings.Contains(secrets["config.json"], private) || !strings.Contains(secrets["config.json"], `"final": "direct"`) || secrets["other.key"] != "unrelated-secret" {
		t.Fatalf("unrelated config changed: %v", err)
	}
	if strings.Contains(stored.Spec, private) || strings.Contains(stored.Spec, resetID) {
		t.Fatal("plaintext secret persisted in template spec")
	}
	if got := mutate("alice", "remove", "", 3, false); got != http.StatusOK {
		t.Fatalf("remove=%d", got)
	}
	if got := mutate("bob", "remove", "", 4, false); got == http.StatusOK {
		t.Fatal("last user removed")
	}
	stored, err = s.GetDefaultApplicationDeploymentTemplate(app.ID)
	if err != nil {
		t.Fatal(err)
	}
	var updated application.ReleaseSpec
	if err = json.Unmarshal([]byte(stored.Spec), &updated); err != nil {
		t.Fatal(err)
	}
	updated.SecretManagedKeys = nil
	snapshot, _ = json.Marshal(updated)
	stored.Spec = string(snapshot)
	if err = s.UpdateApplicationDeploymentTemplate(stored); err != nil {
		t.Fatal(err)
	}
	unmanaged := serve(r, newJSONRequest(http.MethodGet, base+"/config", nil))
	if unmanaged.Code == http.StatusOK {
		t.Fatal("unmanaged config exposed")
	}
}

func TestParseVlessConfigRejectsAmbiguousOrInsecureInbound(t *testing.T) {
	const inbound = `{"type":"vless","users":[{"name":"alice","uuid":"aabbccdd-1111-4111-8111-111111111111","flow":"xtls-rprx-vision"}],"tls":{"enabled":true,"reality":{"enabled":true}}}`
	valid := `{"inbounds":[` + inbound + `]}`
	if _, _, err := parseVlessConfig(valid); err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"null", "{}", `{"inbounds":[` + inbound + `,` + inbound + `]}`, strings.Replace(valid, "xtls-rprx-vision", "other", 1), strings.Replace(valid, `"enabled":true`, `"enabled":false`, 1), strings.Replace(valid, "aabbccdd-1111-4111-8111-111111111111", "invalid", 1)} {
		if _, _, err := parseVlessConfig(content); err == nil {
			t.Fatalf("invalid config accepted: %s", content)
		}
	}
}
