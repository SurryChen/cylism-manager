package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AgentAuthenticator interface {
	AuthenticateAgent(ctx context.Context, token string) (*model.RuntimeInstance, error)
}

type AgentHandler struct {
	store interface {
		HasAgentCapability(runtimeID uint, capability, namespace string) (bool, error)
		ListAgentCapabilityGrants(runtimeID uint) ([]model.AgentCapabilityGrant, error)
		CreateAgentOperation(operation *model.AgentOperation) (*model.AgentOperation, bool, error)
		GetAgentOperation(operationID string) (*model.AgentOperation, error)
		CreateAuditLog(entry *model.AuditLog) error
	}
	client        *k8s.Client
	authenticator AgentAuthenticator
}

func NewAgentHandler(store interface {
	HasAgentCapability(runtimeID uint, capability, namespace string) (bool, error)
	ListAgentCapabilityGrants(runtimeID uint) ([]model.AgentCapabilityGrant, error)
	CreateAgentOperation(operation *model.AgentOperation) (*model.AgentOperation, bool, error)
	GetAgentOperation(operationID string) (*model.AgentOperation, error)
	CreateAuditLog(entry *model.AuditLog) error
}, client *k8s.Client, authenticator AgentAuthenticator) *AgentHandler {
	return &AgentHandler{store: store, client: client, authenticator: authenticator}
}

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
		data[capability] = map[string]any{"enabled": false, "namespaces": []string{}, "approval_required": capability == model.AgentCapabilityDeploymentScale}
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
	h.audit(instance, "agent.capability_status", map[string]string{"action": "read"})
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
	if h.client == nil || h.client.Clientset == nil {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	nodes, err := h.client.Clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "cluster status unavailable", true)
		return
	}
	h.audit(instance, "agent.cluster_status", map[string]string{"capability": model.AgentCapabilityClusterRead})
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: map[string]any{"node_count": len(nodes.Items)}, Summary: "cluster status retrieved"})
}

func (h *AgentHandler) WorkloadGet(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	namespace, kind, name := r.URL.Query().Get("namespace"), r.URL.Query().Get("kind"), r.URL.Query().Get("name")
	if !validAgentNamespace(namespace) || !validAgentKind(kind) || !validAgentName(name) {
		writeAgentError(w, http.StatusBadRequest, "invalid workload query", false)
		return
	}
	if !h.requireCapability(w, instance, model.AgentCapabilityWorkloadRead, namespace) {
		return
	}
	if h.client == nil || h.client.Clientset == nil {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	var data any
	var err error
	switch kind {
	case "deployment":
		object, getErr := h.client.Clientset.AppsV1().Deployments(namespace).Get(r.Context(), name, metav1.GetOptions{})
		err = getErr
		if object != nil {
			data = workloadSummary(object.Name, object.Namespace, kind, object.ResourceVersion, replicas(object.Spec.Replicas), object.Status.ReadyReplicas)
		}
	case "statefulset":
		object, getErr := h.client.Clientset.AppsV1().StatefulSets(namespace).Get(r.Context(), name, metav1.GetOptions{})
		err = getErr
		if object != nil {
			data = workloadSummary(object.Name, object.Namespace, kind, object.ResourceVersion, replicas(object.Spec.Replicas), object.Status.ReadyReplicas)
		}
	case "daemonset":
		object, getErr := h.client.Clientset.AppsV1().DaemonSets(namespace).Get(r.Context(), name, metav1.GetOptions{})
		err = getErr
		if object != nil {
			data = map[string]any{"name": object.Name, "namespace": object.Namespace, "kind": kind, "resource_version": object.ResourceVersion, "desired": object.Status.DesiredNumberScheduled, "ready": object.Status.NumberReady}
		}
	}
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "workload unavailable", true)
		return
	}
	h.audit(instance, "agent.workload_get", map[string]string{"capability": model.AgentCapabilityWorkloadRead, "namespace": namespace, "kind": kind, "name": name})
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: data, Summary: "workload retrieved"})
}

func (h *AgentHandler) WorkloadLogs(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	namespace, pod, container := r.URL.Query().Get("namespace"), r.URL.Query().Get("pod"), r.URL.Query().Get("container")
	tail, err := strconv.ParseInt(r.URL.Query().Get("tail"), 10, 64)
	if !validAgentNamespace(namespace) || !validAgentName(pod) || !validAgentContainer(container) || err != nil || tail < 1 || tail > 200 {
		writeAgentError(w, http.StatusBadRequest, "invalid workload logs query", false)
		return
	}
	if !h.requireCapability(w, instance, model.AgentCapabilityWorkloadLogs, namespace) {
		return
	}
	if h.client == nil || h.client.Clientset == nil {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	stream, err := h.client.Clientset.CoreV1().Pods(namespace).GetLogs(pod, &corev1.PodLogOptions{Container: container, TailLines: &tail}).Stream(r.Context())
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "workload logs unavailable", true)
		return
	}
	defer stream.Close()
	content, err := io.ReadAll(io.LimitReader(stream, agentLogLimit+1))
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "workload logs unavailable", true)
		return
	}
	truncated := len(content) > agentLogLimit
	if truncated {
		content = content[:agentLogLimit]
	}
	h.audit(instance, "agent.workload_logs", map[string]string{"capability": model.AgentCapabilityWorkloadLogs, "namespace": namespace, "pod": pod, "container": container})
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: map[string]any{"logs": redactAgentText(string(content)), "truncated": truncated}, Summary: "workload logs retrieved"})
}

func (h *AgentHandler) DeploymentScale(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	requestID := r.Header.Get("X-Request-ID")
	if !validAgentRequestID(requestID) || r.Header.Get("Idempotency-Key") != requestID {
		writeAgentError(w, http.StatusBadRequest, "matching request and idempotency keys are required", false)
		return
	}
	var request deploymentScaleRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4097))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF || !validAgentNamespace(request.Namespace) || !validAgentName(request.Name) || request.Replicas < 0 || request.Replicas > 50 {
		writeAgentError(w, http.StatusBadRequest, "invalid deployment scale request", false)
		return
	}
	if !h.requireCapability(w, instance, model.AgentCapabilityDeploymentScale, request.Namespace) {
		return
	}
	if h.client == nil || h.client.Clientset == nil {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	deployment, err := h.client.Clientset.AppsV1().Deployments(request.Namespace).Get(r.Context(), request.Name, metav1.GetOptions{})
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "deployment unavailable", true)
		return
	}
	parameters, _ := json.Marshal(request)
	hash := fmt.Sprintf("%x", sha256.Sum256(parameters))
	operation := &model.AgentOperation{OperationID: newAgentOperationID(), RuntimeID: instance.ID, Capability: model.AgentCapabilityDeploymentScale, RequestID: requestID, ChatSessionID: r.Header.Get("X-Chat-Session-ID"), Parameters: string(parameters), ParametersHash: hash, ResourceVersion: deployment.ResourceVersion, Status: model.AgentOperationPendingApproval, Summary: fmt.Sprintf("scale deployment %s/%s to %d replicas", request.Namespace, request.Name, request.Replicas), ExpiresAt: time.Now().Add(15 * time.Minute)}
	stored, _, err := h.store.CreateAgentOperation(operation)
	if err != nil {
		writeAgentError(w, http.StatusInternalServerError, "operation persistence failed", true)
		return
	}
	h.audit(instance, "agent.deployment_scale_requested", map[string]string{"capability": model.AgentCapabilityDeploymentScale, "namespace": request.Namespace, "name": request.Name, "operation_id": stored.OperationID})
	writeAgentResponse(w, http.StatusAccepted, agentAPIResponse{Status: "pending_approval", OperationID: stored.OperationID, Summary: "deployment scale is pending approval"})
}

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

type deploymentScaleRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Replicas  int32  `json:"replicas"`
}

const agentLogLimit = 32 * 1024

var (
	agentResourceNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	agentContainerPattern    = regexp.MustCompile(`^[A-Za-z0-9]([-_A-Za-z0-9.]*[A-Za-z0-9])?$`)
	agentRequestIDPattern    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	agentSensitiveText       = regexp.MustCompile(`(?i)((?:token|password|secret|api[_-]?key)\s*[=:]\s*)[^\s,;]+`)
	agentAuthorizationText   = regexp.MustCompile(`(?i)(authorization\s*:\s*(?:bearer|basic)\s+)[^\s,;]+`)
)

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
func validAgentNamespace(value string) bool { return validAgentName(value) }
func validAgentContainer(value string) bool {
	return len(value) <= 63 && agentContainerPattern.MatchString(value)
}
func validAgentKind(value string) bool {
	return value == "deployment" || value == "statefulset" || value == "daemonset"
}
func validAgentRequestID(value string) bool { return agentRequestIDPattern.MatchString(value) }
func validAgentOperationID(value string) bool {
	return strings.HasPrefix(value, "op_") && validAgentRequestID(value)
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
func redactAgentText(value string) string {
	value = agentSensitiveText.ReplaceAllString(value, "$1[REDACTED]")
	return agentAuthorizationText.ReplaceAllString(value, "$1[REDACTED]")
}
func newAgentOperationID() string {
	bytes := make([]byte, 18)
	if _, err := rand.Read(bytes); err == nil {
		return "op_" + base64.RawURLEncoding.EncodeToString(bytes)
	}
	// The fallback remains opaque and only protects against the vanishingly
	// unlikely RNG failure; the database unique index is the final guard.
	return fmt.Sprintf("op_%d", time.Now().UnixNano())
}
func operationSummary(operation *model.AgentOperation) string {
	if operation.ErrorSummary != "" {
		return operation.ErrorSummary
	}
	return operation.Summary
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
