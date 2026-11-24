package registry

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gopkg.in/yaml.v3"
)

func TestSetRegistryEndpointPreservesOtherRegistriesAndAuth(t *testing.T) {
	raw := []byte(`# existing node configuration
mirrors:
  registry.k8s.io:
    endpoint:
      - http://100.76.53.48:30501
  ghcr.io:
    endpoint:
      - https://mirror.example.com
configs:
  registry.k8s.io:
    auth:
      username: user
      password: secret
  ghcr.io:
    tls:
      insecure_skip_verify: true
`)
	updated, err := setRegistryEndpoint(raw, "registry.k8s.io", "http://100.67.232.15:31501")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Mirrors map[string]struct {
			Endpoint []string `yaml:"endpoint"`
		} `yaml:"mirrors"`
		Configs map[string]struct {
			Auth struct {
				Username string `yaml:"username"`
				Password string `yaml:"password"`
			} `yaml:"auth"`
			TLS struct {
				Insecure bool `yaml:"insecure_skip_verify"`
			} `yaml:"tls"`
		} `yaml:"configs"`
	}
	if err := yaml.Unmarshal(updated, &config); err != nil {
		t.Fatal(err)
	}
	if got := config.Mirrors["registry.k8s.io"].Endpoint; len(got) != 1 || got[0] != "http://100.67.232.15:31501" {
		t.Fatalf("target endpoints: %v", got)
	}
	if got := config.Mirrors["ghcr.io"].Endpoint; len(got) != 1 || got[0] != "https://mirror.example.com" {
		t.Fatalf("other endpoints changed: %v", got)
	}
	if got := config.Configs["registry.k8s.io"].Auth; got.Username != "user" || got.Password != "secret" {
		t.Fatalf("auth changed: %+v", got)
	}
	if !config.Configs["ghcr.io"].TLS.Insecure {
		t.Fatal("other registry TLS config changed")
	}
}

func TestProxyRelayDeployCommandUsesIsolatedUnit(t *testing.T) {
	var command string
	ssh := SSHExecutorFunc(func(_ context.Context, _ time.Duration, _ *model.Server, actual string) ([]byte, error) {
		command = actual
		return nil, nil
	})
	relay := &model.RegistryProxyRelay{ID: 12, ListenIP: "100.67.232.15", ListenPort: 31501}
	proxy := &model.RegistryProxy{EndpointHost: "100.76.53.48", NodePort: 30501}
	if err := NewProxyRelayService(ssh).Deploy(context.Background(), &model.Server{}, relay, proxy); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"cylism-registry-relay-12.service", "100.67.232.15", "31501", "100.76.53.48:30501", "haproxy -c", "systemctl enable --now", "for attempt in 1 2 3 4 5 6 7 8 9 10", "journalctl -u"} {
		if !strings.Contains(command, expected) {
			t.Errorf("deployment command missing %q", expected)
		}
	}
	if strings.Contains(command, "%!(") || strings.Contains(command, "%!d(") || strings.Contains(command, "%!s(") {
		t.Fatalf("format string mismatch: %s", command)
	}
	check := exec.Command("sh", "-n")
	check.Stdin = strings.NewReader(command)
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("invalid deployment script: %s: %v", output, err)
	}
}

func TestProxyRelayNodeUsesEndpointReadsActualConfig(t *testing.T) {
	ssh := SSHExecutorFunc(func(_ context.Context, _ time.Duration, _ *model.Server, _ string) ([]byte, error) {
		return []byte("mirrors:\n  registry.k8s.io:\n    endpoint:\n      - http://100.67.232.15:31501\n"), nil
	})
	service := NewProxyRelayService(ssh)
	server := &model.Server{}
	used, err := service.NodeUsesEndpoint(context.Background(), server, "registry.k8s.io", "http://100.67.232.15:31501")
	if err != nil || !used {
		t.Fatalf("expected actual endpoint to be in use, used=%t err=%v", used, err)
	}
	used, err = service.NodeUsesEndpoint(context.Background(), server, "registry.k8s.io", "http://100.76.53.48:30501")
	if err != nil || used {
		t.Fatalf("expected old route record to be ignored when actual config differs, used=%t err=%v", used, err)
	}
}
