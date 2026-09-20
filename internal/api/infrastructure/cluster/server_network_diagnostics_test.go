package cluster

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestServerNetworkDiagnosticsReturnsK3sVPNStatusWithoutSecrets(t *testing.T) {
	s, _ := store.New(":memory:")
	_ = s.CreateServer(&model.Server{Name: "control-plane", Host: "10.0.0.1"})
	h := NewServerNetworkDiagnosticsHandler(s, make([]byte, 32))
	h.networkSnapshotCollector = func(_ context.Context, server *model.Server) (serverNetworkDiagnostic, error) {
		return serverNetworkDiagnostic{K8sUnit: "k3s", K3sVPN: k3sVPNStatus{Configured: true, Provider: "tailscale"}}, nil
	}
	r := gin.New()
	r.GET("/api/servers/network-diagnostics", h.NetworkDiagnostics)
	w := serve(r, newJSONRequest(http.MethodGet, "/api/servers/network-diagnostics", nil))
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"configured":true`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"provider":"tailscale"`)) {
		t.Fatalf("unexpected diagnostics response: %d %s", w.Code, w.Body.String())
	}
	for _, secret := range []string{"joinKey", "ExecStart", "tskey-auth-", "TAILNET_IP"} {
		if bytes.Contains(w.Body.Bytes(), []byte(secret)) {
			t.Fatalf("diagnostics response leaked %q: %s", secret, w.Body.String())
		}
	}
}

func TestParseNetworkSnapshotOnlyReportsActiveK3sVPNConfiguration(t *testing.T) {
	configured := parseNetworkSnapshot("K8S_UNIT=k3s\nK3S_VPN_CONFIGURED=true\nK3S_VPN_PROVIDER=tailscale\n")
	if configured.K8sUnit != "k3s" || !configured.K3sVPN.Configured || configured.K3sVPN.Provider != "tailscale" {
		t.Fatalf("unexpected configured snapshot: %#v", configured)
	}
	standard := parseNetworkSnapshot("K8S_UNIT=k3s\nK3S_VPN_CONFIGURED=false\n")
	if standard.K3sVPN.Configured || standard.K3sVPN.Provider != "" {
		t.Fatalf("standard K3s must not report a VPN integration: %#v", standard)
	}
	nonK3s := parseNetworkSnapshot("K8S_UNIT=\nK3S_VPN_CONFIGURED=true\nK3S_VPN_PROVIDER=tailscale\n")
	if nonK3s.K3sVPN.Configured {
		t.Fatalf("inactive K3s state must not report a VPN integration: %#v", nonK3s)
	}
}

func TestServerNetworkDiagnosticsKeepsPartialFailuresStructured(t *testing.T) {
	s, _ := store.New(":memory:")
	_ = s.CreateServer(&model.Server{Name: "reachable", Host: "10.0.0.1"})
	_ = s.CreateServer(&model.Server{Name: "unreachable", Host: "10.0.0.2"})
	h := NewServerNetworkDiagnosticsHandler(s, make([]byte, 32))
	h.networkSnapshotCollector = func(_ context.Context, server *model.Server) (serverNetworkDiagnostic, error) {
		if server.Name == "unreachable" {
			return serverNetworkDiagnostic{}, fmt.Errorf("connection timed out: host details must remain private")
		}
		return serverNetworkDiagnostic{K8sUnit: "k3s"}, nil
	}
	r := gin.New()
	r.GET("/api/servers/network-diagnostics", h.NetworkDiagnostics)
	w := serve(r, newJSONRequest(http.MethodGet, "/api/servers/network-diagnostics", nil))
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"error_code":"ssh_unreachable"`)) || bytes.Contains(w.Body.Bytes(), []byte("host details must remain private")) {
		t.Fatalf("unexpected partial response: %d %s", w.Code, w.Body.String())
	}
}
