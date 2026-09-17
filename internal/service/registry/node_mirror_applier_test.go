package registry

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestNodeRegistryMirrorApplyCommandDetectsServiceAndReportsRestartFailure(t *testing.T) {
	command := nodeRegistryMirrorApplyCommand("encoded-config")
	for _, part := range []string{"registries.yaml", "encoded-config", "cylism-backup", "tmp=\"${target}.tmp\"", "systemctl show", "k3s.service", "k3s-agent.service", "systemctl restart \"$unit\"", "systemctl is-active"} {
		if !strings.Contains(command, part) {
			t.Fatalf("command missing %q: %s", part, command)
		}
	}
	if strings.Contains(command, "restart k3s || restart k3s-agent") {
		t.Fatalf("command must not mask service restart failures: %s", command)
	}
	if !strings.Contains(command, "sudo -n") {
		t.Fatalf("command must use non-interactive sudo: %s", command)
	}
}

func TestNodeMirrorApplierSkipsServersWithoutKeyAuth(t *testing.T) {
	called := false
	applier := NewNodeMirrorApplier(SSHExecutorFunc(func(context.Context, time.Duration, *model.Server, string) ([]byte, error) {
		called = true
		return nil, nil
	}))
	status, detail := applier.Apply(context.Background(), &model.Server{SSHAuthType: "password"}, []byte("config"))
	if status != "skipped" || detail != "需要已配置的 SSH 密钥认证" || called {
		t.Fatalf("status=%q detail=%q called=%v", status, detail, called)
	}
}

func TestNodeMirrorApplierIncludesRemoteOutputOnFailure(t *testing.T) {
	applier := NewNodeMirrorApplier(SSHExecutorFunc(func(context.Context, time.Duration, *model.Server, string) ([]byte, error) {
		return []byte("systemctl failed"), errors.New("exit status 1")
	}))
	status, detail := applier.Apply(context.Background(), &model.Server{SSHAuthType: "key", SSHKey: "encrypted-key"}, []byte("config"))
	if status != "failed" || !strings.Contains(detail, "systemctl failed") {
		t.Fatalf("status=%q detail=%q", status, detail)
	}
}
