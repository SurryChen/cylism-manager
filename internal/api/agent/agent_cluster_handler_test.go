package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestAgentHandlerClusterStatusRequiresCapabilityGrant(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	instance := &model.RuntimeInstance{ID: 0, Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged, Namespace: "cylism-assistant", Image: "example/nanobot", Status: model.RuntimeStatusReady, AgentToolEnabled: true}
	if err := s.CreateRuntime(instance); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	client := &k8s.Client{Clientset: fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}})}
	handler := newTestAgentHandler(s, client, agentAuthenticatorStub{instance: instance})

	request := httptest.NewRequest(http.MethodGet, "/api/agent/v1/cluster/status", nil)
	request.Header.Set("Authorization", "Bearer agent-token")
	recorder := httptest.NewRecorder()
	handler.ClusterStatus(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected deny without grant, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if err := s.ReplaceAgentCapabilityGrants(instance.ID, []model.AgentCapabilityGrant{{RuntimeID: instance.ID, Capability: model.AgentCapabilityClusterRead, Namespace: "*", Enabled: true}}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	recorder = httptest.NewRecorder()
	handler.ClusterStatus(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"ok"`) || !strings.Contains(recorder.Body.String(), `"node_count":1`) {
		t.Fatalf("expected granted response, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentHandlerCapabilityStatusReturnsEffectiveScopes(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	instance := &model.RuntimeInstance{Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged, Namespace: "cylism-assistant", Image: "example/nanobot", Status: model.RuntimeStatusReady, AgentToolEnabled: true}
	if err := s.CreateRuntime(instance); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	if err := s.ReplaceAgentCapabilityGrants(instance.ID, []model.AgentCapabilityGrant{
		{RuntimeID: instance.ID, Capability: model.AgentCapabilityClusterRead, Namespace: "*", Enabled: true},
		{RuntimeID: instance.ID, Capability: model.AgentCapabilityWorkloadRead, Namespace: "operations", Enabled: true},
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	handler := newTestAgentHandler(s, &k8s.Client{Clientset: fake.NewSimpleClientset()}, agentAuthenticatorStub{instance: instance})
	request := httptest.NewRequest(http.MethodGet, "/api/agent/v1/capabilities/status", nil)
	request.Header.Set("Authorization", "Bearer agent-token")
	recorder := httptest.NewRecorder()
	handler.CapabilityStatus(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"scope":"cluster"`) || !strings.Contains(recorder.Body.String(), `"operations"`) {
		t.Fatalf("unexpected capability status: %d %s", recorder.Code, recorder.Body.String())
	}
}
