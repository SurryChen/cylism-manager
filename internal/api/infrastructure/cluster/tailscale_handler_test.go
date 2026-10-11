package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/transport"
)

func TestTailscaleQueryErrorSeparatesSSHAndRemoteFailures(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		output     string
		errorType  string
		messageHas string
	}{
		{"authentication", &transport.SSHConnectionError{Err: errors.New("ssh: unable to authenticate")}, "", "ssh_auth", "认证失败"},
		{"timeout", &transport.SSHConnectionError{Err: context.DeadlineExceeded}, "", "ssh_timeout", "连接超时"},
		{"connection", &transport.SSHConnectionError{Err: errors.New("connection refused")}, "", "ssh_connection", "连接失败"},
		{"daemon", errors.New("exit status 1"), "failed to connect to local tailscaled; tailscaled.sock missing", "tailscale_daemon", "tailscaled"},
		{"command", errors.New("exit status 1"), "secret-token-value", "tailscale_command", "执行失败"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind, message := tailscaleQueryError(test.err, []byte(test.output))
			if kind != test.errorType || !strings.Contains(message, test.messageHas) {
				t.Fatalf("tailscaleQueryError() = %q, %q", kind, message)
			}
			if strings.Contains(message, "secret-token-value") {
				t.Fatalf("remote output leaked: %s", message)
			}
		})
	}
}

func TestTailscaleInstallCommandStartsInstalledDaemon(t *testing.T) {
	for _, test := range []struct {
		user       string
		installCmd string
		startCmd   string
	}{
		{"root", "| sh", "systemctl enable --now tailscaled"},
		{"ubuntu", "| sudo -n sh", "sudo -n systemctl enable --now tailscaled"},
	} {
		command := tailscaleInstallCommand(test.user)
		for _, part := range []string{"if ! command -v tailscale", test.installCmd, test.startCmd} {
			if !strings.Contains(command, part) {
				t.Errorf("command for %s missing %q: %s", test.user, part, command)
			}
		}
	}
}

func TestParseTailscalePing(t *testing.T) {
	tests := []struct {
		name   string
		output string
		ok     bool
		path   string
		relay  string
	}{
		{"relay", "pong from vm (100.81.23.123) via DERP(sfo) in 232ms direct connection not established", true, "relay", "sfo"},
		{"direct after relay", "pong from vm via DERP(sfo) in 232ms\npong from vm via 203.0.113.1:41641 in 30ms", true, "direct", ""},
		{"no reply", "timeout waiting for ping response", false, "inconclusive", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ok, path, relay := parseTailscalePing([]byte(test.output))
			if ok != test.ok || path != test.path || relay != test.relay {
				t.Fatalf("parseTailscalePing() = %t, %q, %q", ok, path, relay)
			}
		})
	}
}

func TestTailscaleDirectFindings(t *testing.T) {
	udp := false
	mapping := true
	nodes := []tailscaleNetworkCheck{
		{ServerID: 1, UDP: &udp},
		{ServerID: 2, MappingVariesByDestIP: &mapping},
	}
	findings := strings.Join(tailscaleDirectFindings(tailscaleConnection{OK: true, Path: "relay"}, nodes, "source", "target"), "\n")
	for _, expected := range []string{"source", "出站 UDP", "target", "NAT", "DERP"} {
		if !strings.Contains(findings, expected) {
			t.Errorf("findings missing %q: %s", expected, findings)
		}
	}
	if findings := tailscaleDirectFindings(tailscaleConnection{OK: true, Path: "direct"}, nodes, "source", "target"); len(findings) != 1 || !strings.Contains(findings[0], "已建立直连") {
		t.Fatalf("unexpected direct findings: %v", findings)
	}
}

func TestParseTailscaleStatus(t *testing.T) {
	output, _ := json.Marshal(map[string]any{
		"BackendState": "Running",
		"Self": map[string]any{
			"DNSName":      "worker.tailnet.ts.net.",
			"TailscaleIPs": []string{"fd7a:115c:a1e0::1", "100.81.23.123"},
			"Online":       true,
		},
	})
	status := parseTailscaleStatus(7, output)
	if status.ServerID != 7 || status.Status != "online" || status.IP != "100.81.23.123" || status.DeviceName != "worker.tailnet.ts.net" {
		t.Fatalf("unexpected status: %#v", status)
	}
	if status := parseTailscaleStatus(7, []byte(`{}`)); status.Status != "offline" {
		t.Fatalf("expected offline status, got %#v", status)
	}
}

func TestK3sTailscaleInstallCommandUsesPrivateNodeIP(t *testing.T) {
	control := k3sInstallCommandForNetwork("control-plane", "official", "100.81.23.123", "", "tailscale", "100.81.23.123")
	for _, flag := range []string{"--tls-san 100.81.23.123", "--node-ip 100.81.23.123", "--advertise-address 100.81.23.123", "--flannel-iface tailscale0"} {
		if !strings.Contains(control, flag) {
			t.Fatalf("control command missing %q: %s", flag, control)
		}
	}
	worker := k3sInstallCommandForNetwork("worker", "official", "100.81.23.123", "test-token", "tailscale", "100.81.23.124")
	for _, value := range []string{"100.81.23.123:6443", "--node-ip 100.81.23.124", "--flannel-iface tailscale0"} {
		if !strings.Contains(worker, value) {
			t.Fatalf("worker command missing %q: %s", value, worker)
		}
	}
}
