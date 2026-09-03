package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

type agentAuthenticatorStub struct {
	instance *model.RuntimeInstance
	err      error
}

func TestRedactAgentTextRemovesAssignmentAndAuthorizationCredentials(t *testing.T) {
	value := redactAgentText("token=abc123 Authorization: Bearer secret-value password: hidden")
	if strings.Contains(value, "abc123") || strings.Contains(value, "secret-value") || strings.Contains(value, "hidden") || !strings.Contains(value, "[REDACTED]") {
		t.Fatalf("agent log redaction leaked a credential: %q", value)
	}
}

func (stub agentAuthenticatorStub) AuthenticateAgent(_ context.Context, _ string) (*model.RuntimeInstance, error) {
	return stub.instance, stub.err
}
