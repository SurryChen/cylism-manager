package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	infrastructureapi "github.com/cylism/cylism-manager/internal/api/infrastructure"
	"github.com/cylism/cylism-manager/internal/model"
	registryservice "github.com/cylism/cylism-manager/internal/service/registry"
)

type agentRegistryEndpointResult struct {
	Endpoint string `json:"endpoint"`
	DNS      string `json:"dns"`
	HTTP     string `json:"http"`
}

type agentRegistryNodeVerifier func(*model.Server, []string) ([]agentRegistryEndpointResult, error)
type agentRegistryPullExecutor func(*model.Server, string) error

type agentRegistryConfig struct {
	Registry          string
	Endpoints         []string
	VerificationImage string
	Mirror            *model.NodeRegistryMirror
	Proxy             *model.RegistryProxy
}

func agentRegistryStatus(mirrors []model.NodeRegistryMirror, proxies []model.RegistryProxy) map[string]any {
	result := map[string]any{"mirrors": make([]map[string]any, 0, len(mirrors)), "proxies": make([]map[string]any, 0, len(proxies))}
	for index := range mirrors {
		mirror := &mirrors[index]
		nodes := make([]map[string]any, 0, len(mirror.NodeStatuses))
		for _, status := range mirror.NodeStatuses {
			nodes = append(nodes, map[string]any{"node": status.Server.K8sNodeName, "status": status.Status, "detail": redactAgentText(truncateAgentText(status.Detail, 256))})
		}
		result["mirrors"] = append(result["mirrors"].([]map[string]any), map[string]any{
			"registry": registryservice.NormalizeRegistryHost(mirror.Registry), "enabled": mirror.Enabled, "endpoints": agentSafeEndpoints(mirror.Endpoints),
			"verification_status": mirror.LastVerifyStatus, "verification_error": redactAgentText(truncateAgentText(mirror.LastVerifyError, 256)), "nodes": nodes,
		})
	}
	for _, proxy := range proxies {
		result["proxies"] = append(result["proxies"].([]map[string]any), map[string]any{
			"registry": registryservice.NormalizeRegistryHost(proxy.Registry), "status": proxy.Status, "node": proxy.NodeName,
			"endpoint": safeRegistryEndpoint("http://" + proxy.EndpointHost + fmt.Sprintf(":%d", proxy.NodePort)), "error": redactAgentText(truncateAgentText(proxy.LastError, 256)),
			"egress_status": proxy.LastDiagnosticStatus, "egress_error": redactAgentText(truncateAgentText(proxy.LastDiagnosticError, 256)),
		})
	}
	return result
}

func agentSafeEndpoints(raw string) []string {
	var endpoints []string
	if json.Unmarshal([]byte(raw), &endpoints) != nil {
		return []string{}
	}
	result := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if safe := safeRegistryEndpoint(endpoint); safe != "" {
			result = append(result, safe)
		}
	}
	return result
}

func safeRegistryEndpoint(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func agentResolveRegistry(registry string, mirrors []model.NodeRegistryMirror, proxies []model.RegistryProxy) (agentRegistryConfig, bool) {
	registry = registryservice.NormalizeRegistryHost(registry)
	for index := range mirrors {
		mirror := &mirrors[index]
		if mirror.Enabled && registryservice.NormalizeRegistryHost(mirror.Registry) == registry {
			endpoints := agentSafeEndpoints(mirror.Endpoints)
			if len(endpoints) > 0 {
				return agentRegistryConfig{Registry: registry, Endpoints: endpoints, VerificationImage: strings.TrimSpace(mirror.VerificationImage), Mirror: mirror}, true
			}
		}
	}
	for index := range proxies {
		proxy := &proxies[index]
		if registryservice.NormalizeRegistryHost(proxy.Registry) == registry && proxy.EndpointHost != "" && proxy.NodePort > 0 {
			return agentRegistryConfig{Registry: registry, Endpoints: []string{safeRegistryEndpoint("http://" + proxy.EndpointHost + fmt.Sprintf(":%d", proxy.NodePort))}, Proxy: proxy}, true
		}
	}
	return agentRegistryConfig{}, false
}

func agentServerForNode(node string, servers []model.Server) (*model.Server, bool) {
	for index := range servers {
		if servers[index].K8sNodeName == node {
			return &servers[index], true
		}
	}
	return nil, false
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

func agentImageRegistry(image string) string {
	image = strings.TrimSpace(image)
	parts := strings.Split(image, "/")
	if len(parts) < 2 || (!strings.Contains(parts[0], ".") && !strings.Contains(parts[0], ":") && parts[0] != "localhost") {
		return "docker.io"
	}
	return registryservice.NormalizeRegistryHost(parts[0])
}

func agentImagePullFailures(podContainers []map[string]any) []map[string]string {
	failures := make([]map[string]string, 0)
	for _, container := range podContainers {
		reason, _ := container["reason"].(string)
		if reason != "ErrImagePull" && reason != "ImagePullBackOff" && reason != "InvalidImageName" {
			continue
		}
		image, _ := container["image"].(string)
		message, _ := container["message"].(string)
		failures = append(failures, map[string]string{"container": fmt.Sprint(container["name"]), "image": image, "registry": agentImageRegistry(image), "reason": reason, "message": message})
	}
	return failures
}

func agentRegistryVerificationCommand(endpoints []string) string {
	encoded, _ := json.Marshal(endpoints)
	data := base64.StdEncoding.EncodeToString(encoded)
	// The dynamic value is base64 only; endpoints are decoded as data, never shell syntax.
	return "CYLISM_ENDPOINTS_B64=" + data + " sh -c 'printf %s \"$CYLISM_ENDPOINTS_B64\" | base64 -d | tr -d \"[]\\\" \" | tr \",\" \"\\n\" | while IFS= read -r endpoint || [ -n \"$endpoint\" ]; do host=$(printf %s \"$endpoint\" | sed -E \"s#https?://([^/:]+).*#\\1#\"); dns=failed; getent ahosts \"$host\" >/dev/null 2>&1 && dns=ok; http=$(curl -ksS -o /dev/null -w \"%{http_code}\" --connect-timeout 5 --max-time 10 \"$endpoint/v2/\" 2>/dev/null || true); [ -n \"$http\" ] || http=failed; printf \"%s|%s|%s\\n\" \"$endpoint\" \"$dns\" \"$http\"; done'"
}

func agentRegistryPullCommand(image string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(image))
	return "image=$(printf %s " + encoded + " | base64 -d); " +
		"if [ -x /usr/local/bin/crictl ]; then sudo -n /usr/local/bin/crictl pull \"$image\"; " +
		"elif [ -x /var/lib/rancher/k3s/bin/crictl ]; then sudo -n /var/lib/rancher/k3s/bin/crictl pull \"$image\"; " +
		"elif [ -x /usr/local/bin/k3s ]; then sudo -n /usr/local/bin/k3s crictl pull \"$image\"; " +
		"else echo '未找到 crictl 或 k3s 命令' >&2; exit 127; fi"
}

func defaultAgentRegistryNodeVerifier(encKey []byte) agentRegistryNodeVerifier {
	return func(server *model.Server, endpoints []string) ([]agentRegistryEndpointResult, error) {
		if server == nil || len(endpoints) == 0 {
			return nil, fmt.Errorf("node or endpoint unavailable")
		}
		args := infrastructureapi.BuildSSHArgs(server, encKey, server.Host)
		args = append(args, agentRegistryVerificationCommand(endpoints))
		output, err := infrastructureapi.SSHExec(30*time.Second, args)
		if err != nil {
			return nil, fmt.Errorf("node verification command failed: %w: %s", err, agentRegistryVerificationOutputDetail(string(output)))
		}
		return parseAgentRegistryEndpointResults(string(output), len(endpoints))
	}
}

func parseAgentRegistryEndpointResults(output string, expectedCount int) ([]agentRegistryEndpointResult, error) {
	results := make([]agentRegistryEndpointResult, 0, expectedCount)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		parts := strings.Split(line, "|")
		if len(parts) == 3 && safeRegistryEndpoint(parts[0]) != "" {
			results = append(results, agentRegistryEndpointResult{Endpoint: safeRegistryEndpoint(parts[0]), DNS: strings.TrimSpace(parts[1]), HTTP: strings.TrimSpace(parts[2])})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Endpoint < results[j].Endpoint })
	if expectedCount > 0 && len(results) == 0 {
		return nil, fmt.Errorf("node verification returned no endpoint results: %s", agentRegistryVerificationOutputDetail(output))
	}
	return results, nil
}

func agentRegistryVerificationOutputDetail(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return "no remote output"
	}
	return truncateAgentText(output, 512)
}

func defaultAgentRegistryPullExecutor(encKey []byte) agentRegistryPullExecutor {
	return func(server *model.Server, image string) error {
		if server == nil || strings.TrimSpace(image) == "" {
			return fmt.Errorf("node or verification image unavailable")
		}
		args := infrastructureapi.BuildSSHArgs(server, encKey, server.Host)
		args = append(args, agentRegistryPullCommand(image))
		output, err := infrastructureapi.SSHExec(2*time.Minute, args)
		if err != nil {
			detail := strings.TrimSpace(string(output))
			if detail == "" {
				return fmt.Errorf("verification image pull failed: %w", err)
			}
			return fmt.Errorf("verification image pull failed: %w: %s", err, detail)
		}
		return nil
	}
}
