package registry

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestRegistryCommandsUsePaddedBase64ForShellDecoder(t *testing.T) {
	image := "registry.k8s.io/pause:3.10"
	encodedImage := base64.StdEncoding.EncodeToString([]byte(image))
	pullCommand := PullCommand(image)
	if !strings.Contains(encodedImage, "=") || !strings.Contains(pullCommand, "printf %s "+encodedImage+" | base64 -d") {
		t.Fatalf("pull command must use padded standard base64: %s", pullCommand)
	}
	for _, expected := range []string{"sudo -n /usr/local/bin/crictl pull \"$image\"", "sudo -n /var/lib/rancher/k3s/bin/crictl pull \"$image\"", "sudo -n /usr/local/bin/k3s crictl pull \"$image\"", "未找到 crictl 或 k3s 命令"} {
		if !strings.Contains(pullCommand, expected) {
			t.Fatalf("missing %q", expected)
		}
	}
	verificationCommand := VerificationCommand([]string{"https://a"})
	encodedEndpoints := base64.StdEncoding.EncodeToString([]byte(`["https://a"]`))
	if !strings.Contains(encodedEndpoints, "=") || !strings.Contains(verificationCommand, "CYLISM_ENDPOINTS_B64="+encodedEndpoints+" sh -c") || !strings.Contains(verificationCommand, "read -r endpoint || [ -n \"$endpoint\" ]") {
		t.Fatalf("verification command invalid: %s", verificationCommand)
	}
}

func TestParseEndpointResultsRejectsEmptyVerificationOutput(t *testing.T) {
	results, err := ParseEndpointResults("Warning: Permanently added '10.0.0.1' (ED25519) to the list of known hosts.\nhttps://mirror.example.com|ok|200\n", 1)
	if err != nil || len(results) != 1 || results[0] != (EndpointResult{Endpoint: "https://mirror.example.com", DNS: "ok", HTTP: "200"}) {
		t.Fatalf("parsed results = %#v, %v", results, err)
	}
	if _, err = ParseEndpointResults("Warning: remote command emitted no probe rows", 1); err == nil || !strings.Contains(err.Error(), "no endpoint results") {
		t.Fatalf("expected no-result verification error, got %v", err)
	}
}
