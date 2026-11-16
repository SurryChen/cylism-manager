package cluster

import (
	"context"
	"errors"
	"os/exec"
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
