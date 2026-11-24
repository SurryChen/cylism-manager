package registry

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"sigs.k8s.io/yaml"
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

func TestNodeMirrorApplierSupportsPasswordAuthenticatedServers(t *testing.T) {
	called := false
	applier := NewNodeMirrorApplier(SSHExecutorFunc(func(context.Context, time.Duration, *model.Server, string) ([]byte, error) {
		called = true
		return nil, nil
	}))
	status, detail := applier.Apply(context.Background(), &model.Server{SSHAuthType: "password", SSHPassword: "encrypted-password"}, []byte("config"))
	if status != "success" || detail != "" || !called {
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

func TestNodeRegistryConfigCommandsSupportRootAndRollback(t *testing.T) {
	read := nodeRegistryConfigReadCommand()
	for _, part := range []string{"id -u", "cat \"$target\"", "sudo -n cat \"$target\""} {
		if !strings.Contains(read, part) {
			t.Fatalf("read command missing %q: %s", part, read)
		}
	}

	write := nodeRegistryConfigWriteCommand("encoded-config")
	for _, part := range []string{"sudo_cmd=", "cylism-sync.backup", "systemctl show", "正在恢复旧配置", "systemctl is-active"} {
		if !strings.Contains(write, part) {
			t.Fatalf("write command missing %q: %s", part, write)
		}
	}
}

func TestMergeSafeRegistryConfigsUpdatesCredentialsAndPreservesBlankCredential(t *testing.T) {
	raw := []byte(`mirrors:
  docker.io:
    endpoint:
      - https://old.example.com
configs:
  docker.io:
    auth:
      username: old-user
      password: old-password
`)
	updated, err := mergeSafeRegistryConfigs(raw, []SafeRegistryConfig{{
		Registry:     "docker.io",
		Endpoints:    []string{"https://new.example.com"},
		AuthUsername: "new-user",
	}})
	if err != nil {
		t.Fatal(err)
	}
	var parsed k3sRegistriesConfig
	if err := yaml.Unmarshal(updated, &parsed); err != nil {
		t.Fatal(err)
	}
	auth := parsed.Configs["docker.io"].Auth
	if auth == nil || auth.Username != "new-user" || auth.Password != "old-password" {
		t.Fatalf("credentials were not merged safely: %#v", auth)
	}
	if got := parsed.Mirrors["docker.io"].Endpoint; len(got) != 1 || got[0] != "https://new.example.com" {
		t.Fatalf("endpoints were not updated: %#v", got)
	}
}
