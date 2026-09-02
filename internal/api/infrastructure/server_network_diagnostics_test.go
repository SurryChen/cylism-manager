package infrastructure

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

func TestServerNetworkDiagnosticsReturnsRedactedSnapshotsAndLinks(t *testing.T) {
	s, _ := store.New(":memory:")
	_ = s.CreateServer(&model.Server{Name: "control-plane", Host: "10.0.0.1"})
	_ = s.CreateServer(&model.Server{Name: "worker", Host: "10.0.0.2"})
	h := NewServerNetworkDiagnosticsHandler(s, make([]byte, 32))
	h.networkSnapshotCollector = func(_ context.Context, server *model.Server) (serverNetworkDiagnostic, error) {
		return serverNetworkDiagnostic{NetworkMode: "k3s_embedded_tailscale", K8sUnit: "k3s", Tailscale: tailscaleStatus{Installed: true, Online: true, TailnetIP: "100.101.102." + fmt.Sprint(server.ID)}}, nil
	}
	h.networkLinkCollector = func(context.Context, *model.Server, string) (tailnetLinkDiagnostic, error) {
		return tailnetLinkDiagnostic{Path: "direct", LatencyMS: 12}, nil
	}
	r := gin.New()
	r.GET("/api/servers/network-diagnostics", h.NetworkDiagnostics)
	w := serve(r, newJSONRequest(http.MethodGet, "/api/servers/network-diagnostics", nil))
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"network_mode":"k3s_embedded_tailscale"`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"path":"direct"`)) {
		t.Fatalf("unexpected diagnostics response: %d %s", w.Code, w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), []byte("joinKey")) || bytes.Contains(w.Body.Bytes(), []byte("ExecStart")) {
		t.Fatalf("diagnostics response leaked remote configuration: %s", w.Body.String())
	}
}

func TestParseTailnetLinkDiagnostic(t *testing.T) {
	link := parseTailnetLinkDiagnostic("PONG 100.76.53.48 via DERP(tok) in 126ms")
	if link.Path != "derp" || link.DERPRegion != "tok" || link.LatencyMS != 126 {
		t.Fatalf("unexpected DERP link: %#v", link)
	}
	link = parseTailnetLinkDiagnostic("pong from host (100.76.53.48) via 149.13.91.192:41641 in 125ms")
	if link.Path != "direct" || link.LatencyMS != 125 || link.DERPRegion != "" {
		t.Fatalf("unexpected direct link: %#v", link)
	}
	link = parseTailnetLinkDiagnostic("ping timed out")
	if link.Path != "unreachable" || link.ErrorCode != "ping_timeout" {
		t.Fatalf("unexpected timeout link: %#v", link)
	}
}

func TestParseNetworkSnapshotClassifiesOnlyActiveK3sVPNConfigurationAsEmbedded(t *testing.T) {
	embedded := parseNetworkSnapshot("K8S_UNIT=k3s\nEMBEDDED_VPN=true\nTAILSCALE_INSTALLED=true\nTAILSCALE_ONLINE=true\nTAILNET_IP=100.81.23.123\nUDP=true\nIPv4=true\nNEAREST_DERP=tok\n")
	if embedded.NetworkMode != "k3s_embedded_tailscale" || embedded.K8sUnit != "k3s" || embedded.Tailscale.TailnetIP != "100.81.23.123" {
		t.Fatalf("unexpected embedded snapshot: %#v", embedded)
	}
	external := parseNetworkSnapshot("K8S_UNIT=k3s\nEMBEDDED_VPN=false\nTAILSCALE_INSTALLED=true\nTAILSCALE_ONLINE=false\n")
	if external.NetworkMode != "external_tailscale" {
		t.Fatalf("expected external Tailscale mode, got %#v", external)
	}
	standard := parseNetworkSnapshot("K8S_UNIT=k3s\nEMBEDDED_VPN=false\nTAILSCALE_INSTALLED=false\n")
	if standard.NetworkMode != "standard_network" {
		t.Fatalf("expected standard network mode, got %#v", standard)
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
		return serverNetworkDiagnostic{NetworkMode: "external_tailscale", Tailscale: tailscaleStatus{Installed: true, Online: true, TailnetIP: "100.81.23.123"}}, nil
	}
	r := gin.New()
	r.GET("/api/servers/network-diagnostics", h.NetworkDiagnostics)
	w := serve(r, newJSONRequest(http.MethodGet, "/api/servers/network-diagnostics", nil))
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"error_code":"ssh_unreachable"`)) || bytes.Contains(w.Body.Bytes(), []byte("host details must remain private")) {
		t.Fatalf("unexpected partial response: %d %s", w.Code, w.Body.String())
	}
}
