package cluster

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestRemovalRoutesPreserveUnrelatedSubnetsAndExitNode(t *testing.T) {
	prefs := []byte(`{"AdvertiseRoutes":["10.42.1.0/24","192.168.8.0/24","0.0.0.0/0","::/0","fd00:42::/64"]}`)
	remaining, changed, err := remainingRemovalRoutes(prefs, `["10.42.1.0/24","fd00:42::/64"]`)
	if err != nil || !changed || !reflect.DeepEqual(remaining, []string{"192.168.8.0/24", "0.0.0.0/0", "::/0"}) {
		t.Fatalf("routes: %v %t %v", remaining, changed, err)
	}
	for _, bad := range []string{"", `["not-a-cidr"]`} {
		if _, _, err = remainingRemovalRoutes(prefs, bad); err == nil {
			t.Fatal("invalid task routes accepted")
		}
	}
}

func TestUninstallHostRequiresExplicitChoiceAndStoppedCheckpoint(t *testing.T) {
	calls := 0
	host := &sshNodeRemovalHost{exec: func(context.Context, time.Duration, *model.Server, []byte, string) ([]byte, error) {
		calls++
		return nil, nil
	}}
	for _, task := range []*model.ClusterDeployment{
		{NodeMachineID: "machine", NodeStopped: true},
		{NodeMachineID: "machine", UninstallK3s: true},
		{NodeStopped: true, UninstallK3s: true},
	} {
		if host.Uninstall(context.Background(), &model.Server{}, task) == nil {
			t.Fatal("unsafe uninstall accepted")
		}
	}
	if calls != 0 {
		t.Fatal("invalid cleanup contacted the host")
	}
}

func TestUninstallShellGuardsAndIdempotentCompletion(t *testing.T) {
	for _, scenario := range []string{"stopped", "wrong-machine", "active", "enabled", "control-plane", "containers", "already-gone", "partially-gone"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			bin, data, config := filepath.Join(root, "bin"), filepath.Join(root, "data"), filepath.Join(root, "config")
			for _, dir := range []string{bin, data, config} {
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			machine := "machine-1"
			if scenario == "wrong-machine" {
				machine = "replacement-machine"
			}
			if err := os.WriteFile(filepath.Join(root, "machine-id"), []byte(machine), 0o600); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(root, "executed")
			if scenario != "already-gone" && scenario != "partially-gone" {
				if err := os.WriteFile(filepath.Join(bin, "k3s-agent-uninstall.sh"), []byte("#!/bin/sh\ntouch "+shellEscape(marker)+"\nexit 17\n"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			active, enabled, control, shims, load := "inactive", "disabled", "not-found", "", "loaded"
			if scenario == "active" {
				active = "active"
			}
			if scenario == "enabled" {
				enabled = "enabled"
			}
			if scenario == "control-plane" {
				control = "loaded"
			}
			if scenario == "containers" {
				shims = "/bin/containerd-shim -address /run/k3s/containerd/containerd.sock"
			}
			if scenario == "already-gone" {
				if err := os.Remove(data); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(config); err != nil {
					t.Fatal(err)
				}
				active, enabled, load = "unknown", "", "not-found"
			}
			preamble := `systemctl() { case "$1:$2" in show:k3s) printf '%s' "$CONTROL" ;; show:k3s-agent) printf '%s' "$LOAD" ;; is-active:*) printf '%s' "$ACTIVE" ;; is-enabled:*) printf '%s' "$ENABLED" ;; *) return 1 ;; esac; }
ps() { printf '%s\n' "$SHIMS"; }
`
			task := &model.ClusterDeployment{NodeMachineID: "machine-1", NodeStopped: true, UninstallK3s: true}
			command := strings.NewReplacer("/etc/machine-id", filepath.Join(root, "machine-id"), "/usr/local/bin", bin, "/var/lib/rancher/k3s", data, "/etc/rancher/k3s", config).Replace(removalHostUninstallCommand(task))
			cmd := exec.Command("sh", "-c", preamble+command)
			cmd.Env = append(os.Environ(), "CONTROL="+control, "ACTIVE="+active, "ENABLED="+enabled, "SHIMS="+shims, "LOAD="+load)
			output, err := cmd.CombinedOutput()
			_, markerErr := os.Stat(marker)
			if scenario == "already-gone" {
				if err != nil {
					t.Fatalf("idempotent verification failed: %v %s", err, output)
				}
			} else if err == nil {
				t.Fatal("unsafe or partial uninstall reported success")
			}
			if (markerErr == nil) != (scenario == "stopped") {
				t.Fatalf("wrong script invocation: %s %v %s", scenario, markerErr, output)
			}
		})
	}
}

func TestRemovalHostCommandsAreValidAndDoNotUninstall(t *testing.T) {
	task := &model.ClusterDeployment{NodeMachineID: "machine'; echo injected", NodeStopped: true}
	command := removalHostCheckCommand(task) + removalHostStopCommand
	cmd := exec.Command("sh", "-n")
	cmd.Stdin = strings.NewReader(command)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("invalid shell: %s %v", output, err)
	}
	for _, unsafe := range []string{"uninstall.sh", "killall.sh", "rm -", "tailscale down"} {
		if strings.Contains(command, unsafe) {
			t.Fatalf("unsafe command: %s", unsafe)
		}
	}
	for _, required := range []string{"systemctl disable --now k3s-agent", "/etc/machine-id", "/var/lib/rancher/k3s/server/db", "/run/k3s/containerd/containerd.sock", "containerd-shim"} {
		if !strings.Contains(command, required) {
			t.Fatalf("missing check: %s", required)
		}
	}
}

func TestRemovalHostFailureDoesNotProceedToRouteChanges(t *testing.T) {
	calls := 0
	host := &sshNodeRemovalHost{exec: func(context.Context, time.Duration, *model.Server, []byte, string) ([]byte, error) {
		calls++
		return nil, errors.New("host mismatch")
	}}
	task := &model.ClusterDeployment{NodeMachineID: "machine-1", NodePodCIDRs: `["10.42.1.0/24"]`}
	if err := host.Stop(context.Background(), &model.Server{SSHUser: "root"}, task); err == nil || calls != 1 {
		t.Fatalf("unexpected host calls: %d %v", calls, err)
	}
}

func TestRemovalHostOnlyWithdrawsItsOwnPodRoutes(t *testing.T) {
	var commands []string
	host := &sshNodeRemovalHost{exec: func(_ context.Context, _ time.Duration, _ *model.Server, _ []byte, command string) ([]byte, error) {
		commands = append(commands, command)
		if strings.Contains(command, "tailscale debug prefs") {
			return []byte(`{"AdvertiseRoutes":["10.42.1.0/24","192.168.8.0/24","0.0.0.0/0","::/0"]}`), nil
		}
		return nil, nil
	}}
	task := &model.ClusterDeployment{NodeMachineID: "machine-1", NodePodCIDRs: `["10.42.1.0/24"]`}
	if err := host.Stop(context.Background(), &model.Server{SSHUser: "root"}, task); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 3 || !strings.Contains(commands[2], "--advertise-routes='192.168.8.0/24'") {
		t.Fatalf("unexpected route mutation: %v", commands)
	}
}
