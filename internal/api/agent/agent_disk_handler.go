package agent

import (
	"net/http"
	"strings"

	"github.com/cylism/cylism-manager/internal/model"
)

// MaintenanceDiskInspect is the Agent-facing HTTP adapter for node disk
// inspection. The SSH probe and parser are owned by service/maintenance.
func (h *AgentHandler) MaintenanceDiskInspect(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok || !h.requireCapability(w, instance, model.AgentCapabilityMaintenanceInspect, "") {
		return
	}
	node := strings.TrimSpace(r.URL.Query().Get("node"))
	if node == "" || !validAgentResourceName(node) {
		writeAgentError(w, http.StatusBadRequest, "invalid node", false)
		return
	}
	servers, err := h.store.ListServers()
	if err != nil {
		writeAgentError(w, http.StatusInternalServerError, "managed nodes unavailable", true)
		return
	}
	server, found := agentServerForNode(node, servers)
	if !found {
		writeAgentError(w, http.StatusNotFound, "managed node not found", false)
		return
	}
	if h.maintenanceInspector == nil {
		writeAgentError(w, http.StatusServiceUnavailable, "node disk inspection unavailable", true)
		return
	}
	inspection, err := h.maintenanceInspector.Inspect(r.Context(), server)
	if err != nil {
		summary := redactAgentText(truncateAgentText(strings.TrimSpace(err.Error()), 512))
		if summary == "" {
			summary = "node disk inspection failed"
		}
		writeAgentError(w, http.StatusBadGateway, summary, true)
		return
	}
	inspection.Node = node
	h.audit(instance, "agent.maintenance_disk_inspect", map[string]string{"capability": model.AgentCapabilityMaintenanceInspect, "node": node})
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: inspection, Summary: "disk inspection retrieved"})
}
