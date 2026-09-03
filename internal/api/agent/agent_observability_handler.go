package agent

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

func (h *AgentHandler) ApprovalGet(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	operationID := strings.TrimPrefix(r.URL.Path, "/api/agent/v1/approvals/")
	if !validAgentOperationID(operationID) {
		writeAgentError(w, http.StatusBadRequest, "invalid operation id", false)
		return
	}
	operation, err := h.store.GetAgentOperation(operationID)
	if err != nil || operation.RuntimeID != instance.ID {
		writeAgentError(w, http.StatusNotFound, "operation not found", false)
		return
	}
	if operation.Status == model.AgentOperationPendingApproval && time.Now().After(operation.ExpiresAt) {
		writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: model.AgentOperationExpired, OperationID: operation.OperationID, Summary: "operation approval expired"})
		return
	}
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: operation.Status, OperationID: operation.OperationID, Summary: operationSummary(operation)})
}

// AlertGet exposes a persisted alert by numeric ID. Alert payload labels are
// diagnostic data; the endpoint intentionally does not expose any secret or
// webhook configuration.
func (h *AgentHandler) AlertGet(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok || !h.requireCapability(w, instance, model.AgentCapabilityAlertRead, "") {
		return
	}
	id, err := strconv.ParseUint(strings.TrimSpace(r.URL.Query().Get("id")), 10, 64)
	if err != nil || id == 0 {
		writeAgentError(w, http.StatusBadRequest, "invalid alert id", false)
		return
	}
	event, err := h.store.GetAlertEvent(uint(id))
	if err != nil {
		writeAgentError(w, http.StatusNotFound, "alert event not found", false)
		return
	}
	h.audit(instance, "agent.alert_get", map[string]string{"capability": model.AgentCapabilityAlertRead, "alert_id": strconv.FormatUint(id, 10)})
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: agentAlertEventData(event), Summary: "alert event retrieved"})
}

// AlertList lets an authorized Runtime discover recent persisted events before
// selecting one with AlertGet. It is deliberately fixed to a small recent
// window and does not permit filtering expressions or arbitrary database reads.
func (h *AgentHandler) AlertList(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok || !h.requireCapability(w, instance, model.AgentCapabilityAlertRead, "") {
		return
	}
	events, err := h.store.ListAlertEvents(20)
	if err != nil {
		writeAgentError(w, http.StatusInternalServerError, "alert events unavailable", true)
		return
	}
	items := make([]map[string]any, 0, len(events))
	for _, event := range events {
		automationStatus, alertState := agentAlertEventStates(&event)
		items = append(items, map[string]any{"id": event.ID, "alert_name": event.AlertName, "severity": event.Severity, "node": event.NodeName, "mount_point": event.MountPoint, "status": automationStatus, "automation_status": automationStatus, "alert_state": alertState, "starts_at": event.StartsAt, "updated_at": event.UpdatedAt, "diagnostic_summary": event.DiagnosticSummary, "operation_id": event.OperationID})
	}
	h.audit(instance, "agent.alert_list", map[string]string{"capability": model.AgentCapabilityAlertRead})
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: map[string]any{"events": items}, Summary: "recent alert events retrieved"})
}

// agentAlertEventData distinguishes the durable automation workflow from the
// underlying Alertmanager condition. status remains as a compatibility alias
// for automation_status; agents must use alert_state for remediation decisions.
func agentAlertEventData(event *model.AlertEvent) map[string]any {
	automationStatus, alertState := agentAlertEventStates(event)
	return map[string]any{
		"id":                 event.ID,
		"alert_name":         event.AlertName,
		"severity":           event.Severity,
		"node":               event.NodeName,
		"mount_point":        event.MountPoint,
		"status":             automationStatus,
		"automation_status":  automationStatus,
		"alert_state":        alertState,
		"starts_at":          event.StartsAt,
		"updated_at":         event.UpdatedAt,
		"labels":             json.RawMessage(event.Labels),
		"annotations":        json.RawMessage(event.Annotations),
		"diagnostic_summary": event.DiagnosticSummary,
		"operation_id":       event.OperationID,
	}
}

func agentAlertEventStates(event *model.AlertEvent) (automationStatus, alertState string) {
	automationStatus = event.Status
	alertState = "firing"
	if event.EndsAt != nil || automationStatus == model.AlertEventResolved {
		alertState = "resolved"
	}
	return automationStatus, alertState
}

// MonitoringDiskGrowth makes one Manager-owned metrics query. The Runtime may
// select only a node and a bounded range; it cannot submit PromQL.
func (h *AgentHandler) MonitoringDiskGrowth(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok || !h.requireCapability(w, instance, model.AgentCapabilityMonitoringRead, "") {
		return
	}
	node := strings.TrimSpace(r.URL.Query().Get("node"))
	rangeName := strings.TrimSpace(r.URL.Query().Get("range"))
	if h.monitoringDiskGrowth == nil {
		writeAgentError(w, http.StatusServiceUnavailable, "monitoring data store unavailable", true)
		return
	}
	data, err := h.monitoringDiskGrowth.QueryNode(r.Context(), rangeName, node)
	if err != nil {
		status := http.StatusBadGateway
		if strings.HasPrefix(err.Error(), "invalid") || strings.HasPrefix(err.Error(), "range must") {
			status = http.StatusBadRequest
		}
		writeAgentError(w, status, err.Error(), status != http.StatusBadRequest)
		return
	}
	mounts := data["mounts"]
	h.audit(instance, "agent.monitoring_disk_growth", map[string]string{"capability": model.AgentCapabilityMonitoringRead, "node": node, "range": rangeName})
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: map[string]any{"node": node, "range": rangeName, "mounts": mounts}, Summary: "disk growth retrieved"})
}

// MaintenanceDiskInspect runs a Manager-owned, fixed read-only probe on one
// managed node. The Runtime can choose only a node name and cannot supply a
// command, directory, or timeout.
