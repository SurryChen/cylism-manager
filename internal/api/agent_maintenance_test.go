package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
)

func TestParseAgentDiskInspectionProducesBoundedStructuredData(t *testing.T) {
	output := `__CYLISM_FILESYSTEMS__
/ 42949672960 35567697920 7381975040 83%
/var/lib/containerd 21474836480 10737418240 10737418240 50%
__CYLISM_JOURNAL__
Archived and active journals take up 2.5G in the file system.
__CYLISM_DIR__:/var/log
2147483648 /var/log
1610612736 /var/log/journal
536870912 /var/log/pods
__CYLISM_DIR__:/var/lib/rancher/k3s
4294967296 /var/lib/rancher/k3s
3221225472 /var/lib/rancher/k3s/agent
`

	inspection, err := parseAgentDiskInspection(output)
	if err != nil {
		t.Fatalf("parse inspection: %v", err)
	}
	if len(inspection.Filesystems) != 2 || inspection.Filesystems[0].MountPoint != "/" || inspection.Filesystems[0].UsedPercent != 83 {
		t.Fatalf("unexpected filesystems: %#v", inspection.Filesystems)
	}
	if inspection.JournalBytes != 2684354560 {
		t.Fatalf("unexpected journal bytes: %d", inspection.JournalBytes)
	}
	if len(inspection.Directories) != 2 || inspection.Directories[0].Path != "/var/log" || len(inspection.Directories[0].Entries) != 2 {
		t.Fatalf("unexpected directory data: %#v", inspection.Directories)
	}
}

func TestAgentDiskInspectionCommandIsFixedAndBounded(t *testing.T) {
	command := agentDiskInspectionCommand()
	for _, path := range agentDiskInspectionPaths {
		if !strings.Contains(command, path) {
			t.Fatalf("missing allowlisted path %q: %s", path, command)
		}
	}
	if strings.Contains(command, "$CYLISM_") || !strings.Contains(command, "timeout 12s") || !strings.Contains(command, "head -n 20") {
		t.Fatalf("inspection command must stay fixed and bounded: %s", command)
	}
}

func TestParseAgentDiskInspectionRejectsMissingFilesystemSection(t *testing.T) {
	if _, err := parseAgentDiskInspection("__CYLISM_JOURNAL__\nnone\n"); err == nil {
		t.Fatal("expected malformed inspection to fail")
	}
}

func TestMaintenanceDiskInspectRequiresExplicitGrantAndRedactsFailures(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	instance := &model.RuntimeInstance{Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged, Namespace: "cylism-assistant", Image: "example/nanobot", Status: model.RuntimeStatusReady, AgentToolEnabled: true}
	if err := s.CreateRuntime(instance); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	if err := s.CreateServer(&model.Server{Name: "node-1", Host: "10.0.0.1", SSHUser: "root", SSHAuthType: "key", K8sNodeName: "node-1"}); err != nil {
		t.Fatalf("create server: %v", err)
	}
	called := 0
	handler := NewAgentHandler(s, nil, agentAuthenticatorStub{instance: instance}).WithMaintenanceInspector(func(server *model.Server) (agentDiskInspection, error) {
		called++
		if server.K8sNodeName != "node-1" {
			return agentDiskInspection{}, fmt.Errorf("unexpected node")
		}
		return agentDiskInspection{Filesystems: []agentFilesystemUsage{{MountPoint: "/", UsedPercent: 83}}, Directories: []agentDiskDirectoryUsage{}}, nil
	})
	request := httptest.NewRequest(http.MethodGet, "/api/agent/v1/maintenance/disk-inspect?node=node-1", nil)
	request.Header.Set("Authorization", "Bearer agent-token")
	recorder := httptest.NewRecorder()
	handler.MaintenanceDiskInspect(recorder, request)
	if recorder.Code != http.StatusForbidden || called != 0 {
		t.Fatalf("expected ungranted inspection to be denied: %d %s", recorder.Code, recorder.Body.String())
	}
	if err := s.ReplaceAgentCapabilityGrants(instance.ID, []model.AgentCapabilityGrant{{RuntimeID: instance.ID, Capability: model.AgentCapabilityMaintenanceInspect, Namespace: "*", Enabled: true}}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	recorder = httptest.NewRecorder()
	handler.MaintenanceDiskInspect(recorder, request)
	if recorder.Code != http.StatusOK || called != 1 || !strings.Contains(recorder.Body.String(), `"used_percent":83`) {
		t.Fatalf("unexpected inspection response: %d %s", recorder.Code, recorder.Body.String())
	}
	failing := NewAgentHandler(s, nil, agentAuthenticatorStub{instance: instance}).WithMaintenanceInspector(func(*model.Server) (agentDiskInspection, error) {
		return agentDiskInspection{}, fmt.Errorf("token=secret remote inspection failed")
	})
	recorder = httptest.NewRecorder()
	failing.MaintenanceDiskInspect(recorder, request)
	if recorder.Code != http.StatusBadGateway || strings.Contains(recorder.Body.String(), "secret") {
		t.Fatalf("unsafe inspection error: %d %s", recorder.Code, recorder.Body.String())
	}
}
