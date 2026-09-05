package model

import "testing"

func TestAgentCapabilityAndOperationStatus(t *testing.T) {
	if !ValidAgentCapability(AgentCapabilityWorkloadRead) || ValidAgentCapability("unknown.capability") {
		t.Fatal("agent capability validation is incorrect")
	}
	if !AgentOperationTerminal(AgentOperationSucceeded) || AgentOperationTerminal(AgentOperationApproved) {
		t.Fatal("agent operation terminal status classification is incorrect")
	}
}
