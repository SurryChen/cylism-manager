package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	maintenance "github.com/cylism/cylism-manager/internal/service/maintenance"
)

type maintenanceCleanupRequest struct {
	AlertID uint   `json:"alert_id"`
	Recipe  string `json:"recipe"`
}
type maintenanceCleanupParameters struct {
	AlertID uint   `json:"alert_id"`
	Node    string `json:"node"`
	Recipe  string `json:"recipe"`
}

func (h *AgentHandler) MaintenanceCleanupRequest(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok || !h.requireCapability(w, instance, model.AgentCapabilityMaintenanceCleanup, "") {
		return
	}
	var request maintenanceCleanupRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&request); err != nil || request.AlertID == 0 || !maintenance.ValidCleanupRecipe(request.Recipe) {
		writeAgentError(w, http.StatusBadRequest, "invalid maintenance cleanup request", false)
		return
	}
	event, err := h.store.GetAlertEvent(request.AlertID)
	if err != nil || event.Status == model.AlertEventResolved || strings.TrimSpace(event.NodeName) == "" {
		writeAgentError(w, http.StatusConflict, "alert event is not eligible for cleanup", false)
		return
	}
	policy, err := h.store.GetAlertAutomationPolicy()
	if err != nil || !policy.Enabled || policy.Mode != model.AlertAutomationApproval || policy.RuntimeID != instance.ID || event.RuntimeID == nil || *event.RuntimeID != instance.ID {
		writeAgentError(w, http.StatusForbidden, "cleanup request is not enabled for this alert runtime", false)
		return
	}
	parameters, _ := json.Marshal(maintenanceCleanupParameters{AlertID: event.ID, Node: event.NodeName, Recipe: request.Recipe})
	requestID := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if requestID == "" || len(requestID) > 128 {
		writeAgentError(w, http.StatusBadRequest, "missing idempotency key", false)
		return
	}
	operation := &model.AgentOperation{OperationID: newAgentOperationID(), RuntimeID: instance.ID, Capability: model.AgentCapabilityMaintenanceCleanup, RequestID: requestID, ChatSessionID: r.Header.Get("X-Chat-Session-ID"), Parameters: string(parameters), ParametersHash: fmt.Sprintf("%x", sha256.Sum256(parameters)), Status: model.AgentOperationPendingApproval, Summary: maintenance.CleanupSummary(event.NodeName, request.Recipe), ExpiresAt: time.Now().Add(15 * time.Minute)}
	stored, _, err := h.store.CreateAgentOperation(operation)
	if err != nil {
		writeAgentError(w, http.StatusInternalServerError, "unable to create cleanup approval", true)
		return
	}
	event.OperationID, event.Status, event.DiagnosticSummary = stored.OperationID, model.AlertEventAwaitingApproval, "等待管理员审批固定清理配方"
	_ = h.store.UpdateAlertEvent(event)
	h.audit(instance, "agent.maintenance_cleanup_requested", map[string]string{"capability": model.AgentCapabilityMaintenanceCleanup, "alert_id": strconv.Itoa(int(event.ID)), "recipe": request.Recipe, "request_id": requestID, "operation_id": stored.OperationID})
	writeAgentResponse(w, http.StatusAccepted, agentAPIResponse{Status: "pending_approval", OperationID: stored.OperationID, Summary: "maintenance cleanup requires approval"})
}

func validMaintenanceRecipe(recipe string) bool { return maintenance.ValidCleanupRecipe(recipe) }

type deploymentScaleRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Replicas  int32  `json:"replicas"`
}

const agentLogLimit = 32 * 1024
