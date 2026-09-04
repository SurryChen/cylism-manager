package shared

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestServerDTODoesNotExposeCredentials(t *testing.T) {
	view := ServerDTO(&model.Server{ID: 7, Name: "node-a", Host: "10.0.0.7", SSHPassword: "secret", SSHKey: "private-key", SSHKeyPassphrase: "passphrase", SSHKeyHash: "hash"})
	if view == nil || view.ID != 7 || view.Host != "10.0.0.7" {
		t.Fatalf("unexpected server view: %+v", view)
	}
	encoded := string(mustJSON(view))
	for _, secret := range []string{"secret", "private-key", "passphrase", "hash"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("server DTO leaked %q: %s", secret, encoded)
		}
	}
}

func TestImageRegistryDTOUsesCredentialConfiguredFlag(t *testing.T) {
	view := ImageRegistryDTO(&model.ImageRegistry{ID: 3, Name: "harbor", Credential: "encrypted", CredentialConfigured: true})
	if view == nil || !view.CredentialConfigured || view.Name != "harbor" {
		t.Fatalf("unexpected registry view: %+v", view)
	}
	if strings.Contains(string(mustJSON(view)), "encrypted") {
		t.Fatal("image registry DTO leaked encrypted credential")
	}
}

func TestRegistryDTOsDoNotExposeEncryptedCredentials(t *testing.T) {
	mirror := NodeRegistryMirrorDTO(&model.NodeRegistryMirror{Name: "mirror", Credential: "mirror-secret"})
	managed := ManagedOCIRegistryDTO(&model.ManagedOCIRegistry{Name: "managed", EncryptedCredential: "managed-secret"})
	for name, value := range map[string]interface{}{"mirror": mirror, "managed": managed} {
		if strings.Contains(string(mustJSON(value)), "secret") {
			t.Fatalf("%s DTO leaked encrypted credential", name)
		}
	}
}

func TestApplicationDTOOmitsPersistenceOnlyFields(t *testing.T) {
	app := &model.Application{ID: 9, Name: "api", CapabilitiesData: `["internal"]`, Capabilities: []string{"public"}, Endpoints: []model.ApplicationEndpoint{{ID: 2, ApplicationID: 9, Domain: "api.example.com", IngressMode: "internal"}}}
	view := ApplicationDTO(app)
	if view == nil || view.ID != 9 || len(view.Endpoints) != 1 || view.Endpoints[0].Domain != "api.example.com" {
		t.Fatalf("unexpected application view: %+v", view)
	}
	encoded := string(mustJSON(view))
	if strings.Contains(encoded, "internal") {
		t.Fatalf("application DTO leaked persistence-only fields: %s", encoded)
	}
}

func TestReleaseAndProxyDTOsExcludeSecrets(t *testing.T) {
	release := ReleaseDTO(&model.Release{ID: 1, DesiredSpec: `{"secret":"value"}`, Operations: []model.ReleaseOperation{{Step: "apply", Detail: "ok"}}})
	proxy := RegistryProxyDTO(&model.RegistryProxy{ID: 2, EncryptedHTTPProxy: "http-secret", EncryptedHTTPSProxy: "https-secret", DNSResolvers: "private"})
	if release == nil || len(release.Operations) != 1 || proxy == nil {
		t.Fatal("expected DTOs")
	}
	encoded := string(mustJSON(proxy))
	for _, secret := range []string{"http-secret", "https-secret", "private"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("proxy DTO leaked %q", secret)
		}
	}
}

func TestInfrastructureDTOsExcludePersistenceOnlySecrets(t *testing.T) {
	site := SiteDTO(&model.Site{ID: 1, Domain: "example.com", Server: model.Server{SSHPassword: "ssh-secret"}})
	credential := DNSCredentialDTO(&model.DNSCredential{Name: "dns", EncryptedValues: "dns-secret", SecretConfigured: true})
	importTask := HostDirectoryPVCImportDTO(&model.HostDirectoryPVCImport{SourcePath: "/data", ApplicationReplicas: "replicas-secret"})
	for name, value := range map[string]interface{}{"site": site, "credential": credential, "import": importTask} {
		encoded := string(mustJSON(value))
		for _, secret := range []string{"ssh-secret", "dns-secret", "replicas-secret"} {
			if strings.Contains(encoded, secret) {
				t.Fatalf("%s DTO leaked %q: %s", name, secret, encoded)
			}
		}
	}
}

func TestAdditionalDTOsProjectOnlyPublicFields(t *testing.T) {
	endpoint := PlatformEndpointDTO(&model.PlatformEndpoint{ID: 4, Hostname: "console.example.com"})
	grant := AgentCapabilityGrantDTO(model.AgentCapabilityGrant{RuntimeID: 8, Capability: "disk.inspect", Namespace: "prod"})
	policy := AlertAutomationPolicyDTO(&model.AlertAutomationPolicy{RuntimeID: 8, Enabled: true, MinimumSeverity: "warning"})
	if endpoint == nil || endpoint.Hostname != "console.example.com" || grant.RuntimeID != 8 || policy == nil || !policy.Enabled {
		t.Fatalf("unexpected DTO projection: endpoint=%+v grant=%+v policy=%+v", endpoint, grant, policy)
	}
}

func TestDashboardAndLogDTOs(t *testing.T) {
	stats := DashboardStatsDTO(&model.DashboardStats{TotalServers: 2, TotalSites: 3, ExpiringCerts: 1})
	certs := CertsDTO([]model.Cert{{ID: 1, Domains: `["api.example.com"]`}})
	logs := AuditLogsDTO([]model.AuditLog{{Action: "read", Detail: `{"secret":"hidden"}`}})
	if stats == nil || stats.TotalServers != 2 || len(certs) != 1 || len(logs) != 1 {
		t.Fatalf("unexpected dashboard DTOs: stats=%+v certs=%+v logs=%+v", stats, certs, logs)
	}
}

func mustJSON(value interface{}) []byte {
	// JSON marshaling cannot fail for these plain DTOs; keeping the helper local
	// makes the redaction assertions concise.
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}
