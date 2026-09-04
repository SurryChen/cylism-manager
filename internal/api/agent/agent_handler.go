package agent

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	security "github.com/cylism/cylism-manager/internal/security"
	maintenance "github.com/cylism/cylism-manager/internal/service/maintenance"
	monitoringservice "github.com/cylism/cylism-manager/internal/service/observability/monitoring"
	registryservice "github.com/cylism/cylism-manager/internal/service/registry"
	corev1 "k8s.io/api/core/v1"
)

type AgentAuthenticator interface {
	AuthenticateAgent(context.Context, string) (*model.RuntimeInstance, error)
}

var (
	agentResourceNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	agentContainerPattern    = regexp.MustCompile(`^[A-Za-z0-9]([-_A-Za-z0-9.]*[A-Za-z0-9])?$`)
	agentRequestIDPattern    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	agentRegistryPattern     = regexp.MustCompile(`^[A-Za-z0-9](?:[-A-Za-z0-9.]*[A-Za-z0-9])?(?::[0-9]{1,5})?$`)
)

type AgentHandler struct {
	store                repository.AgentReadRepository
	client               KubernetesAdapter
	authenticator        AgentAuthenticator
	registryVerifier     registryservice.NodeVerifier
	maintenanceInspector maintenance.Inspector
	monitoringDiskGrowth *monitoringservice.AgentDiskGrowthService
}

func NewAgentHandlerWithKubernetesAdapter(store repository.AgentReadRepository, client KubernetesAdapter, authenticator AgentAuthenticator) *AgentHandler {
	return &AgentHandler{store: store, client: client, authenticator: authenticator}
}
func (h *AgentHandler) WithRegistryVerifier(verifier registryservice.NodeVerifier) *AgentHandler {
	h.registryVerifier = verifier
	return h
}
func (h *AgentHandler) WithMaintenanceInspector(inspector maintenance.Inspector) *AgentHandler {
	h.maintenanceInspector = inspector
	return h
}
func (h *AgentHandler) WithMonitoringDiskGrowth(service *monitoringservice.AgentDiskGrowthService) *AgentHandler {
	h.monitoringDiskGrowth = service
	return h
}

func (h *AgentHandler) requireCapability(w http.ResponseWriter, instance *model.RuntimeInstance, capability, namespace string) bool {
	granted, err := h.store.HasAgentCapability(instance.ID, capability, namespace)
	if err != nil {
		writeAgentError(w, http.StatusInternalServerError, "capability check failed", true)
		return false
	}
	if !granted {
		h.audit(instance, "agent.denied", map[string]string{"capability": capability, "namespace": namespace, "reason": "not_granted"})
		writeAgentError(w, http.StatusForbidden, "capability not granted", false)
		return false
	}
	return true
}
func (h *AgentHandler) audit(instance *model.RuntimeInstance, action string, detail map[string]string) {
	if h == nil || h.store == nil || instance == nil {
		return
	}
	encoded, err := json.Marshal(detail)
	if err == nil {
		_ = h.store.CreateAuditLog(&model.AuditLog{Action: action, ResourceType: "agent_runtime", ResourceID: instance.ID, Detail: string(encoded), CreatedAt: time.Now()})
	}
}
func validAgentName(value string) bool {
	return len(value) <= 63 && agentResourceNamePattern.MatchString(value)
}
func validAgentResourceName(value string) bool { return validAgentName(value) }
func validAgentNamespace(value string) bool    { return validAgentName(value) }
func validAgentContainer(value string) bool {
	return len(value) <= 63 && agentContainerPattern.MatchString(value)
}
func validAgentKind(value string) bool {
	return value == "deployment" || value == "statefulset" || value == "daemonset"
}
func validAgentInvolvedKind(value string) bool {
	return value == "pod" || value == "persistentvolumeclaim"
}
func validAgentRequestID(value string) bool { return agentRequestIDPattern.MatchString(value) }
func validAgentOperationID(value string) bool {
	return strings.HasPrefix(value, "op_") && validAgentRequestID(value)
}
func validAgentRegistry(value string) bool {
	value = strings.TrimSpace(value)
	return len(value) > 0 && len(value) <= 253 && agentRegistryPattern.MatchString(value)
}
func agentServerForNode(node string, servers []model.Server) (*model.Server, bool) {
	for i := range servers {
		if servers[i].K8sNodeName == node {
			return &servers[i], true
		}
	}
	return nil, false
}
func replicas(value *int32) int32 {
	if value == nil {
		return 0
	}
	return *value
}
func workloadSummary(name, namespace, kind, resourceVersion string, desired, ready int32) map[string]any {
	return map[string]any{"name": name, "namespace": namespace, "kind": kind, "resource_version": resourceVersion, "desired": desired, "ready": ready}
}
func podDiagnosticSummary(pod *corev1.Pod) map[string]any {
	conditions := make([]map[string]string, 0, len(pod.Status.Conditions))
	for _, condition := range pod.Status.Conditions {
		conditions = append(conditions, map[string]string{"type": string(condition.Type), "status": string(condition.Status), "reason": condition.Reason, "message": redactAgentText(truncateAgentText(condition.Message, 512))})
	}
	statuses := append(append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...), pod.Status.ContainerStatuses...)
	containers := make([]map[string]any, 0, len(statuses))
	for _, status := range statuses {
		entry := map[string]any{"name": status.Name, "ready": status.Ready, "restart_count": status.RestartCount, "image": status.Image}
		if status.State.Waiting != nil {
			entry["state"], entry["reason"], entry["message"] = "waiting", status.State.Waiting.Reason, redactAgentText(truncateAgentText(status.State.Waiting.Message, 512))
		} else if status.State.Terminated != nil {
			entry["state"], entry["reason"], entry["exit_code"] = "terminated", status.State.Terminated.Reason, status.State.Terminated.ExitCode
		} else {
			entry["state"] = "running"
		}
		containers = append(containers, entry)
	}
	volumes := make([]map[string]string, 0, len(pod.Spec.Volumes))
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			volumes = append(volumes, map[string]string{"name": volume.Name, "persistent_volume_claim": volume.PersistentVolumeClaim.ClaimName})
		}
	}
	return map[string]any{"name": pod.Name, "namespace": pod.Namespace, "phase": pod.Status.Phase, "node_name": pod.Spec.NodeName, "conditions": conditions, "containers": containers, "persistent_volume_claims": volumes}
}
func eventTime(event corev1.Event) time.Time {
	if !event.EventTime.IsZero() {
		return event.EventTime.Time
	}
	if !event.LastTimestamp.IsZero() {
		return event.LastTimestamp.Time
	}
	return event.CreationTimestamp.Time
}
func resourceListSummary(resources corev1.ResourceList) map[string]string {
	values := make(map[string]string, len(resources))
	for name, quantity := range resources {
		values[string(name)] = quantity.String()
	}
	return values
}
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func TruncateAgentText(value string, limit int) string { return security.Truncate(value, limit) }
func RedactAgentText(value string) string              { return security.Redact(value) }
func truncateAgentText(value string, limit int) string { return TruncateAgentText(value, limit) }
func redactAgentText(value string) string              { return RedactAgentText(value) }
func newAgentOperationID() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err == nil {
		return "op_" + base64.RawURLEncoding.EncodeToString(b)
	}
	return fmt.Sprintf("op_%d", time.Now().UnixNano())
}
func operationSummary(operation *model.AgentOperation) string {
	if operation.ErrorSummary != "" {
		return operation.ErrorSummary
	}
	return operation.Summary
}

type agentAPIResponse struct {
	Status      string `json:"status"`
	OperationID string `json:"operation_id,omitempty"`
	Data        any    `json:"data,omitempty"`
	Summary     string `json:"summary"`
	Retryable   bool   `json:"retryable"`
}

func writeAgentError(w http.ResponseWriter, status int, summary string, retryable bool) {
	writeAgentResponse(w, status, agentAPIResponse{Status: "error", Summary: summary, Retryable: retryable})
}
func writeAgentResponse(w http.ResponseWriter, status int, response agentAPIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}
func (h *AgentHandler) authenticate(w http.ResponseWriter, r *http.Request) (*model.RuntimeInstance, bool) {
	if h == nil || h.authenticator == nil {
		writeAgentError(w, http.StatusServiceUnavailable, "agent authentication unavailable", true)
		return nil, false
	}
	token, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		writeAgentError(w, http.StatusUnauthorized, "agent authentication required", false)
		return nil, false
	}
	instance, err := h.authenticator.AuthenticateAgent(r.Context(), token)
	if err != nil || instance == nil {
		writeAgentError(w, http.StatusUnauthorized, "agent authentication required", false)
		return nil, false
	}
	return instance, true
}
