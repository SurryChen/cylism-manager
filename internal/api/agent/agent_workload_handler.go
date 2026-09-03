package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
	if h.client == nil || !h.client.KubernetesAvailable() {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	var data any
	var err error
	switch kind {
	case "deployment":
		object, getErr := h.client.Clientset().AppsV1().Deployments(namespace).Get(r.Context(), name, metav1.GetOptions{})
		err = getErr
		if object != nil {
			data = workloadSummary(object.Name, object.Namespace, kind, object.ResourceVersion, replicas(object.Spec.Replicas), object.Status.ReadyReplicas)
		}
	case "statefulset":
		object, getErr := h.client.Clientset().AppsV1().StatefulSets(namespace).Get(r.Context(), name, metav1.GetOptions{})
		err = getErr
		if object != nil {
			data = workloadSummary(object.Name, object.Namespace, kind, object.ResourceVersion, replicas(object.Spec.Replicas), object.Status.ReadyReplicas)
		}
	case "daemonset":
		object, getErr := h.client.Clientset().AppsV1().DaemonSets(namespace).Get(r.Context(), name, metav1.GetOptions{})
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
	previous := false
	if value := r.URL.Query().Get("previous"); value != "" {
		previous, err = strconv.ParseBool(value)
	}
	if !validAgentNamespace(namespace) || !validAgentName(pod) || !validAgentContainer(container) || err != nil || tail < 1 || tail > 200 {
		writeAgentError(w, http.StatusBadRequest, "invalid workload logs query", false)
		return
	}
	if !h.requireCapability(w, instance, model.AgentCapabilityWorkloadLogs, namespace) {
		return
	}
	if h.client == nil || !h.client.KubernetesAvailable() {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	stream, err := h.client.Clientset().CoreV1().Pods(namespace).GetLogs(pod, &corev1.PodLogOptions{Container: container, TailLines: &tail, Previous: previous}).Stream(r.Context())
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
	h.audit(instance, "agent.workload_logs", map[string]string{"capability": model.AgentCapabilityWorkloadLogs, "namespace": namespace, "pod": pod, "container": container, "previous": strconv.FormatBool(previous)})
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: map[string]any{"logs": redactAgentText(string(content)), "truncated": truncated, "previous": previous}, Summary: "workload logs retrieved"})
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
	if h.client == nil || !h.client.KubernetesAvailable() {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	pod, err := h.client.Clientset().CoreV1().Pods(namespace).Get(r.Context(), name, metav1.GetOptions{})
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
	if h.client == nil || !h.client.KubernetesAvailable() {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	events, err := h.client.Clientset().CoreV1().Events(namespace).List(r.Context(), metav1.ListOptions{})
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
	if h.client == nil || !h.client.KubernetesAvailable() {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	pvc, err := h.client.Clientset().CoreV1().PersistentVolumeClaims(namespace).Get(r.Context(), name, metav1.GetOptions{})
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
	if h.client == nil || !h.client.KubernetesAvailable() {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	node, err := h.client.Clientset().CoreV1().Nodes().Get(r.Context(), name, metav1.GetOptions{})
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
	if h.client == nil || !h.client.KubernetesAvailable() {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	deployment, err := h.client.Clientset().AppsV1().Deployments(request.Namespace).Get(r.Context(), request.Name, metav1.GetOptions{})
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
