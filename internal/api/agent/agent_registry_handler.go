package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	networkservice "github.com/cylism/cylism-manager/internal/service/network"
	registryservice "github.com/cylism/cylism-manager/internal/service/registry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	coreDNSNamespace = "kube-system"
	coreDNSConfigMap = "coredns"
)

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
	if h.client == nil || !h.client.KubernetesAvailable() {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	configMap, err := h.client.Clientset().CoreV1().ConfigMaps(coreDNSNamespace).Get(r.Context(), coreDNSConfigMap, metav1.GetOptions{})
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "CoreDNS configuration unavailable", true)
		return
	}
	pods, err := h.client.Clientset().CoreV1().Pods(coreDNSNamespace).List(r.Context(), metav1.ListOptions{LabelSelector: "k8s-app=kube-dns"})
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "CoreDNS status unavailable", true)
		return
	}
	ready, total := 0, len(pods.Items)
	for index := range pods.Items {
		if networkservice.CoreDNSPodReady(&pods.Items[index]) {
			ready++
		}
	}
	policy, err := h.store.GetActiveClusterDNSPolicy()
	if err != nil && !strings.Contains(err.Error(), "record not found") {
		writeAgentError(w, http.StatusInternalServerError, "DNS policy unavailable", true)
		return
	}
	writeAgentResponse(w, http.StatusOK, agentAPIResponse{Status: "ok", Data: map[string]any{
		"forwarding":    networkservice.ForwardTargets(configMap.Data["Corefile"]),
		"active_policy": networkservice.PolicyPayload(policy),
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
	config, configured := registryservice.ResolveRegistry(registry, mirrors, proxies)
	data := map[string]any{"name": name, "registry": registry, "configured": configured, "observation": "no managed egress observation"}
	if configured && config.Proxy != nil {
		data["observation"] = config.Proxy.LastDiagnosticStatus
		data["summary"] = redactAgentText(truncateAgentText(config.Proxy.LastDiagnosticError, 256))
		data["observed_at"] = config.Proxy.LastDiagnosticAt
	}
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
	registry = registryservice.NormalizeRegistryHost(registry)
	for _, proxy := range proxies {
		if registryservice.NormalizeRegistryHost(proxy.Registry) != registry {
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
	if h.client == nil || !h.client.KubernetesAvailable() {
		writeAgentError(w, http.StatusServiceUnavailable, "Kubernetes client unavailable", true)
		return
	}
	pod, err := h.client.Clientset().CoreV1().Pods(namespace).Get(r.Context(), name, metav1.GetOptions{})
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
	failures := registryservice.ImagePullFailures(containers)
	diagnoses := make([]map[string]any, 0, len(failures))
	for _, failure := range failures {
		config, configured := registryservice.ResolveRegistry(failure["registry"], mirrors, proxies)
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
	results, err := h.registryVerifier.Verify(r.Context(), server, config.Endpoints)
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
	h.audit(instance, "agent.registry_pull_check_requested", map[string]string{"capability": model.AgentCapabilityRegistryPullCheck, "node": request.Node, "registry": config.Registry, "request_id": requestID, "operation_id": stored.OperationID})
	writeAgentResponse(w, http.StatusAccepted, agentAPIResponse{Status: "pending_approval", OperationID: stored.OperationID, Summary: "registry verification image pull is pending approval"})
}

type registryNodeRequest struct {
	Node              string `json:"node"`
	Registry          string `json:"registry"`
	VerificationImage string `json:"verification_image,omitempty"`
}

func (h *AgentHandler) registryNodeConfig(node, registry string) (registryservice.RegistryConfig, *model.Server, error) {
	mirrors, err := h.store.ListNodeRegistryMirrors()
	if err != nil {
		return registryservice.RegistryConfig{}, nil, fmt.Errorf("registry status unavailable")
	}
	proxies, err := h.store.ListRegistryProxies()
	if err != nil {
		return registryservice.RegistryConfig{}, nil, fmt.Errorf("registry status unavailable")
	}
	config, found := registryservice.ResolveRegistry(registry, mirrors, proxies)
	if !found {
		return registryservice.RegistryConfig{}, nil, fmt.Errorf("registry is not platform configured")
	}
	servers, err := h.store.ListServers()
	if err != nil {
		return registryservice.RegistryConfig{}, nil, fmt.Errorf("node mapping unavailable")
	}
	server, found := agentServerForNode(node, servers)
	if !found {
		return registryservice.RegistryConfig{}, nil, fmt.Errorf("node is not a platform managed server")
	}
	return config, server, nil
}

func agentRegistryStatus(mirrors []model.NodeRegistryMirror, proxies []model.RegistryProxy) map[string]any {
	result := map[string]any{"mirrors": make([]map[string]any, 0, len(mirrors)), "proxies": make([]map[string]any, 0, len(proxies))}
	for index := range mirrors {
		mirror := &mirrors[index]
		nodes := make([]map[string]any, 0, len(mirror.NodeStatuses))
		for _, status := range mirror.NodeStatuses {
			nodes = append(nodes, map[string]any{"node": status.Server.K8sNodeName, "status": status.Status, "detail": redactAgentText(truncateAgentText(status.Detail, 256))})
		}
		result["mirrors"] = append(result["mirrors"].([]map[string]any), map[string]any{"registry": registryservice.NormalizeRegistryHost(mirror.Registry), "enabled": mirror.Enabled, "endpoints": registryservice.SafeEndpoints(mirror.Endpoints), "verification_status": mirror.LastVerifyStatus, "verification_error": redactAgentText(truncateAgentText(mirror.LastVerifyError, 256)), "nodes": nodes})
	}
	for _, proxy := range proxies {
		result["proxies"] = append(result["proxies"].([]map[string]any), map[string]any{"registry": registryservice.NormalizeRegistryHost(proxy.Registry), "status": proxy.Status, "node": proxy.NodeName, "endpoint": registryservice.SafeEndpoint("http://" + proxy.EndpointHost + fmt.Sprintf(":%d", proxy.NodePort)), "error": redactAgentText(truncateAgentText(proxy.LastError, 256)), "egress_status": proxy.LastDiagnosticStatus, "egress_error": redactAgentText(truncateAgentText(proxy.LastDiagnosticError, 256))})
	}
	return result
}

func agentMirrorNodeStatus(mirror *model.NodeRegistryMirror, node string) map[string]any {
	if mirror == nil {
		return nil
	}
	for _, status := range mirror.NodeStatuses {
		if status.Server.K8sNodeName == node {
			return map[string]any{"status": status.Status, "detail": redactAgentText(truncateAgentText(status.Detail, 256))}
		}
	}
	return map[string]any{"status": "not_applied"}
}
