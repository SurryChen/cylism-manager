package agent

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestAgentOperationListFiltersAndDoesNotExposeExecutionParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	instance := &model.RuntimeInstance{Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged, Namespace: "cylism-assistant", Image: "example/nanobot", Status: model.RuntimeStatusReady}
	if err := s.CreateRuntime(instance); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	for _, operation := range []*model.AgentOperation{
		{OperationID: "op_pending", RuntimeID: instance.ID, Capability: model.AgentCapabilityDeploymentScale, RequestID: "request_pending", ChatSessionID: "chat-a", Parameters: `{"token":"must-not-leak"}`, ParametersHash: "hash", Status: model.AgentOperationPendingApproval, Summary: "scale api", ExpiresAt: time.Now().Add(time.Hour)},
		{OperationID: "op_done", RuntimeID: instance.ID, Capability: model.AgentCapabilityDeploymentScale, RequestID: "request_done", ChatSessionID: "chat-b", Parameters: `{"password":"must-not-leak"}`, ParametersHash: "hash", Status: model.AgentOperationSucceeded, Summary: "scale worker", ExpiresAt: time.Now().Add(time.Hour)},
	} {
		if _, _, err := s.CreateAgentOperation(operation); err != nil {
			t.Fatalf("create operation: %v", err)
		}
	}
	handler := newTestAgentOperationHandler(s, nil)
	router := gin.New()
	router.GET("/runtimes/:id/agent-operations", handler.ListOperations)
	response := serve(router, newJSONRequest(http.MethodGet, "/runtimes/"+itoa(instance.ID)+"/agent-operations?status=pending_approval&session_id=chat-a", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("list: %d %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, "op_pending") || strings.Contains(body, "op_done") || strings.Contains(body, "must-not-leak") || strings.Contains(body, `"parameters"`) {
		t.Fatalf("unexpected operation summary: %s", body)
	}
}

func TestAgentOperationErrorSummaryRedactsAndBoundsExecutorDetails(t *testing.T) {
	detail := agentOperationErrorSummary("节点验证镜像拉取失败", fmt.Errorf("rpc failed: token=abc123 Authorization: Bearer secret-value password: hidden %s", strings.Repeat("x", 600)))
	if strings.Contains(detail, "abc123") || strings.Contains(detail, "secret-value") || strings.Contains(detail, "hidden") {
		t.Fatalf("operation error summary leaked a credential: %q", detail)
	}
	if !strings.Contains(detail, "节点验证镜像拉取失败") || !strings.Contains(detail, "[REDACTED]") || len(detail) > agentOperationErrorSummaryLimit {
		t.Fatalf("unexpected operation error summary: len=%d value=%q", len(detail), detail)
	}
}

func TestValidAgentOperationStatus(t *testing.T) {
	for _, status := range []string{"pending_approval", "approved", "rejected", "succeeded", "failed", "stale", "expired"} {
		if !validAgentOperationStatus(status) {
			t.Errorf("status %q should be valid", status)
		}
	}
	for _, status := range []string{"", "pending", "unknown", " APPROVED"} {
		if validAgentOperationStatus(status) {
			t.Errorf("status %q should be invalid", status)
		}
	}
}
