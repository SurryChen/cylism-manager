package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

type SSHExecutor interface {
	Execute(context.Context, time.Duration, *model.Server, string) ([]byte, error)
}
type SSHExecutorFunc func(context.Context, time.Duration, *model.Server, string) ([]byte, error)

func (f SSHExecutorFunc) Execute(ctx context.Context, timeout time.Duration, server *model.Server, command string) ([]byte, error) {
	return f(ctx, timeout, server, command)
}

type EndpointResult struct {
	Endpoint string `json:"endpoint"`
	DNS      string `json:"dns"`
	HTTP     string `json:"http"`
}
type NodeVerifier interface {
	Verify(context.Context, *model.Server, []string) ([]EndpointResult, error)
}
type PullExecutor interface {
	Pull(context.Context, *model.Server, string) error
}
type NodeVerifierFunc func(context.Context, *model.Server, []string) ([]EndpointResult, error)

func (f NodeVerifierFunc) Verify(ctx context.Context, server *model.Server, endpoints []string) ([]EndpointResult, error) {
	return f(ctx, server, endpoints)
}

type PullExecutorFunc func(context.Context, *model.Server, string) error

func (f PullExecutorFunc) Pull(ctx context.Context, server *model.Server, image string) error {
	return f(ctx, server, image)
}

type RegistryConfig struct {
	Registry          string
	Endpoints         []string
	VerificationImage string
	Mirror            *model.NodeRegistryMirror
	Proxy             *model.RegistryProxy
}

func SafeEndpoints(raw string) []string {
	var values []string
	if json.Unmarshal([]byte(raw), &values) != nil {
		return []string{}
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if safe := SafeEndpoint(value); safe != "" {
			result = append(result, safe)
		}
	}
	return result
}
func SafeEndpoint(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}
func ResolveRegistry(registry string, mirrors []model.NodeRegistryMirror, proxies []model.RegistryProxy) (RegistryConfig, bool) {
	registry = NormalizeRegistryHost(registry)
	for i := range mirrors {
		mirror := &mirrors[i]
		if mirror.Enabled && NormalizeRegistryHost(mirror.Registry) == registry {
			endpoints := SafeEndpoints(mirror.Endpoints)
			if len(endpoints) > 0 {
				return RegistryConfig{Registry: registry, Endpoints: endpoints, VerificationImage: strings.TrimSpace(mirror.VerificationImage), Mirror: mirror}, true
			}
		}
	}
	for i := range proxies {
		proxy := &proxies[i]
		if NormalizeRegistryHost(proxy.Registry) == registry && proxy.EndpointHost != "" && proxy.NodePort > 0 {
			return RegistryConfig{Registry: registry, Endpoints: []string{SafeEndpoint("http://" + proxy.EndpointHost + fmt.Sprintf(":%d", proxy.NodePort))}, Proxy: proxy}, true
		}
	}
	return RegistryConfig{}, false
}
func ImageRegistry(image string) string {
	parts := strings.Split(strings.TrimSpace(image), "/")
	if len(parts) < 2 || (!strings.Contains(parts[0], ".") && !strings.Contains(parts[0], ":") && parts[0] != "localhost") {
		return "docker.io"
	}
	return NormalizeRegistryHost(parts[0])
}
func ImagePullFailures(containers []map[string]any) []map[string]string {
	failures := make([]map[string]string, 0)
	for _, container := range containers {
		reason, _ := container["reason"].(string)
		if reason != "ErrImagePull" && reason != "ImagePullBackOff" && reason != "InvalidImageName" {
			continue
		}
		image, _ := container["image"].(string)
		message, _ := container["message"].(string)
		failures = append(failures, map[string]string{"container": fmt.Sprint(container["name"]), "image": image, "registry": ImageRegistry(image), "reason": reason, "message": message})
	}
	return failures
}

func VerificationCommand(endpoints []string) string {
	encoded, _ := json.Marshal(endpoints)
	data := base64.StdEncoding.EncodeToString(encoded)
	return "CYLISM_ENDPOINTS_B64=" + data + " sh -c 'printf %s \"$CYLISM_ENDPOINTS_B64\" | base64 -d | tr -d \"[]\\\" \" | tr \",\" \"\\n\" | while IFS= read -r endpoint || [ -n \"$endpoint\" ]; do host=$(printf %s \"$endpoint\" | sed -E \"s#https?://([^/:]+).*#\\1#\"); dns=failed; getent ahosts \"$host\" >/dev/null 2>&1 && dns=ok; http=$(curl -ksS -o /dev/null -w \"%{http_code}\" --connect-timeout 5 --max-time 10 \"$endpoint/v2/\" 2>/dev/null || true); [ -n \"$http\" ] || http=failed; printf \"%s|%s|%s\\n\" \"$endpoint\" \"$dns\" \"$http\"; done'"
}
func PullCommand(image string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(image))
	return "image=$(printf %s " + encoded + " | base64 -d); if [ -x /usr/local/bin/crictl ]; then sudo -n /usr/local/bin/crictl pull \"$image\"; elif [ -x /var/lib/rancher/k3s/bin/crictl ]; then sudo -n /var/lib/rancher/k3s/bin/crictl pull \"$image\"; elif [ -x /usr/local/bin/k3s ]; then sudo -n /usr/local/bin/k3s crictl pull \"$image\"; else echo '未找到 crictl 或 k3s 命令' >&2; exit 127; fi"
}
func ParseEndpointResults(output string, expected int) ([]EndpointResult, error) {
	results := make([]EndpointResult, 0, expected)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		parts := strings.Split(line, "|")
		if len(parts) == 3 && SafeEndpoint(parts[0]) != "" {
			results = append(results, EndpointResult{Endpoint: SafeEndpoint(parts[0]), DNS: strings.TrimSpace(parts[1]), HTTP: strings.TrimSpace(parts[2])})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Endpoint < results[j].Endpoint })
	if expected > 0 && len(results) == 0 {
		return nil, fmt.Errorf("node verification returned no endpoint results: %s", VerificationOutputDetail(output))
	}
	return results, nil
}
func VerificationOutputDetail(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return "no remote output"
	}
	if len(output) > 512 {
		return output[:512]
	}
	return output
}

type NodeVerifierService struct{ ssh SSHExecutor }

func NewNodeVerifierService(ssh SSHExecutor) *NodeVerifierService {
	return &NodeVerifierService{ssh: ssh}
}
func (s *NodeVerifierService) Verify(ctx context.Context, server *model.Server, endpoints []string) ([]EndpointResult, error) {
	if server == nil || len(endpoints) == 0 {
		return nil, fmt.Errorf("node or endpoint unavailable")
	}
	if s == nil || s.ssh == nil {
		return nil, fmt.Errorf("node verification unavailable")
	}
	output, err := s.ssh.Execute(ctx, 30*time.Second, server, VerificationCommand(endpoints))
	if err != nil {
		return nil, fmt.Errorf("node verification command failed: %w: %s", err, VerificationOutputDetail(string(output)))
	}
	return ParseEndpointResults(string(output), len(endpoints))
}

type NodePullService struct{ ssh SSHExecutor }

func NewNodePullService(ssh SSHExecutor) *NodePullService { return &NodePullService{ssh: ssh} }
func (s *NodePullService) Pull(ctx context.Context, server *model.Server, image string) error {
	if server == nil || strings.TrimSpace(image) == "" {
		return fmt.Errorf("node or verification image unavailable")
	}
	if s == nil || s.ssh == nil {
		return fmt.Errorf("verification image pull unavailable")
	}
	output, err := s.ssh.Execute(ctx, 2*time.Minute, server, PullCommand(image))
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			return fmt.Errorf("verification image pull failed: %w", err)
		}
		return fmt.Errorf("verification image pull failed: %w: %s", err, detail)
	}
	return nil
}
