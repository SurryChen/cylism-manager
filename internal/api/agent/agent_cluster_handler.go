package agent

import (
	"net/http"

	"github.com/cylism/cylism-manager/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CapabilityStatus exposes the authenticated Runtime's effective capability scopes.
func (h *AgentHandler) CapabilityStatus(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	grants, err := h.store.ListAgentCapabilityGrants(instance.ID)
	if err != nil {
		writeAgentError(w, http.StatusInternalServerError, "capability status unavailable", true)
		return
	}
	data := make(map[string]map[string]any, len(model.AgentCapabilities))
	for capability := range model.AgentCapabilities {
		data[capability] = map[string]any{"enabled": false, "namespaces": []string{}, "approval_required": capability == model.AgentCapabilityDeploymentScale || capability == model.AgentCapabilityRegistryPullCheck || capability == model.AgentCapabilityMaintenanceCleanup}
	}
	for _, grant := range grants {
		if !grant.Enabled {
			continue
		}
		status := data[grant.Capability]
		if status == nil {
			continue
		}
		status["enabled"] = true
		if grant.Namespace == "*" {
			status["scope"] = "cluster"
			status["namespaces"] = []string{"*"}
			continue
		}
		if status["scope"] == "cluster" {
			continue
		}
		status["namespaces"] = append(status["namespaces"].([]string), grant.Namespace)
	}
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: data, Summary: "capability status retrieved"})
}

func (h *AgentHandler) ClusterStatus(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	granted, err := h.store.HasAgentCapability(instance.ID, model.AgentCapabilityClusterRead, "")
	if err != nil {
		writeAgentError(w, http.StatusInternalServerError, "capability check failed", true)
		return
	}
	if !granted {
		h.audit(instance, "agent.denied", map[string]string{"capability": model.AgentCapabilityClusterRead, "reason": "not_granted"})
		writeAgentError(w, http.StatusForbidden, "capability not granted", false)
		return
	}
	if h.client == nil || !h.client.KubernetesAvailable() {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	nodes, err := h.client.Clientset().CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "cluster status unavailable", true)
		return
	}
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: map[string]any{"node_count": len(nodes.Items)}, Summary: "cluster status retrieved"})
}
