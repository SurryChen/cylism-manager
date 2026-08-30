package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/cylism/cylism-manager/internal/agentartifact"
)

const (
	CLIArtifactPath         = "/internal/runtime-tools/v1/cylism-cli/linux-amd64"
	CLIArtifactManifestPath = CLIArtifactPath + "/manifest"
	cliArtifactPlatform     = "linux-amd64"
)

var errUnauthorizedInstaller = errors.New("unauthorized installer")

// InstallerAuthorizer identifies a projected Runtime installer token. It must
// reject agent-operation tokens and any token not belonging to an enabled Runtime.
type InstallerAuthorizer interface {
	AuthorizeInstaller(ctx context.Context, token string) error
}

type AgentArtifactHandler struct {
	directory  string
	authorizer InstallerAuthorizer
}

func NewAgentArtifactHandler(directory string, authorizer InstallerAuthorizer) *AgentArtifactHandler {
	return &AgentArtifactHandler{directory: directory, authorizer: authorizer}
}

func (h *AgentArtifactHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || (r.URL.Path != CLIArtifactPath && r.URL.Path != CLIArtifactManifestPath) {
		http.NotFound(w, r)
		return
	}
	if h == nil || h.authorizer == nil {
		http.Error(w, "installer authentication unavailable", http.StatusServiceUnavailable)
		return
	}
	token, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok || h.authorizer.AuthorizeInstaller(r.Context(), token) != nil {
		http.Error(w, "installer authentication required", http.StatusUnauthorized)
		return
	}
	artifact, err := agentartifact.Load(h.directory, cliArtifactPlatform)
	if err != nil {
		http.Error(w, "CLI artifact unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.URL.Path == CLIArtifactManifestPath {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(artifact.Manifest); err != nil {
			http.Error(w, "unable to encode CLI manifest", http.StatusInternalServerError)
		}
		return
	}
	file, err := os.Open(artifact.BinaryPath)
	if err != nil {
		http.Error(w, "CLI artifact unavailable", http.StatusServiceUnavailable)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Cylism-CLI-Version", artifact.Manifest.Version)
	w.Header().Set("X-Cylism-CLI-SHA256", artifact.Manifest.SHA256)
	w.Header().Set("X-Cylism-CLI-Size", strconv.FormatInt(artifact.Manifest.Size, 10))
	_, _ = io.Copy(w, file)
}

func bearerToken(value string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(value, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(value, prefix))
	return token, token != "" && !strings.ContainsAny(token, "\r\n")
}
