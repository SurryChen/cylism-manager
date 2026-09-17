package registry

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestNodeK3sRestarterUsesFixedK3sServiceCommand(t *testing.T) {
	repo := &configInspectorRepositoryFake{servers: []model.Server{{ID: 9, Name: "worker-a", ClusterRole: "worker", SSHAuthType: "key", SSHKey: "key"}}}
	var command string
	restarter := NewNodeK3sServiceRestarter(repo, SSHExecutorFunc(func(_ context.Context, _ time.Duration, _ *model.Server, got string) ([]byte, error) {
		command = got
		return []byte("k3s-agent.service\n"), nil
	}))
	result, err := restarter.Restart(context.Background(), 9)
	if err != nil || result.Status != NodeK3sRestartStatusSucceeded || result.Service != "k3s-agent.service" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	for _, value := range []string{"systemctl show", "k3s.service", "k3s-agent.service", "systemctl restart", "is-active"} {
		if !strings.Contains(command, value) {
			t.Fatalf("command missing %q: %s", value, command)
		}
	}
	if strings.Contains(command, "reboot") || strings.Contains(command, "registries.yaml") {
		t.Fatalf("unsafe command: %s", command)
	}
}

func TestNodeK3sRestarterRejectsUnsupportedNodesAndRedactsRemoteFailure(t *testing.T) {
	repo := &configInspectorRepositoryFake{servers: []model.Server{
		{ID: 1, Name: "unsupported", ClusterRole: "worker", SSHAuthType: "password"},
		{ID: 2, Name: "worker-b", ClusterRole: "worker", SSHAuthType: "key", SSHKey: "key"},
		{ID: 3, Name: "standalone", SSHAuthType: "key", SSHKey: "key"},
	}}
	calls := 0
	restarter := NewNodeK3sServiceRestarter(repo, SSHExecutorFunc(func(_ context.Context, _ time.Duration, _ *model.Server, _ string) ([]byte, error) {
		calls++
		return []byte("password: should-not-leak"), errors.New("exit status 1")
	}))
	unsupported, err := restarter.Restart(context.Background(), 1)
	if err != nil || unsupported.Status != NodeK3sRestartStatusUnsupported || calls != 0 {
		t.Fatalf("result=%#v err=%v calls=%d", unsupported, err, calls)
	}
	failed, err := restarter.Restart(context.Background(), 2)
	if err != nil || failed.Status != NodeK3sRestartStatusFailed || strings.Contains(failed.Detail, "should-not-leak") {
		t.Fatalf("result=%#v err=%v", failed, err)
	}
	if _, err := restarter.Restart(context.Background(), 3); err == nil {
		t.Fatal("expected non-cluster server rejection")
	}
}
