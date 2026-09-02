package cluster

import (
	"context"
	"errors"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
)

type fakeNodeAdapter struct {
	nodes       []k8s.NodeInfo
	labels      *k8s.NodeLabels
	deletedNode string
	drainPlan   *k8s.DrainPlan
	drainResult *k8s.DrainResult
	rejoined    *k8s.NodeInfo
	removal     *k8s.NodeRemovalCheck
	getLabels   *k8s.NodeLabels
	listErr     error
	drainErr    error
	forceErr    error
	lastDrain   k8s.DrainOptions
	lastForce   k8s.ForceDrainOptions
}

type fakeServerInspector struct {
	reachable  bool
	probeError string
	checks     []Precheck
	probed     uint
	checked    uint
}

type fakeServerImporter struct {
	hostname string
	err      error
}

type fakeMetricsInspector struct{}

func (fakeMetricsInspector) ResourceStats(_ context.Context, _ *model.Server) (map[string]interface{}, error) {
	return map[string]interface{}{"cpu_percent": 12.5}, nil
}

func (f fakeServerImporter) Hostname(_ context.Context, _ *model.Server) (string, error) {
	return f.hostname, f.err
}

func (f *fakeServerInspector) Probe(_ context.Context, server *model.Server) (bool, string) {
	f.probed = server.ID
	return f.reachable, f.probeError
}

func (f *fakeServerInspector) Precheck(_ context.Context, server *model.Server) []Precheck {
	f.checked = server.ID
	return f.checks
}

func (f *fakeNodeAdapter) ListNodeInfosContext(context.Context) ([]k8s.NodeInfo, error) {
	return f.nodes, f.listErr
}

func (f *fakeNodeAdapter) GetNodeLabelsContext(context.Context, string) (*k8s.NodeLabels, error) {
	return f.getLabels, nil
}

func (f *fakeNodeAdapter) UpdateNodeLabelsContext(context.Context, string, map[string]string, []string) (*k8s.NodeLabels, error) {
	return f.labels, nil
}

func (f *fakeNodeAdapter) DeleteNodeContext(_ context.Context, name string) error {
	f.deletedNode = name
	return nil
}

func (f *fakeNodeAdapter) DrainPlanContext(context.Context, string) (*k8s.DrainPlan, error) {
	return f.drainPlan, nil
}

func (f *fakeNodeAdapter) DrainNodeContext(_ context.Context, _ string, options k8s.DrainOptions) (*k8s.DrainResult, error) {
	f.lastDrain = options
	return f.drainResult, f.drainErr
}

func (f *fakeNodeAdapter) ForceDrainNodeContext(_ context.Context, _ string, options k8s.ForceDrainOptions) (*k8s.DrainResult, error) {
	f.lastForce = options
	return f.drainResult, f.forceErr
}

func (f *fakeNodeAdapter) RejoinNodeContext(context.Context, string) (*k8s.NodeInfo, error) {
	return f.rejoined, nil
}

func (f *fakeNodeAdapter) NodeRemovalCheckContext(context.Context, string) (*k8s.NodeRemovalCheck, error) {
	return f.removal, nil
}

func TestServiceReconcilesOnlyRemovedClusterBindings(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	missing := &model.Server{Name: "missing", Host: "10.0.0.11", ClusterRole: "worker", K8sNodeName: "worker-missing"}
	present := &model.Server{Name: "present", Host: "10.0.0.12", ClusterRole: "worker", K8sNodeName: "worker-present"}
	if err := st.CreateServer(missing); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateServer(present); err != nil {
		t.Fatal(err)
	}

	service := NewService(st, &fakeNodeAdapter{nodes: []k8s.NodeInfo{{Name: "worker-present"}}})
	servers, err := service.ListServersContext(context.Background())
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("server count = %d, want 2", len(servers))
	}
	storedMissing, err := st.GetServer(missing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedMissing.ClusterRole != "" || storedMissing.K8sNodeName != "" {
		t.Fatalf("missing node binding was not cleared: %#v", storedMissing)
	}
	storedPresent, err := st.GetServer(present.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedPresent.K8sNodeName != "worker-present" {
		t.Fatalf("present node binding changed: %#v", storedPresent)
	}
}

func TestServiceUpdatesLabelsAndUnbindsRemovedNode(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	server := &model.Server{Name: "worker", Host: "10.0.0.11", ClusterRole: "worker", K8sNodeName: "worker-a"}
	if err := st.CreateServer(server); err != nil {
		t.Fatal(err)
	}
	adapter := &fakeNodeAdapter{labels: &k8s.NodeLabels{Name: "worker-a", Labels: map[string]string{"team": "platform"}}}
	service := NewService(st, adapter)

	labels, err := service.UpdateNodeLabelsContext(context.Background(), "worker-a", map[string]string{"team": "platform"}, nil)
	if err != nil || labels.Labels["team"] != "platform" {
		t.Fatalf("UpdateNodeLabels = %#v, %v", labels, err)
	}
	if err := service.RemoveNodeContext(context.Background(), "worker-a"); err != nil {
		t.Fatalf("RemoveNode: %v", err)
	}
	if adapter.deletedNode != "worker-a" {
		t.Fatalf("deleted node = %q", adapter.deletedNode)
	}
	updated, err := st.GetServer(server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ClusterRole != "" || updated.K8sNodeName != "" {
		t.Fatalf("server binding was not cleared: %#v", updated)
	}
}

func TestServiceManagesServerLifecycleWithoutHTTPTypes(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(st, nil)
	server := &model.Server{Name: "worker", Host: "10.0.0.11", SSHPort: 22, SSHUser: "root"}
	if err := service.CreateServer(server); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	server.Name = "worker-renamed"
	if err := service.UpdateServer(server); err != nil {
		t.Fatalf("UpdateServer: %v", err)
	}
	updated, err := service.UnbindServer(server.ID)
	if err != nil {
		t.Fatalf("UnbindServer: %v", err)
	}
	if updated.Name != "worker-renamed" || updated.Host != "10.0.0.11" {
		t.Fatalf("unbind changed server identity: %#v", updated)
	}
	if err := service.DeleteServer(server.ID); err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}
	if _, err := service.GetServer(server.ID); err == nil {
		t.Fatal("deleted server is still returned")
	}
}

func TestServiceDelegatesNodeOperationsAndValidatesForceDrainConfirmation(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	adapter := &fakeNodeAdapter{
		getLabels:   &k8s.NodeLabels{Name: "worker-a"},
		drainPlan:   &k8s.DrainPlan{NodeName: "worker-a"},
		drainResult: &k8s.DrainResult{Plan: &k8s.DrainPlan{NodeName: "worker-a"}},
		rejoined:    &k8s.NodeInfo{Name: "worker-a"},
		removal:     &k8s.NodeRemovalCheck{NodeName: "worker-a", CanRemove: true},
	}
	service := NewService(st, adapter)

	if _, err := service.ListNodesContext(context.Background()); err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if _, err := service.GetNodeLabelsContext(context.Background(), "worker-a"); err != nil {
		t.Fatalf("GetNodeLabels: %v", err)
	}
	if _, err := service.GetDrainPlanContext(context.Background(), "worker-a"); err != nil {
		t.Fatalf("GetDrainPlan: %v", err)
	}
	if _, err := service.DrainNodeContext(context.Background(), "worker-a", k8s.DrainOptions{DeleteEmptyDirData: true}); err != nil {
		t.Fatalf("DrainNode: %v", err)
	}
	if !adapter.lastDrain.DeleteEmptyDirData {
		t.Fatal("drain option was not forwarded")
	}
	if _, err := service.ForceDrainNodeContext(context.Background(), "worker-a", k8s.ForceDrainOptions{}); err == nil {
		t.Fatal("ForceDrainNode accepted missing confirmation")
	}
	if _, err := service.ForceDrainNodeContext(context.Background(), "worker-a", k8s.ForceDrainOptions{AcknowledgeRisk: true, ConfirmNodeName: "worker-a"}); err != nil {
		t.Fatalf("ForceDrainNode: %v", err)
	}
	if _, err := service.RejoinNodeContext(context.Background(), "worker-a"); err != nil {
		t.Fatalf("RejoinNode: %v", err)
	}
	if _, err := service.GetRemovalCheckContext(context.Background(), "worker-a"); err != nil {
		t.Fatalf("GetRemovalCheck: %v", err)
	}
}

func TestServicePreservesServerBindingsWhenNodeLookupFails(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	server := &model.Server{Name: "worker", Host: "10.0.0.11", ClusterRole: "worker", K8sNodeName: "worker-a"}
	if err := st.CreateServer(server); err != nil {
		t.Fatal(err)
	}
	service := NewService(st, &fakeNodeAdapter{listErr: errors.New("cluster unavailable")})
	if _, err := service.ListServersContext(context.Background()); err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	stored, err := st.GetServer(server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.K8sNodeName != "worker-a" {
		t.Fatalf("transient lookup cleared binding: %#v", stored)
	}
}

func TestServiceRunsServerInspectionThroughInjectedAdapter(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	server := &model.Server{Name: "worker", Host: "10.0.0.11"}
	if err := st.CreateServer(server); err != nil {
		t.Fatal(err)
	}
	inspector := &fakeServerInspector{
		reachable:  false,
		probeError: "connection refused",
		checks: []Precheck{
			{Name: "ssh_connect", Label: "SSH 连接", Pass: false, Detail: "连接失败"},
			{Name: "disk_space", Label: "磁盘空间", Pass: false, Detail: "SSH 不可达，跳过"},
		},
	}
	service := NewService(st, nil).WithServerInspector(inspector)

	probe, err := service.ProbeServerContext(context.Background(), server.ID)
	if err != nil {
		t.Fatalf("ProbeServer: %v", err)
	}
	if probe.Reachable || probe.Error != "connection refused" || inspector.probed != server.ID {
		t.Fatalf("unexpected probe result: %#v, fake=%#v", probe, inspector)
	}
	precheck, err := service.PrecheckServerContext(context.Background(), server.ID)
	if err != nil {
		t.Fatalf("PrecheckServer: %v", err)
	}
	if precheck.AllPass || len(precheck.Checks) != 2 || inspector.checked != server.ID {
		t.Fatalf("unexpected precheck result: %#v, fake=%#v", precheck, inspector)
	}
}

func TestServiceRejectsInspectionWithoutInjectedAdapter(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	server := &model.Server{Name: "worker", Host: "10.0.0.11"}
	if err := st.CreateServer(server); err != nil {
		t.Fatal(err)
	}
	service := NewService(st, nil)
	if _, err := service.ProbeServerContext(context.Background(), server.ID); err == nil {
		t.Fatal("ProbeServer succeeded without an inspector")
	}
	if _, err := service.PrecheckServerContext(context.Background(), server.ID); err == nil {
		t.Fatal("PrecheckServer succeeded without an inspector")
	}
}

func TestServiceMatchesAndConfirmsImportedNode(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	server := &model.Server{Name: "worker", Host: "10.0.0.11"}
	if err := st.CreateServer(server); err != nil {
		t.Fatal(err)
	}
	service := NewService(st, &fakeNodeAdapter{nodes: []k8s.NodeInfo{{Name: "worker-a", Ready: true, Roles: "worker", InternalIP: "10.0.0.11", Version: "v1", OS: "linux"}}}).WithServerImporter(fakeServerImporter{hostname: "worker-a"})
	result, err := service.PreImportServerContext(context.Background(), server.ID)
	if err != nil {
		t.Fatalf("PreImportServer: %v", err)
	}
	if result.NodeName != "worker-a" || result.Role != "worker" {
		t.Fatalf("unexpected import result: %#v", result)
	}
	confirmed, err := service.ConfirmImport(server.ID, result.NodeName, result.Role)
	if err != nil {
		t.Fatalf("ConfirmImport: %v", err)
	}
	if confirmed.NodeName != "worker-a" {
		t.Fatalf("unexpected confirmation: %#v", confirmed)
	}
	stored, err := st.GetServer(server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.K8sNodeName != "worker-a" || stored.ClusterRole != "worker" {
		t.Fatalf("import binding not persisted: %#v", stored)
	}
}

func TestServiceCollectsResourceStatsWithServerMetadata(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateServer(&model.Server{Name: "worker", Host: "10.0.0.11"}); err != nil {
		t.Fatal(err)
	}
	service := NewService(st, nil).WithMetricsInspector(fakeMetricsInspector{})
	results, err := service.ResourceStatsContext(context.Background())
	if err != nil {
		t.Fatalf("ResourceStats: %v", err)
	}
	if len(results) != 1 || results[0]["server_name"] != "worker" || results[0]["status"] != "ready" || results[0]["cpu_percent"] != 12.5 {
		t.Fatalf("unexpected resource stats: %#v", results)
	}
	stats, err := service.ServerStatsContext(context.Background(), 1)
	if err != nil || stats["cpu_percent"] != 12.5 {
		t.Fatalf("unexpected single-server stats: %#v, %v", stats, err)
	}
}
