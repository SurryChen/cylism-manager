package registry

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

type configInspectorRepositoryFake struct {
	mirrors []model.NodeRegistryMirror
	servers []model.Server
}

func (f *configInspectorRepositoryFake) CreateNodeRegistryMirror(*model.NodeRegistryMirror) error {
	return nil
}
func (f *configInspectorRepositoryFake) GetNodeRegistryMirror(uint) (*model.NodeRegistryMirror, error) {
	return nil, nil
}
func (f *configInspectorRepositoryFake) ListNodeRegistryMirrors() ([]model.NodeRegistryMirror, error) {
	return f.mirrors, nil
}
func (f *configInspectorRepositoryFake) UpdateNodeRegistryMirror(*model.NodeRegistryMirror) error {
	return nil
}
func (f *configInspectorRepositoryFake) UpdateNodeRegistryMirrorVerification(uint, string, string, time.Time) error {
	return nil
}
func (f *configInspectorRepositoryFake) DeleteNodeRegistryMirror(uint) error { return nil }
func (f *configInspectorRepositoryFake) UpsertNodeRegistryMirrorStatus(*model.NodeRegistryMirrorNode) error {
	return nil
}
func (f *configInspectorRepositoryFake) ListServers() ([]model.Server, error) { return f.servers, nil }

func TestNodeRegistryConfigInspectorSanitizesAndMatchesConfig(t *testing.T) {
	var command string
	repo := &configInspectorRepositoryFake{
		mirrors: []model.NodeRegistryMirror{{Registry: "docker.io", Endpoints: `["https://mirror.example.com"]`, Username: "robot", Credential: "encrypted", InsecureSkipVerify: true, Enabled: true}},
		servers: []model.Server{{ID: 8, Name: "worker-a", ClusterRole: "worker", SSHAuthType: "key", SSHKey: "key"}},
	}
	inspector := NewNodeRegistryConfigInspector(repo, SSHExecutorFunc(func(_ context.Context, _ time.Duration, _ *model.Server, got string) ([]byte, error) {
		command = got
		return []byte("mirrors:\n  docker.io:\n    endpoint: [https://user:secret@mirror.example.com/path]\nconfigs:\n  docker.io:\n    auth:\n      username: robot\n      password: secret\n    tls:\n      insecure_skip_verify: true\n"), nil
	}))

	result, err := inspector.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 1 || result.Nodes[0].State != NodeRegistryConfigStateMatching {
		t.Fatalf("unexpected result: %#v", result)
	}
	if got := result.Nodes[0].Actual[0].Endpoints; len(got) != 1 || got[0] != "https://mirror.example.com" {
		t.Fatalf("unsafe endpoints: %#v", got)
	}
	if strings.Contains(command, "secret") || !strings.Contains(command, "/etc/rancher/k3s/registries.yaml") || strings.Contains(command, "user:") {
		t.Fatalf("unexpected read command: %q", command)
	}
	if strings.Contains(stringifyInspection(result), "secret") || strings.Contains(stringifyInspection(result), "user@") {
		t.Fatalf("inspection leaked sensitive data: %#v", result)
	}
}

func TestNodeRegistryConfigInspectorClassifiesMissingDriftAndUnsupported(t *testing.T) {
	repo := &configInspectorRepositoryFake{
		mirrors: []model.NodeRegistryMirror{{Registry: "docker.io", Endpoints: `["https://mirror.example.com"]`, Enabled: true}},
		servers: []model.Server{
			{ID: 1, Name: "missing", ClusterRole: "worker", SSHAuthType: "key", SSHKey: "key"},
			{ID: 2, Name: "drifted", ClusterRole: "worker", SSHAuthType: "key", SSHKey: "key"},
			{ID: 3, Name: "unsupported", ClusterRole: "worker", SSHAuthType: "password"},
			{ID: 4, Name: "standalone", SSHAuthType: "key", SSHKey: "key"},
		},
	}
	calls := 0
	inspector := NewNodeRegistryConfigInspector(repo, SSHExecutorFunc(func(_ context.Context, _ time.Duration, server *model.Server, _ string) ([]byte, error) {
		calls++
		if server.ID == 1 {
			return nil, nil
		}
		return []byte("mirrors:\n  docker.io:\n    endpoint: [https://other.example.com]\n  ghcr.io:\n    endpoint: [https://ghcr.example.com]\n"), nil
	}))
	result, err := inspector.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(result.Nodes) != 3 {
		t.Fatalf("calls=%d nodes=%#v", calls, result.Nodes)
	}
	if result.Nodes[0].State != NodeRegistryConfigStateMissing {
		t.Fatalf("missing node: %#v", result.Nodes[0])
	}
	if result.Nodes[1].State != NodeRegistryConfigStateDrifted || len(result.Nodes[1].Extra) != 1 || len(result.Nodes[1].Changed) != 1 {
		t.Fatalf("drifted node: %#v", result.Nodes[1])
	}
	if result.Nodes[2].State != NodeRegistryConfigStateUnsupported {
		t.Fatalf("unsupported node: %#v", result.Nodes[2])
	}
}

func TestNodeRegistryConfigInspectorContinuesAfterReadFailureWithoutLeakingOutput(t *testing.T) {
	repo := &configInspectorRepositoryFake{
		servers: []model.Server{{ID: 1, Name: "failed", ClusterRole: "worker", SSHAuthType: "key", SSHKey: "key"}, {ID: 2, Name: "okay", ClusterRole: "worker", SSHAuthType: "key", SSHKey: "key"}},
	}
	inspector := NewNodeRegistryConfigInspector(repo, SSHExecutorFunc(func(_ context.Context, _ time.Duration, server *model.Server, _ string) ([]byte, error) {
		if server.ID == 1 {
			return []byte("password: very-secret"), errors.New("remote failed")
		}
		return nil, nil
	}))
	result, err := inspector.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Nodes[0].State != NodeRegistryConfigStateUnreachable || result.Nodes[1].State != NodeRegistryConfigStateMatching {
		t.Fatalf("unexpected nodes: %#v", result.Nodes)
	}
	if strings.Contains(stringifyInspection(result), "very-secret") {
		t.Fatalf("inspection leaked remote output: %#v", result)
	}
}

func TestNodeRegistryConfigInspectorChecksOnlyTheSelectedManagedClusterNode(t *testing.T) {
	repo := &configInspectorRepositoryFake{servers: []model.Server{
		{ID: 1, Name: "worker-a", ClusterRole: "worker", SSHAuthType: "key", SSHKey: "key"},
		{ID: 2, Name: "standalone", SSHAuthType: "key", SSHKey: "key"},
	}}
	called := 0
	inspector := NewNodeRegistryConfigInspector(repo, SSHExecutorFunc(func(_ context.Context, _ time.Duration, server *model.Server, _ string) ([]byte, error) {
		called++
		if server.ID != 1 {
			t.Fatalf("unexpected node %d", server.ID)
		}
		return nil, nil
	}))
	result, err := inspector.InspectNode(context.Background(), 1)
	if err != nil || called != 1 || len(result.Nodes) != 1 || result.Nodes[0].ServerID != 1 {
		t.Fatalf("result=%#v err=%v calls=%d", result, err, called)
	}
	if _, err := inspector.InspectNode(context.Background(), 2); err == nil {
		t.Fatal("expected non-cluster server rejection")
	}
}

func stringifyInspection(result NodeRegistryConfigInspection) string {
	payload, _ := json.Marshal(result)
	return string(payload)
}
