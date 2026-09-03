package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	maintenance "github.com/cylism/cylism-manager/internal/service/maintenance"
	"github.com/cylism/cylism-manager/internal/store"
)

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
	handler := newTestAgentHandler(s, nil, agentAuthenticatorStub{instance: instance}).WithMaintenanceInspector(maintenance.InspectorFunc(func(_ context.Context, server *model.Server) (maintenance.Inspection, error) {
		called++
		if server.K8sNodeName != "node-1" {
			return maintenance.Inspection{}, fmt.Errorf("unexpected node")
		}
		return maintenance.Inspection{Filesystems: []maintenance.FilesystemUsage{{MountPoint: "/", UsedPercent: 83}}, Directories: []maintenance.DirectoryUsage{}}, nil
	}))
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
	failing := newTestAgentHandler(s, nil, agentAuthenticatorStub{instance: instance}).WithMaintenanceInspector(maintenance.InspectorFunc(func(context.Context, *model.Server) (maintenance.Inspection, error) {
		return maintenance.Inspection{}, fmt.Errorf("token=secret remote inspection failed")
	}))
	recorder = httptest.NewRecorder()
	failing.MaintenanceDiskInspect(recorder, request)
	if recorder.Code != http.StatusBadGateway || strings.Contains(recorder.Body.String(), "secret") {
		t.Fatalf("unsafe inspection error: %d %s", recorder.Code, recorder.Body.String())
	}
}
