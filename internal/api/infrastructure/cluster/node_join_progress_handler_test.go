package cluster

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type stubK3sPlatformReader struct {
	version string
	err     error
}

func (r stubK3sPlatformReader) ServerVersionContext(context.Context) (string, error) {
	return r.version, r.err
}

func TestShellEscapeProtectsSingleQuotes(t *testing.T) {
	if got := shellEscape("worker'node"); got != "'worker'\\''node'" {
		t.Fatalf("shellEscape() = %q", got)
	}
}

func TestNodeJoinProgressHandlerConstructorCopiesEncryptionKey(t *testing.T) {
	key := []byte("secret")
	h := NewNodeJoinProgressHandler(nil, key, nil)
	key[0] = 'X'
	if string(h.encKey) != "secret" {
		t.Fatalf("constructor retained mutable key: %q", h.encKey)
	}
}

func TestNodeJoinProgressRejectsUnavailableOrNonK3sPlatformsBeforeWebSocketUpgrade(t *testing.T) {
	tests := []struct {
		name     string
		handler  *NodeJoinProgressHandler
		wantCode int
	}{
		{name: "missing reader", handler: NewNodeJoinProgressHandler(nil, nil, nil), wantCode: http.StatusServiceUnavailable},
		{name: "discovery failure", handler: NewNodeJoinProgressHandler(nil, nil, nil).WithPlatformReader(stubK3sPlatformReader{err: errors.New("unavailable")}), wantCode: http.StatusConflict},
		{name: "kubernetes", handler: NewNodeJoinProgressHandler(nil, nil, nil).WithPlatformReader(stubK3sPlatformReader{version: "v1.31.0"}), wantCode: http.StatusConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/api/nodes/:id/join-progress", tt.handler.JoinProgress)
			response := serve(r, newJSONRequest(http.MethodGet, "/api/nodes/1/join-progress", nil))
			if response.Code != tt.wantCode {
				t.Fatalf("JoinProgress status = %d, want %d: %s", response.Code, tt.wantCode, response.Body.String())
			}
		})
	}
}

func TestK3sAgentInstallCommandHasNoTailscaleDependency(t *testing.T) {
	command := k3sAgentInstallCommand("control.example.internal", "k3s-token")
	for _, forbidden := range []string{"tailscale", "tailscale_auth_key", "tskey", "register_tailscale"} {
		if strings.Contains(strings.ToLower(command), forbidden) {
			t.Fatalf("K3s install command contains retired dependency %q: %s", forbidden, command)
		}
	}
	for _, expected := range []string{"https://control.example.internal:6443", "K3S_TOKEN"} {
		if !strings.Contains(command, expected) {
			t.Fatalf("K3s install command missing %q: %s", expected, command)
		}
	}
}
