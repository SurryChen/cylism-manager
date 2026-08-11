package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cylism/cylism-manager/internal/agentartifact"
)

type artifactAuthorizerStub struct {
	token string
	err   error
}

func (stub artifactAuthorizerStub) AuthorizeInstaller(_ context.Context, token string) error {
	if stub.err != nil {
		return stub.err
	}
	if token != stub.token {
		return errUnauthorizedInstaller
	}
	return nil
}

func TestAgentArtifactHandlerRequiresInstallerIdentityAndServesFixedArtifact(t *testing.T) {
	directory := writeCLIArtifact(t)
	handler := NewAgentArtifactHandler(directory, artifactAuthorizerStub{token: "installer-token"})

	unauthorized := httptest.NewRequest(http.MethodGet, "/internal/runtime-tools/v1/cylism-cli/linux-amd64", nil)
	unauthorizedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedRecorder, unauthorized)
	if unauthorizedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated request to fail, got %d", unauthorizedRecorder.Code)
	}

	manifestRequest := httptest.NewRequest(http.MethodGet, "/internal/runtime-tools/v1/cylism-cli/linux-amd64/manifest", nil)
	manifestRequest.Header.Set("Authorization", "Bearer installer-token")
	manifestRecorder := httptest.NewRecorder()
	handler.ServeHTTP(manifestRecorder, manifestRequest)
	if manifestRecorder.Code != http.StatusOK {
		t.Fatalf("expected manifest request to succeed, got %d: %s", manifestRecorder.Code, manifestRecorder.Body.String())
	}
	var manifest agentartifact.Manifest
	if err := json.Unmarshal(manifestRecorder.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.Platform != "linux-amd64" {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}

	binaryRequest := httptest.NewRequest(http.MethodGet, "/internal/runtime-tools/v1/cylism-cli/linux-amd64", nil)
	binaryRequest.Header.Set("Authorization", "Bearer installer-token")
	binaryRecorder := httptest.NewRecorder()
	handler.ServeHTTP(binaryRecorder, binaryRequest)
	if binaryRecorder.Code != http.StatusOK || binaryRecorder.Header().Get("X-Cylism-CLI-SHA256") != manifest.SHA256 || binaryRecorder.Body.String() != "cli-binary" {
		t.Fatalf("unexpected binary response: status=%d headers=%#v body=%q", binaryRecorder.Code, binaryRecorder.Header(), binaryRecorder.Body.String())
	}
}

func TestAgentArtifactHandlerRejectsOtherPathsAndMissingArtifact(t *testing.T) {
	handler := NewAgentArtifactHandler(t.TempDir(), artifactAuthorizerStub{token: "installer-token"})
	request := httptest.NewRequest(http.MethodGet, "/internal/runtime-tools/v1/cylism-cli/linux-arm64", nil)
	request.Header.Set("Authorization", "Bearer installer-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected non-fixed platform to be hidden, got %d", recorder.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/internal/runtime-tools/v1/cylism-cli/linux-amd64", nil)
	request.Header.Set("Authorization", "Bearer installer-token")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected missing artifact to be unavailable, got %d", recorder.Code)
	}
}

func writeCLIArtifact(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	binary := []byte("cli-binary")
	if err := os.WriteFile(filepath.Join(directory, agentartifact.BinaryFileName), binary, 0o555); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	digest := sha256.Sum256(binary)
	manifest := agentartifact.Manifest{Version: "test", Platform: "linux-amd64", Size: int64(len(binary)), SHA256: hex.EncodeToString(digest[:])}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, agentartifact.ManifestFileName), encoded, 0o444); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return directory
}
