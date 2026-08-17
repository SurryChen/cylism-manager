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
	"net/url"
	"regexp"
	"sort"
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
		GetAlertEvent(id uint) (*model.AlertEvent, error)
		ListAlertEvents(int) ([]model.AlertEvent, error)
		UpdateAlertEvent(*model.AlertEvent) error
		GetAlertAutomationPolicy() (*model.AlertAutomationPolicy, error)
		ListNodeRegistryMirrors() ([]model.NodeRegistryMirror, error)
		ListRegistryProxies() ([]model.RegistryProxy, error)
		GetActiveClusterDNSPolicy() (*model.ClusterDNSPolicy, error)
		ListServers() ([]model.Server, error)
		CreateAuditLog(entry *model.AuditLog) error
	}
	client           *k8s.Client
	authenticator    AgentAuthenticator
	registryVerifier agentRegistryNodeVerifier
}

func NewAgentHandler(store interface {
	HasAgentCapability(runtimeID uint, capability, namespace string) (bool, error)
	ListAgentCapabilityGrants(runtimeID uint) ([]model.AgentCapabilityGrant, error)
	CreateAgentOperation(operation *model.AgentOperation) (*model.AgentOperation, bool, error)
	GetAgentOperation(operationID string) (*model.AgentOperation, error)
	GetAlertEvent(id uint) (*model.AlertEvent, error)
	ListAlertEvents(int) ([]model.AlertEvent, error)
	UpdateAlertEvent(*model.AlertEvent) error
	GetAlertAutomationPolicy() (*model.AlertAutomationPolicy, error)
	ListNodeRegistryMirrors() ([]model.NodeRegistryMirror, error)
	ListRegistryProxies() ([]model.RegistryProxy, error)
	GetActiveClusterDNSPolicy() (*model.ClusterDNSPolicy, error)
	ListServers() ([]model.Server, error)
	CreateAuditLog(entry *model.AuditLog) error
}, client *k8s.Client, authenticator AgentAuthenticator) *AgentHandler {
	return &AgentHandler{store: store, client: client, authenticator: authenticator}
}

func (h *AgentHandler) WithRegistryVerifier(verifier agentRegistryNodeVerifier) *AgentHandler {
	h.registryVerifier = verifier
	return h
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

// PodGet returns only status fields useful for scheduling and startup diagnosis.
func (h *AgentHandler) PodGet(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	namespace, name := r.URL.Query().Get("namespace"), r.URL.Query().Get("name")
	if !validAgentNamespace(namespace) || !validAgentName(name) {
		writeAgentError(w, http.StatusBadRequest, "invalid pod query", false)
		return
	}
	if !h.requireCapability(w, instance, model.AgentCapabilityWorkloadRead, namespace) {
		return
	}
	if h.client == nil || h.client.Clientset == nil {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	pod, err := h.client.Clientset.CoreV1().Pods(namespace).Get(r.Context(), name, metav1.GetOptions{})
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "pod unavailable", true)
		return
	}
	h.audit(instance, "agent.pod_get", map[string]string{"capability": model.AgentCapabilityWorkloadRead, "namespace": namespace, "name": name})
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: podDiagnosticSummary(pod), Summary: "pod status retrieved"})
}

// EventList returns a bounded, redacted list of Events for one supported object.
func (h *AgentHandler) EventList(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	namespace, kind, name := r.URL.Query().Get("namespace"), r.URL.Query().Get("involved_kind"), r.URL.Query().Get("involved_name")
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if !validAgentNamespace(namespace) || !validAgentInvolvedKind(kind) || !validAgentName(name) || err != nil || limit < 1 || limit > 30 {
		writeAgentError(w, http.StatusBadRequest, "invalid event query", false)
		return
	}
	if !h.requireCapability(w, instance, model.AgentCapabilityEventsRead, namespace) {
		return
	}
	if h.client == nil || h.client.Clientset == nil {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	events, err := h.client.Clientset.CoreV1().Events(namespace).List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "events unavailable", true)
		return
	}
	matching := make([]corev1.Event, 0, len(events.Items))
	for _, event := range events.Items {
		if strings.EqualFold(event.InvolvedObject.Kind, kind) && event.InvolvedObject.Name == name {
			matching = append(matching, event)
		}
	}
	sort.SliceStable(matching, func(i, j int) bool { return eventTime(matching[i]).After(eventTime(matching[j])) })
	if len(matching) > limit {
		matching = matching[:limit]
	}
	data := make([]map[string]any, 0, len(matching))
	for _, event := range matching {
		data = append(data, map[string]any{"type": event.Type, "reason": event.Reason, "message": redactAgentText(truncateAgentText(event.Message, 1024)), "count": event.Count, "last_timestamp": eventTime(event).UTC().Format(time.RFC3339)})
	}
	h.audit(instance, "agent.event_list", map[string]string{"capability": model.AgentCapabilityEventsRead, "namespace": namespace, "kind": kind, "name": name})
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: data, Summary: "events retrieved"})
}

func (h *AgentHandler) PVCGet(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	namespace, name := r.URL.Query().Get("namespace"), r.URL.Query().Get("name")
	if !validAgentNamespace(namespace) || !validAgentName(name) {
		writeAgentError(w, http.StatusBadRequest, "invalid PVC query", false)
		return
	}
	if !h.requireCapability(w, instance, model.AgentCapabilityStorageRead, namespace) {
		return
	}
	if h.client == nil || h.client.Clientset == nil {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	pvc, err := h.client.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(r.Context(), name, metav1.GetOptions{})
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "persistent volume claim unavailable", true)
		return
	}
	requests := ""
	if storage := pvc.Spec.Resources.Requests.Storage(); storage != nil {
		requests = storage.String()
	}
	h.audit(instance, "agent.pvc_get", map[string]string{"capability": model.AgentCapabilityStorageRead, "namespace": namespace, "name": name})
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: map[string]any{"name": pvc.Name, "namespace": pvc.Namespace, "phase": pvc.Status.Phase, "volume_name": pvc.Spec.VolumeName, "storage_class": stringValue(pvc.Spec.StorageClassName), "requested_storage": requests, "access_modes": pvc.Spec.AccessModes}, Summary: "persistent volume claim retrieved"})
}

func (h *AgentHandler) NodeGet(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	name := r.URL.Query().Get("name")
	if !validAgentName(name) {
		writeAgentError(w, http.StatusBadRequest, "invalid node query", false)
		return
	}
	if !h.requireCapability(w, instance, model.AgentCapabilityClusterRead, "") {
		return
	}
	if h.client == nil || h.client.Clientset == nil {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	node, err := h.client.Clientset.CoreV1().Nodes().Get(r.Context(), name, metav1.GetOptions{})
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "node unavailable", true)
		return
	}
	conditions := make([]map[string]string, 0, len(node.Status.Conditions))
	for _, condition := range node.Status.Conditions {
		conditions = append(conditions, map[string]string{"type": string(condition.Type), "status": string(condition.Status), "reason": condition.Reason, "message": redactAgentText(truncateAgentText(condition.Message, 512))})
	}
	h.audit(instance, "agent.node_get", map[string]string{"capability": model.AgentCapabilityClusterRead, "name": name})
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: map[string]any{"name": node.Name, "unschedulable": node.Spec.Unschedulable, "taints": node.Spec.Taints, "conditions": conditions, "allocatable": resourceListSummary(node.Status.Allocatable)}, Summary: "node status retrieved"})
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

// RegistryStatus reports only the safe, Manager-owned projection of registry state.
func (h *AgentHandler) RegistryStatus(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.requireCapability(w, instance, model.AgentCapabilityRegistryRead, "") {
		return
	}
	mirrors, err := h.store.ListNodeRegistryMirrors()
	if err != nil {
		writeAgentError(w, http.StatusInternalServerError, "registry status unavailable", true)
		return
	}
	proxies, err := h.store.ListRegistryProxies()
	if err != nil {
		writeAgentError(w, http.StatusInternalServerError, "registry status unavailable", true)
		return
	}
	h.audit(instance, "agent.registry_status", map[string]string{"capability": model.AgentCapabilityRegistryRead})
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: agentRegistryStatus(mirrors, proxies), Summary: "registry status retrieved"})
}

// DNSStatus returns only the platform-managed forwarding state and CoreDNS
// readiness. It does not expose the Corefile or permit Runtime-initiated DNS
// or Kubernetes exec requests.
func (h *AgentHandler) DNSStatus(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.requireCapability(w, instance, model.AgentCapabilityDNSRead, "") {
		return
	}
	if h.client == nil || h.client.Clientset == nil {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	configMap, err := h.client.Clientset.CoreV1().ConfigMaps(coreDNSNamespace).Get(r.Context(), coreDNSConfigMap, metav1.GetOptions{})
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "CoreDNS configuration unavailable", true)
		return
	}
	pods, err := h.client.Clientset.CoreV1().Pods(coreDNSNamespace).List(r.Context(), metav1.ListOptions{LabelSelector: "k8s-app=kube-dns"})
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "CoreDNS status unavailable", true)
		return
	}
	ready, total := 0, len(pods.Items)
	for index := range pods.Items {
		if coreDNSPodReady(&pods.Items[index]) {
			ready++
		}
	}
	policy, err := h.store.GetActiveClusterDNSPolicy()
	if err != nil && !strings.Contains(err.Error(), "record not found") {
		writeAgentError(w, http.StatusInternalServerError, "DNS policy unavailable", true)
		return
	}
	h.audit(instance, "agent.dns_status", map[string]string{"capability": model.AgentCapabilityDNSRead})
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: map[string]any{
		"forwarding":    forwardTargets(configMap.Data["Corefile"]),
		"active_policy": policyPayload(policy),
		"coredns":       map[string]int{"ready": ready, "total": total},
	}, Summary: "cluster DNS status retrieved"})
}

// DNSResolve reports the last controlled egress observation for a fixed image
// registry hostname. It intentionally does not accept arbitrary DNS targets.
func (h *AgentHandler) DNSResolve(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(r.URL.Query().Get("name")), "."))
	registry, allowed := agentDNSRegistry(name)
	if !allowed {
		writeAgentError(w, http.StatusBadRequest, "DNS name is not allowlisted", false)
		return
	}
	if !h.requireCapability(w, instance, model.AgentCapabilityDNSRead, "") {
		return
	}
	proxies, err := h.store.ListRegistryProxies()
	if err != nil {
		writeAgentError(w, http.StatusInternalServerError, "registry DNS observation unavailable", true)
		return
	}
	mirrors, err := h.store.ListNodeRegistryMirrors()
	if err != nil {
		writeAgentError(w, http.StatusInternalServerError, "registry DNS observation unavailable", true)
		return
	}
	config, configured := agentResolveRegistry(registry, mirrors, proxies)
	data := map[string]any{"name": name, "registry": registry, "configured": configured, "observation": "no managed egress observation"}
	if configured && config.Proxy != nil {
		data["observation"] = config.Proxy.LastDiagnosticStatus
		data["summary"] = redactAgentText(truncateAgentText(config.Proxy.LastDiagnosticError, 256))
		data["observed_at"] = config.Proxy.LastDiagnosticAt
	}
	h.audit(instance, "agent.dns_resolve", map[string]string{"capability": model.AgentCapabilityDNSRead, "name": name})
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: data, Summary: "managed DNS observation retrieved"})
}

// RegistryProxyDiagnose returns a redacted saved diagnostic. New egress probes
// remain a browser-admin action against a selected managed proxy only.
func (h *AgentHandler) RegistryProxyDiagnose(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	registry := r.URL.Query().Get("registry")
	if !validAgentRegistry(registry) {
		writeAgentError(w, http.StatusBadRequest, "invalid registry query", false)
		return
	}
	if !h.requireCapability(w, instance, model.AgentCapabilityRegistryProxyDiagnose, "") {
		return
	}
	proxies, err := h.store.ListRegistryProxies()
	if err != nil {
		writeAgentError(w, http.StatusInternalServerError, "registry proxy diagnostics unavailable", true)
		return
	}
	registry = normalizeRegistry(registry)
	for _, proxy := range proxies {
		if normalizeRegistry(proxy.Registry) != registry {
			continue
		}
		h.audit(instance, "agent.registry_proxy_diagnose", map[string]string{"capability": model.AgentCapabilityRegistryProxyDiagnose, "registry": registry})
		writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: map[string]any{
			"registry": registry, "status": proxy.Status, "node": proxy.NodeName,
			"diagnostic_status": proxy.LastDiagnosticStatus, "diagnostic_summary": redactAgentText(truncateAgentText(proxy.LastDiagnosticError, 256)), "diagnostic_at": proxy.LastDiagnosticAt,
		}, Summary: "registry proxy diagnostic retrieved"})
		return
	}
	writeAgentError(w, http.StatusNotFound, "managed registry proxy not found", false)
}

func agentDNSRegistry(name string) (string, bool) {
	switch name {
	case "registry-1.docker.io":
		return "docker.io", true
	case "registry.k8s.io":
		return "registry.k8s.io", true
	case "ghcr.io":
		return "ghcr.io", true
	default:
		return "", false
	}
}

// ImageDiagnose combines Pod image-pull state with Manager-owned registry state.
func (h *AgentHandler) ImageDiagnose(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	namespace, name := r.URL.Query().Get("namespace"), r.URL.Query().Get("pod")
	if !validAgentNamespace(namespace) || !validAgentName(name) {
		writeAgentError(w, http.StatusBadRequest, "invalid image diagnose query", false)
		return
	}
	if !h.requireCapability(w, instance, model.AgentCapabilityWorkloadRead, namespace) || !h.requireCapability(w, instance, model.AgentCapabilityRegistryRead, "") {
		return
	}
	if h.client == nil || h.client.Clientset == nil {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	pod, err := h.client.Clientset.CoreV1().Pods(namespace).Get(r.Context(), name, metav1.GetOptions{})
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "pod unavailable", true)
		return
	}
	mirrors, err := h.store.ListNodeRegistryMirrors()
	if err != nil {
		writeAgentError(w, http.StatusInternalServerError, "registry status unavailable", true)
		return
	}
	proxies, err := h.store.ListRegistryProxies()
	if err != nil {
		writeAgentError(w, http.StatusInternalServerError, "registry status unavailable", true)
		return
	}
	summary := podDiagnosticSummary(pod)
	containers, _ := summary["containers"].([]map[string]any)
	failures := agentImagePullFailures(containers)
	diagnoses := make([]map[string]any, 0, len(failures))
	for _, failure := range failures {
		config, configured := agentResolveRegistry(failure["registry"], mirrors, proxies)
		entry := map[string]any{"code": "image_pull_failure_detected", "registry": failure["registry"], "reason": failure["reason"]}
		if !configured {
			entry["configuration_code"] = "registry_config_missing"
		} else if config.Mirror != nil {
			entry["mirror"] = map[string]any{"enabled": config.Mirror.Enabled, "verification_status": config.Mirror.LastVerifyStatus, "node_status": agentMirrorNodeStatus(config.Mirror, pod.Spec.NodeName)}
			if config.Mirror.LastVerifyStatus != "" && config.Mirror.LastVerifyStatus != "succeeded" {
				entry["configuration_code"] = "mirror_unhealthy"
			}
		} else if config.Proxy != nil {
			entry["proxy"] = map[string]any{"status": config.Proxy.Status, "node": config.Proxy.NodeName, "egress_status": config.Proxy.LastDiagnosticStatus}
			if config.Proxy.Status != "ready" && config.Proxy.Status != "running" && config.Proxy.Status != "succeeded" {
				entry["configuration_code"] = "registry_proxy_unready"
			} else if config.Proxy.LastDiagnosticStatus == "upstream_connect_timeout" || config.Proxy.LastDiagnosticStatus == "dns_resolution_failed" {
				entry["configuration_code"] = config.Proxy.LastDiagnosticStatus
			}
		}
		diagnoses = append(diagnoses, entry)
	}
	h.audit(instance, "agent.image_diagnose", map[string]string{"capability": model.AgentCapabilityRegistryRead, "namespace": namespace, "pod": name})
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: map[string]any{"pod": summary, "image_pull_failures": failures, "diagnoses": diagnoses}, Summary: "image pull diagnosis retrieved"})
}

func (h *AgentHandler) RegistryNodeVerify(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	node, registry := r.URL.Query().Get("node"), r.URL.Query().Get("registry")
	if !validAgentName(node) || !validAgentRegistry(registry) {
		writeAgentError(w, http.StatusBadRequest, "invalid registry verification query", false)
		return
	}
	if !h.requireCapability(w, instance, model.AgentCapabilityRegistryVerify, "") {
		return
	}
	config, server, err := h.registryNodeConfig(node, registry)
	if err != nil {
		writeAgentError(w, http.StatusBadRequest, err.Error(), false)
		return
	}
	if h.registryVerifier == nil {
		writeAgentError(w, http.StatusServiceUnavailable, "registry verification unavailable", true)
		return
	}
	results, err := h.registryVerifier(server, config.Endpoints)
	if err != nil {
		detail := redactAgentText(truncateAgentText(err.Error(), 512))
		writeAgentError(w, http.StatusBadGateway, "registry endpoint verification failed: "+detail, true)
		return
	}
	h.audit(instance, "agent.registry_node_verify", map[string]string{"capability": model.AgentCapabilityRegistryVerify, "node": node, "registry": config.Registry})
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: map[string]any{"node": node, "registry": config.Registry, "endpoints": results}, Summary: "registry endpoint verification completed"})
}

func (h *AgentHandler) RegistryNodePullCheck(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	requestID := r.Header.Get("X-Request-ID")
	if !validAgentRequestID(requestID) || r.Header.Get("Idempotency-Key") != requestID {
		writeAgentError(w, http.StatusBadRequest, "matching request and idempotency keys are required", false)
		return
	}
	var request registryNodeRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4097))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF || !validAgentName(request.Node) || !validAgentRegistry(request.Registry) {
		writeAgentError(w, http.StatusBadRequest, "invalid registry pull check request", false)
		return
	}
	if !h.requireCapability(w, instance, model.AgentCapabilityRegistryPullCheck, "") {
		return
	}
	config, _, err := h.registryNodeConfig(request.Node, request.Registry)
	if err != nil || config.Mirror == nil || strings.TrimSpace(config.VerificationImage) == "" {
		writeAgentError(w, http.StatusBadRequest, "registered verification image unavailable", false)
		return
	}
	parameters, _ := json.Marshal(registryNodeRequest{Node: request.Node, Registry: config.Registry, VerificationImage: config.VerificationImage})
	operation := &model.AgentOperation{OperationID: newAgentOperationID(), RuntimeID: instance.ID, Capability: model.AgentCapabilityRegistryPullCheck, RequestID: requestID, ChatSessionID: r.Header.Get("X-Chat-Session-ID"), Parameters: string(parameters), ParametersHash: fmt.Sprintf("%x", sha256.Sum256(parameters)), Status: model.AgentOperationPendingApproval, Summary: fmt.Sprintf("pull verification image %s for registry %s on node %s", config.VerificationImage, config.Registry, request.Node), ExpiresAt: time.Now().Add(15 * time.Minute)}
	stored, _, err := h.store.CreateAgentOperation(operation)
	if err != nil {
		writeAgentError(w, http.StatusInternalServerError, "operation persistence failed", true)
		return
	}
	h.audit(instance, "agent.registry_pull_check_requested", map[string]string{"capability": model.AgentCapabilityRegistryPullCheck, "node": request.Node, "registry": config.Registry, "operation_id": stored.OperationID})
	writeAgentResponse(w, http.StatusAccepted, agentAPIResponse{Status: "pending_approval", OperationID: stored.OperationID, Summary: "registry verification image pull is pending approval"})
}

type registryNodeRequest struct {
	Node              string `json:"node"`
	Registry          string `json:"registry"`
	VerificationImage string `json:"verification_image,omitempty"`
}

func (h *AgentHandler) registryNodeConfig(node, registry string) (agentRegistryConfig, *model.Server, error) {
	mirrors, err := h.store.ListNodeRegistryMirrors()
	if err != nil {
		return agentRegistryConfig{}, nil, fmt.Errorf("registry status unavailable")
	}
	proxies, err := h.store.ListRegistryProxies()
	if err != nil {
		return agentRegistryConfig{}, nil, fmt.Errorf("registry status unavailable")
	}
	config, found := agentResolveRegistry(registry, mirrors, proxies)
	if !found {
		return agentRegistryConfig{}, nil, fmt.Errorf("registry is not platform configured")
	}
	servers, err := h.store.ListServers()
	if err != nil {
		return agentRegistryConfig{}, nil, fmt.Errorf("node mapping unavailable")
	}
	server, found := agentServerForNode(node, servers)
	if !found {
		return agentRegistryConfig{}, nil, fmt.Errorf("node is not a platform managed server")
	}
	return config, server, nil
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
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: map[string]any{"id": event.ID, "alert_name": event.AlertName, "severity": event.Severity, "node": event.NodeName, "mount_point": event.MountPoint, "status": event.Status, "starts_at": event.StartsAt, "labels": json.RawMessage(event.Labels), "annotations": json.RawMessage(event.Annotations), "diagnostic_summary": event.DiagnosticSummary}, Summary: "alert event retrieved"})
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
		items = append(items, map[string]any{"id": event.ID, "alert_name": event.AlertName, "severity": event.Severity, "node": event.NodeName, "mount_point": event.MountPoint, "status": event.Status, "starts_at": event.StartsAt, "updated_at": event.UpdatedAt, "diagnostic_summary": event.DiagnosticSummary, "operation_id": event.OperationID})
	}
	h.audit(instance, "agent.alert_list", map[string]string{"capability": model.AgentCapabilityAlertRead})
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: map[string]any{"events": items}, Summary: "recent alert events retrieved"})
}

// MonitoringDiskGrowth makes one Manager-owned metrics query. The Runtime may
// select only a node and a bounded range; it cannot submit PromQL.
func (h *AgentHandler) MonitoringDiskGrowth(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authenticate(w, r)
	if !ok || !h.requireCapability(w, instance, model.AgentCapabilityMonitoringRead, "") {
		return
	}
	if K8s == nil || !monitoringDataStoreAvailable(K8s.VictoriaMetricsStatus()) {
		writeAgentError(w, http.StatusServiceUnavailable, "monitoring data store unavailable", true)
		return
	}
	node := strings.TrimSpace(r.URL.Query().Get("node"))
	if node == "" || !validAgentResourceName(node) {
		writeAgentError(w, http.StatusBadRequest, "invalid node", false)
		return
	}
	rangeName := strings.TrimSpace(r.URL.Query().Get("range"))
	if rangeName != "6h" && rangeName != "24h" {
		writeAgentError(w, http.StatusBadRequest, "range must be 6h or 24h", false)
		return
	}
	rangeSpec := monitoringRanges[rangeName]
	data, err := queryVictoriaMetrics(r.Context(), "/api/v1/query", url.Values{"query": []string{diskGrowthQueries(monitoringPromQLWindow(rangeSpec.window), node)[0].query}})
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "monitoring disk growth unavailable", true)
		return
	}
	mounts := normalizeMountGrowth(data)
	h.audit(instance, "agent.monitoring_disk_growth", map[string]string{"capability": model.AgentCapabilityMonitoringRead, "node": node, "range": rangeName})
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: map[string]any{"node": node, "range": rangeName, "mounts": mounts}, Summary: "disk growth retrieved"})
}

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
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&request); err != nil || request.AlertID == 0 || !validMaintenanceRecipe(request.Recipe) {
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
	operation := &model.AgentOperation{OperationID: newAgentOperationID(), RuntimeID: instance.ID, Capability: model.AgentCapabilityMaintenanceCleanup, RequestID: requestID, ChatSessionID: r.Header.Get("X-Chat-Session-ID"), Parameters: string(parameters), ParametersHash: fmt.Sprintf("%x", sha256.Sum256(parameters)), Status: model.AgentOperationPendingApproval, Summary: maintenanceCleanupSummary(event.NodeName, request.Recipe), ExpiresAt: time.Now().Add(15 * time.Minute)}
	stored, _, err := h.store.CreateAgentOperation(operation)
	if err != nil {
		writeAgentError(w, http.StatusInternalServerError, "unable to create cleanup approval", true)
		return
	}
	event.OperationID, event.Status, event.DiagnosticSummary = stored.OperationID, model.AlertEventAwaitingApproval, "等待管理员审批固定清理配方"
	_ = h.store.UpdateAlertEvent(event)
	h.audit(instance, "agent.maintenance_cleanup_requested", map[string]string{"capability": model.AgentCapabilityMaintenanceCleanup, "alert_id": strconv.Itoa(int(event.ID)), "recipe": request.Recipe, "operation_id": stored.OperationID})
	writeAgentResponse(w, http.StatusAccepted, agentAPIResponse{Status: "pending_approval", OperationID: stored.OperationID, Summary: "maintenance cleanup requires approval"})
}

func validMaintenanceRecipe(recipe string) bool {
	return recipe == "journal-vacuum" || recipe == "container-image-prune"
}
func maintenanceCleanupSummary(node, recipe string) string {
	if recipe == "journal-vacuum" {
		return fmt.Sprintf("vacuum system journal older than 7 days on node %s", node)
	}
	return fmt.Sprintf("prune unused container images on node %s", node)
}

func validAgentResourceName(value string) bool { return agentResourceNamePattern.MatchString(value) }

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
	agentRegistryPattern     = regexp.MustCompile(`^[A-Za-z0-9](?:[-A-Za-z0-9.]*[A-Za-z0-9])?(?::[0-9]{1,5})?$`)
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
	containers := make([]map[string]any, 0, len(pod.Status.InitContainerStatuses)+len(pod.Status.ContainerStatuses))
	for _, status := range append(append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...), pod.Status.ContainerStatuses...) {
		entry := map[string]any{"name": status.Name, "ready": status.Ready, "restart_count": status.RestartCount, "image": status.Image}
		if status.State.Waiting != nil {
			entry["state"] = "waiting"
			entry["reason"] = status.State.Waiting.Reason
			entry["message"] = redactAgentText(truncateAgentText(status.State.Waiting.Message, 512))
		} else if status.State.Terminated != nil {
			entry["state"] = "terminated"
			entry["reason"] = status.State.Terminated.Reason
			entry["exit_code"] = status.State.Terminated.ExitCode
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

func truncateAgentText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
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
