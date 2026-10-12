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

func TestAllMigrationNodesConfiguredOnlyAllowsStateSync(t *testing.T) {
	if allMigrationNodesConfigured(nil) {
		t.Fatal("empty precheck allowed state sync")
	}
	for _, action := range []string{"migrate", "normalize", "block"} {
		if allMigrationNodesConfigured([]networkMigrationNode{{Action: "skip", Ready: true}, {Action: action, Ready: true}}) {
			t.Fatalf("%s node allowed state sync", action)
		}
	}
	if !allMigrationNodesConfigured([]networkMigrationNode{{Action: "skip", Ready: true}}) {
		t.Fatal("configured node blocked state sync")
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
	states, err := parseMigrationKubeNodes([]byte(`{"items":[{"metadata":{"name":"node-a"},"status":{"addresses":[{"type":"InternalIP","address":"100.81.23.123"},{"type":"InternalIP","address":"fd7a:115c:a1e0::1"}],"conditions":[{"type":"Ready","status":"True"}]}}]}`))
	if err != nil || !states["node-a"].Ready || states["node-a"].IP != "100.81.23.123" {
		t.Fatalf("states=%v err=%v", states, err)
	}
}

func TestParseMigrationConfigRecognizesInstalledTailscaleNode(t *testing.T) {
	output := `# /etc/systemd/system/k3s-agent.service
[Service]
EnvironmentFile=-/etc/default/%N
EnvironmentFile=-/etc/sysconfig/%N
EnvironmentFile=-/etc/systemd/system/k3s-agent.service.env
ExecStart=/usr/local/bin/k3s agent --node-ip 100.67.232.15 --flannel-iface tailscale0 --server https://100.81.23.123:6443
---CYLISM_ENV---
K3S_TOKEN=secret
---CYLISM_YAML---
`
	config, err := parseMigrationConfig(output)
	if err != nil || configuredTailscaleReason(config, "worker", "100.67.232.15", "100.81.23.123") != "" {
		t.Fatalf("config=%+v err=%v", config, err)
	}
	if configuredTailscaleReason(config, "worker", "100.67.232.15", "100.81.23.124") == "" {
		t.Fatal("accepted worker joined to a different control-plane address")
	}
}

func TestParseMigrationConfigDetectsYAMLAndUnknownEnvironment(t *testing.T) {
	output := `# /etc/systemd/system/k3s.service
[Service]
EnvironmentFile=-/etc/custom/k3s.env
ExecStart=/usr/local/bin/k3s server
---CYLISM_ENV---
---CYLISM_YAML---
node-ip: 100.81.23.123
flannel-iface: tailscale0
advertise-address: 100.81.23.123
---
`
	config, err := parseMigrationConfig(output)
	if err != nil || config.unknownReason != "K3s 使用了自定义环境文件，暂不支持自动迁移" || !config.hasNetworkSettings() {
		t.Fatalf("config=%+v err=%v", config, err)
	}
}

func TestParseMigrationConfigAllowsExistingWorkerJoinAddress(t *testing.T) {
	output := `# /etc/systemd/system/k3s-agent.service
[Service]
EnvironmentFile=-/etc/systemd/system/k3s-agent.service.env
ExecStart=/usr/local/bin/k3s agent
---CYLISM_ENV---
K3S_URL='https://100.81.23.123:6443'
---CYLISM_YAML---
`
	config, err := parseMigrationConfig(output)
	if err != nil || config.unknownReason != "" || config.hasNetworkSettings() || config.values["server"] != "https://100.81.23.123:6443" {
		t.Fatalf("config=%+v err=%v", config, err)
	}
}

func TestParseMigrationConfigRecognizesInstallerMultilineFlags(t *testing.T) {
	output := `active
# /etc/systemd/system/k3s.service
[Service]
EnvironmentFile=-/etc/default/%N
EnvironmentFile=-/etc/sysconfig/%N
EnvironmentFile=-/etc/systemd/system/k3s.service.env
ExecStart=/usr/local/bin/k3s \
  server \
  '--node-ip' \
  '100.81.23.123' \
  '--advertise-address' \
  '100.81.23.123' \
  '--flannel-iface' \
  'tailscale0'
---CYLISM_ENV---
K3S_TOKEN=secret
---CYLISM_YAML---
`
	config, err := parseMigrationConfig(output)
	reason := configuredTailscaleReason(config, "control-plane", "100.81.23.123", "100.81.23.123")
	if err != nil || reason != "" {
		t.Fatalf("config=%+v reason=%q err=%v", config, reason, err)
	}
}
