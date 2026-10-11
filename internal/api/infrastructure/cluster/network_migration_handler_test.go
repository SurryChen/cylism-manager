package cluster

import (
	"strings"
	"testing"
)

func TestParseMigrationUnitAcceptsInstallerService(t *testing.T) {
	unit := `active
/etc/systemd/system/k3s.service

[Service]
ExecStart=/usr/local/bin/k3s \
  server \
  '--tls-san' \
  '203.0.113.10'
`
	start, err := parseMigrationUnit(unit, "k3s", "control-plane")
	if err != nil || !strings.Contains(start, "server '--tls-san' '203.0.113.10'") {
		t.Fatalf("start=%q err=%v", start, err)
	}
}

func TestParseMigrationUnitRejectsCustomNetworkAndDropIn(t *testing.T) {
	for _, output := range []string{
		"active\n/etc/systemd/system/k3s.service\n/etc/systemd/system/k3s.service.d/custom.conf\nExecStart=/usr/local/bin/k3s server\n",
		"active\n/etc/systemd/system/k3s.service\n\nExecStart=/usr/local/bin/k3s server --node-ip 10.0.0.2\n",
		"active\n/etc/systemd/system/k3s.service\n\nExecStart=/bin/sh -c 'k3s server'\n",
	} {
		if _, err := parseMigrationUnit(output, "k3s", "control-plane"); err == nil {
			t.Fatalf("accepted unsafe unit %q", output)
		}
	}
}

func TestParseMigrationKubeNodes(t *testing.T) {
	states, err := parseMigrationKubeNodes([]byte(`{"items":[{"metadata":{"name":"node-a"},"status":{"addresses":[{"type":"InternalIP","address":"100.81.23.123"}],"conditions":[{"type":"Ready","status":"True"}]}}]}`))
	if err != nil || !states["node-a"].Ready || states["node-a"].IP != "100.81.23.123" {
		t.Fatalf("states=%v err=%v", states, err)
	}
}
