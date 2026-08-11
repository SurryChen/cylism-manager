package store

import (
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestAgentCapabilityGrantsAreDenyByDefaultAndNamespaceScoped(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	if granted, err := s.HasAgentCapability(3, model.AgentCapabilityWorkloadRead, "operations"); err != nil || granted {
		t.Fatalf("expected deny by default, granted=%t err=%v", granted, err)
	}
	grants := []model.AgentCapabilityGrant{
		{RuntimeID: 3, Capability: model.AgentCapabilityWorkloadRead, Namespace: "operations", Enabled: true},
		{RuntimeID: 3, Capability: model.AgentCapabilityClusterRead, Namespace: "*", Enabled: true},
	}
	if err := s.ReplaceAgentCapabilityGrants(3, grants); err != nil {
		t.Fatalf("replace grants: %v", err)
	}
	if granted, _ := s.HasAgentCapability(3, model.AgentCapabilityWorkloadRead, "operations"); !granted {
		t.Fatal("expected namespace grant")
	}
	if granted, _ := s.HasAgentCapability(3, model.AgentCapabilityWorkloadRead, "other"); granted {
		t.Fatal("unexpected grant outside namespace")
	}
	if granted, _ := s.HasAgentCapability(3, model.AgentCapabilityClusterRead, ""); !granted {
		t.Fatal("expected cluster-wide grant")
	}
}

func TestAgentOperationIsIdempotentAndTerminalStatusCannotBeRewritten(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	operation := &model.AgentOperation{OperationID: "op_123", RuntimeID: 3, Capability: model.AgentCapabilityDeploymentScale, RequestID: "request_123", Parameters: `{"name":"api","namespace":"operations","replicas":3}`, ParametersHash: "abc", Status: model.AgentOperationPendingApproval, Summary: "scale deployment", ExpiresAt: time.Now().Add(time.Hour)}
	stored, created, err := s.CreateAgentOperation(operation)
	if err != nil || !created || stored.ID == 0 {
		t.Fatalf("create operation: created=%t operation=%+v err=%v", created, stored, err)
	}
	retry := &model.AgentOperation{OperationID: "op_other", RuntimeID: 3, Capability: model.AgentCapabilityDeploymentScale, RequestID: "request_123", Parameters: `{"name":"api","namespace":"operations","replicas":3}`, ParametersHash: "abc", Status: model.AgentOperationPendingApproval, Summary: "scale deployment", ExpiresAt: time.Now().Add(time.Hour)}
	stored, created, err = s.CreateAgentOperation(retry)
	if err != nil || created || stored.OperationID != "op_123" {
		t.Fatalf("retry must return original operation: created=%t operation=%+v err=%v", created, stored, err)
	}
	if changed, err := s.UpdateAgentOperationStatus("op_123", model.AgentOperationPendingApproval, model.AgentOperationRejected, "rejected", nil, nil); err != nil || !changed {
		t.Fatalf("reject operation: changed=%t err=%v", changed, err)
	}
	if changed, err := s.UpdateAgentOperationStatus("op_123", model.AgentOperationPendingApproval, model.AgentOperationApproved, "", nil, nil); err != nil || changed {
		t.Fatalf("terminal operation must not transition: changed=%t err=%v", changed, err)
	}
}
