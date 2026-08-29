package registry

import (
	"strings"
	"testing"

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
