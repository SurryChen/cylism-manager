package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
)

const DefaultRegistryProxyResourceName = "cylism-registry-proxy"

// ProxyInput is the transport-independent proxy configuration accepted by the
// Registry service. It deliberately excludes database and HTTP state.
type ProxyInput struct {
	Name                 string
	Registry             string
	UpstreamURL          string
	NodeName             string
	EndpointHost         string
	NodePort             int32
	CacheLimitGi         int32
	CleanupIntervalHours int32
	HTTPProxy            string
	HTTPSProxy           string
	NoProxy              string
	DNSServers           []string
	ClearOutboundProxy   bool
}

// ProxyService owns validation, credential handling and persistence for
// Registry proxy configuration. Kubernetes execution stays behind an adapter.
type ProxyService struct {
	repository repository.RegistryProxyRepository
	encKey     []byte
}

func NewProxyService(repository repository.RegistryProxyRepository, encKey []byte) *ProxyService {
	return &ProxyService{repository: repository, encKey: encKey}
}

func (s *ProxyService) GetLegacy() (*model.RegistryProxy, error) {
	proxy, err := s.repository.GetRegistryProxy()
	if err != nil {
		return nil, err
	}
	return s.normalizeAndSave(proxy)
}

func (s *ProxyService) Get(id uint) (*model.RegistryProxy, error) {
	proxy, err := s.repository.GetRegistryProxyByID(id)
	if err != nil {
		return nil, err
	}
	return s.normalizeAndSave(proxy)
}

func (s *ProxyService) List() ([]model.RegistryProxy, error) {
	proxies, err := s.repository.ListRegistryProxies()
	if err != nil {
		return nil, err
	}
	for index := range proxies {
		if NormalizeRegistryProxy(&proxies[index]) {
			_ = s.repository.SaveRegistryProxy(&proxies[index])
		}
	}
	return proxies, nil
}

// PrepareDeployment validates and persists a desired proxy configuration. The
// caller then hands the returned, non-redacted model to its K8s adapter.
func (s *ProxyService) PrepareDeployment(input ProxyInput, current *model.RegistryProxy, createdBy uint) (*model.RegistryProxy, error) {
	if err := ValidateProxyInput(input); err != nil {
		return nil, err
	}
	dnsServers, err := NormalizeProxyDNSServers(input.DNSServers)
	if err != nil {
		return nil, err
	}
	proxyID := uint(0)
	if current != nil {
		proxyID = current.ID
	}
	if err := s.validateNodePortAvailable(proxyID, input.NodePort); err != nil {
		return nil, err
	}
	if current == nil {
		current = &model.RegistryProxy{CreatedBy: createdBy}
	} else {
		NormalizeRegistryProxy(current)
	}
	current.Name = strings.TrimSpace(input.Name)
	current.Registry = NormalizeRegistryHost(input.Registry)
	current.UpstreamURL = NormalizedProxyUpstream(input)
	current.NodeName = strings.TrimSpace(input.NodeName)
	current.EndpointHost = strings.TrimSpace(input.EndpointHost)
	current.NodePort = input.NodePort
	current.CacheLimitGi = input.CacheLimitGi
	current.CleanupIntervalHours = input.CleanupIntervalHours
	if len(dnsServers) == 0 {
		current.DNSResolvers = ""
	} else {
		encoded, _ := json.Marshal(dnsServers)
		current.DNSResolvers = string(encoded)
	}
	if err := s.setOutboundProxy(current, input); err != nil {
		return nil, err
	}
	current.Status, current.LastError = "deploying", ""
	if err := s.repository.SaveRegistryProxy(current); err != nil {
		return nil, err
	}
	if current.ResourceName == "" {
		current.ResourceName = ProxyResourceName(current)
		if err := s.repository.SaveRegistryProxy(current); err != nil {
			return nil, err
		}
	}
	current.OutboundProxyConfigured = current.EncryptedHTTPProxy != "" || current.EncryptedHTTPSProxy != ""
	current.DNSServers = ProxyDNSServers(current)
	return current, nil
}

func (s *ProxyService) MarkFailed(proxy *model.RegistryProxy, cause error) {
	proxy.Status, proxy.LastError = "failed", cause.Error()
	_ = s.repository.SaveRegistryProxy(proxy)
}

func (s *ProxyService) Save(proxy *model.RegistryProxy) error {
	return s.repository.SaveRegistryProxy(proxy)
}

func (s *ProxyService) RefreshCheck(proxy *model.RegistryProxy) error {
	now := time.Now()
	proxy.LastCheckedAt = &now
	return s.repository.SaveRegistryProxy(proxy)
}

func (s *ProxyService) ProxyEnvironment(proxy *model.RegistryProxy) (map[string]string, error) {
	environment := map[string]string{
		"REGISTRY_PROXY_REMOTEURL":                  proxy.UpstreamURL,
		"REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY": "/var/lib/registry",
	}
	for _, configured := range []struct{ name, value string }{{"HTTP_PROXY", proxy.EncryptedHTTPProxy}, {"HTTPS_PROXY", proxy.EncryptedHTTPSProxy}} {
		if configured.value == "" {
			continue
		}
		if len(s.encKey) != 32 {
			return nil, errors.New("平台加密密钥不可用，无法读取出网代理")
		}
		value, err := crypto.Decrypt(s.encKey, configured.value)
		if err != nil {
			return nil, errors.New("读取出网代理失败")
		}
		environment[configured.name] = value
	}
	if proxy.NoProxy != "" {
		environment["NO_PROXY"] = proxy.NoProxy
	}
	return environment, nil
}

func (s *ProxyService) normalizeAndSave(proxy *model.RegistryProxy) (*model.RegistryProxy, error) {
	if NormalizeRegistryProxy(proxy) {
		if err := s.repository.SaveRegistryProxy(proxy); err != nil {
			return nil, err
		}
	}
	return proxy, nil
}

func (s *ProxyService) setOutboundProxy(proxy *model.RegistryProxy, input ProxyInput) error {
	if input.ClearOutboundProxy {
		proxy.EncryptedHTTPProxy, proxy.EncryptedHTTPSProxy, proxy.NoProxy = "", "", ""
		return nil
	}
	for _, configured := range []struct{ raw, target string }{{input.HTTPProxy, "http"}, {input.HTTPSProxy, "https"}} {
		if strings.TrimSpace(configured.raw) == "" {
			continue
		}
		if err := ValidateOutboundProxyURL(configured.raw); err != nil {
			return err
		}
		if len(s.encKey) != 32 {
			return errors.New("平台加密密钥不可用，无法保存出网代理")
		}
		encoded, err := crypto.Encrypt(s.encKey, strings.TrimSpace(configured.raw))
		if err != nil {
			return fmt.Errorf("加密 %s 出网代理失败", strings.ToUpper(configured.target))
		}
		if configured.target == "http" {
			proxy.EncryptedHTTPProxy = encoded
		} else {
			proxy.EncryptedHTTPSProxy = encoded
		}
	}
	if input.HTTPProxy == "" && input.HTTPSProxy == "" && proxy.ID == 0 {
		proxy.EncryptedHTTPProxy, proxy.EncryptedHTTPSProxy = "", ""
	}
	if len(input.NoProxy) > 1024 || strings.ContainsAny(input.NoProxy, "\r\n") {
		return errors.New("NO_PROXY 配置无效")
	}
	if input.NoProxy != "" || proxy.ID == 0 {
		proxy.NoProxy = strings.TrimSpace(input.NoProxy)
	}
	return nil
}

func (s *ProxyService) validateNodePortAvailable(proxyID uint, nodePort int32) error {
	proxies, err := s.repository.ListRegistryProxies()
	if err != nil {
		return fmt.Errorf("读取现有镜像代理失败: %w", err)
	}
	for _, existing := range proxies {
		if existing.ID != proxyID && existing.NodePort == nodePort {
			return fmt.Errorf("NodePort %d 已被镜像代理 %q 使用", nodePort, existing.Name)
		}
	}
	return nil
}

func ValidateProxyInput(input ProxyInput) error {
	if strings.TrimSpace(input.Name) == "" || len(strings.TrimSpace(input.Name)) > 128 {
		return errors.New("代理名称不能为空且不能超过 128 个字符")
	}
	registry := NormalizeRegistryHost(input.Registry)
	if registry == "" || strings.Contains(registry, "/") || net.ParseIP(registry) != nil {
		return errors.New("Registry 必须是镜像仓库域名，例如 registry.k8s.io")
	}
	upstream, err := url.Parse(NormalizedProxyUpstream(input))
	if err != nil || upstream.Scheme != "https" || upstream.Hostname() == "" || upstream.User != nil || upstream.RawQuery != "" || upstream.Fragment != "" || (upstream.Path != "" && upstream.Path != "/") {
		return errors.New("上游地址必须是无路径、无认证信息的 HTTPS Registry 地址")
	}
	if registry != "docker.io" && !strings.EqualFold(upstream.Hostname(), registry) {
		return errors.New("非 Docker Hub 代理的上游地址必须与 Registry 域名一致")
	}
	if strings.TrimSpace(input.NodeName) == "" || !PrivateOrTailnetIP(strings.TrimSpace(input.EndpointHost)) {
		return errors.New("代理地址必须是节点间可访问的私网或 Tailscale IP")
	}
	if input.NodePort < 30000 || input.NodePort > 32767 {
		return errors.New("NodePort 必须在 30000 到 32767 之间")
	}
	if input.CacheLimitGi < 1 || input.CacheLimitGi > 100 {
		return errors.New("临时缓存上限必须在 1 到 100 Gi 之间")
	}
	if input.CleanupIntervalHours < 1 || input.CleanupIntervalHours > 168 {
		return errors.New("清理周期必须在 1 到 168 小时之间")
	}
	return nil
}

func NormalizeProxyDNSServers(raw []string) ([]string, error) {
	if len(raw) > 3 {
		return nil, errors.New("代理 DNS 最多配置 3 个地址")
	}
	seen, result := map[string]bool{}, make([]string, 0, len(raw))
	for _, value := range raw {
		ip := net.ParseIP(strings.TrimSpace(value))
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
			return nil, errors.New("代理 DNS 必须是可路由的 IP 地址，不能使用 127.0.0.53")
		}
		value = ip.String()
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result, nil
}

func ValidateOutboundProxyURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.RawQuery != "" || parsed.Fragment != "" || len(raw) > 512 {
		return errors.New("出网代理必须是无查询参数的 HTTP 或 HTTPS 地址")
	}
	return nil
}

func NormalizeRegistryHost(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "index.docker.io" || value == "registry-1.docker.io" {
		return "docker.io"
	}
	return value
}

func NormalizedProxyUpstream(input ProxyInput) string {
	if upstream := strings.TrimSpace(input.UpstreamURL); upstream != "" {
		return strings.TrimSuffix(upstream, "/")
	}
	if NormalizeRegistryHost(input.Registry) == "docker.io" {
		return "https://registry-1.docker.io"
	}
	return "https://" + NormalizeRegistryHost(input.Registry)
}

func NormalizeRegistryProxy(proxy *model.RegistryProxy) bool {
	changed := false
	if strings.TrimSpace(proxy.Registry) == "" {
		proxy.Registry, changed = "docker.io", true
	}
	if strings.TrimSpace(proxy.UpstreamURL) == "" {
		proxy.UpstreamURL, changed = "https://registry-1.docker.io", true
	}
	if strings.TrimSpace(proxy.Name) == "" {
		proxy.Name, changed = "Docker Hub 代理", true
	}
	if strings.TrimSpace(proxy.ResourceName) == "" {
		proxy.ResourceName, changed = DefaultRegistryProxyResourceName, true
	}
	return changed
}

func ProxyResourceName(proxy *model.RegistryProxy) string {
	if name := strings.TrimSpace(proxy.ResourceName); name != "" {
		return name
	}
	return DefaultRegistryProxyResourceName + "-" + strconv.Itoa(int(proxy.ID))
}

func ProxyDNSServers(proxy *model.RegistryProxy) []string {
	if proxy.DNSResolvers == "" {
		return nil
	}
	var servers []string
	if json.Unmarshal([]byte(proxy.DNSResolvers), &servers) != nil {
		return nil
	}
	return servers
}

func PrivateOrTailnetIP(value string) bool {
	ip := net.ParseIP(value)
	if ip == nil || ip.IsLoopback() {
		return false
	}
	if ip.IsPrivate() {
		return true
	}
	_, tailnet, _ := net.ParseCIDR("100.64.0.0/10")
	return tailnet.Contains(ip)
}
