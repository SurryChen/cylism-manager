package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
)

func TestAgentAlertEndpointsSeparateAlertAndAutomationState(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	instance := &model.RuntimeInstance{Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged, Namespace: "cylism-assistant", Image: "example/nanobot", Status: model.RuntimeStatusReady, AgentToolEnabled: true}
	if err := s.CreateRuntime(instance); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	if err := s.ReplaceAgentCapabilityGrants(instance.ID, []model.AgentCapabilityGrant{{RuntimeID: instance.ID, Capability: model.AgentCapabilityAlertRead, Namespace: "*", Enabled: true}}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	event, err := s.UpsertAlertEvent(&model.AlertEvent{Fingerprint: "alert-1", AlertName: "NodeDiskHigh", Severity: "warning", NodeName: "node-a", Labels: `{}`, Annotations: `{}`, Status: model.AlertEventFiring, StartsAt: time.Now().UTC()})
	if err != nil {
		t.Fatalf("create alert event: %v", err)
	}
	event.Status = model.AlertEventAnalyzing
	if err := s.UpdateAlertEvent(event); err != nil {
		t.Fatalf("mark alert analyzing: %v", err)
	}
	handler := newTestAgentHandler(s, nil, agentAuthenticatorStub{instance: instance})

	getRequest := httptest.NewRequest(http.MethodGet, "/api/agent/v1/alerts/get?id="+itoa(event.ID), nil)
	getRequest.Header.Set("Authorization", "Bearer agent-token")
	getResponse := httptest.NewRecorder()
	handler.AlertGet(getResponse, getRequest)
	if getResponse.Code != http.StatusOK || !strings.Contains(getResponse.Body.String(), `"automation_status":"analyzing"`) || !strings.Contains(getResponse.Body.String(), `"alert_state":"firing"`) {
		t.Fatalf("expected distinct alert states, got %d: %s", getResponse.Code, getResponse.Body.String())
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/agent/v1/alerts/list", nil)
	listRequest.Header.Set("Authorization", "Bearer agent-token")
	listResponse := httptest.NewRecorder()
	handler.AlertList(listResponse, listRequest)
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), `"automation_status":"analyzing"`) || !strings.Contains(listResponse.Body.String(), `"alert_state":"firing"`) {
		t.Fatalf("expected distinct list states, got %d: %s", listResponse.Code, listResponse.Body.String())
	}
}
