package cluster

import "testing"

func TestParseServerStats(t *testing.T) {
	stats := ParseServerStats("CPU: 42.5\nCPU_CORES: 4\nMEM: 8192 4096 3072\nDISK: 40 12 28 30%\nLOAD: 1.25 0.75 0.50\nUP: 3 days")
	if stats["cpu_percent"] != 42.5 || stats["cpu_cores"] != 4 || stats["memory_available_mb"] != 3072 {
		t.Fatalf("unexpected parsed stats: %#v", stats)
	}
	if stats["disk_percent"] != "30%" || stats["uptime"] != "3 days" {
		t.Fatalf("unexpected disk/uptime stats: %#v", stats)
	}
}

func TestCleanHostnameIgnoresSSHWarnings(t *testing.T) {
	got := CleanHostname("Warning: Permanently added host\nnode-a\n")
	if got != "node-a" {
		t.Fatalf("hostname = %q", got)
	}
}
