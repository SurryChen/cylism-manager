package registry

import (
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
)

func TestRenderK3sRegistriesUsesDecryptedCredentialAndTLSSetting(t *testing.T) {
	content, err := RenderK3sRegistries([]model.NodeRegistryMirror{{
		ID:                 7,
		Name:               "platform",
		Registry:           "registry.example.com",
		Endpoints:          `["https://registry.example.com"]`,
		Username:           "pull",
		Credential:         "encrypted-at-rest",
		InsecureSkipVerify: true,
		Enabled:            true,
	}}, map[uint]string{7: "decrypted-password"})
	if err != nil {
		t.Fatalf("RenderK3sRegistries returned error: %v", err)
	}
	text := string(content)
	for _, expected := range []string{"registry.example.com", "pull", "decrypted-password", "insecure_skip_verify: true"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("rendered K3s configuration missing %q: %s", expected, text)
		}
	}
}

func TestRenderK3sRegistriesWithStoredCredentialsDecryptsOnlyForRendering(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	encrypted, err := crypto.Encrypt(key, "registry-password")
	if err != nil {
		t.Fatal(err)
	}
	content, err := RenderK3sRegistriesWithStoredCredentials([]model.NodeRegistryMirror{{
		ID: 7, Name: "platform", Registry: "registry.example.com", Endpoints: `["https://registry.example.com"]`,
		Username: "pull", Credential: encrypted, Enabled: true,
	}}, key)
	if err != nil {
		t.Fatalf("RenderK3sRegistriesWithStoredCredentials returned error: %v", err)
	}
	if !strings.Contains(string(content), "registry-password") {
		t.Fatalf("rendered configuration did not contain the resolved credential: %s", content)
	}
}
