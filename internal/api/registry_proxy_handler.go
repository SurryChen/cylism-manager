package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

const registryProxyNamespace = "kube-system"
const registryProxyName = "cylism-registry-proxy"

type RegistryProxyHandler struct {
	store     *store.Store
	encKey    []byte
	diagnoser registryProxyDiagnoser
}

type registryProxyRequest struct {
	Name                 string   `json:"name"`
	Registry             string   `json:"registry"`
	UpstreamURL          string   `json:"upstream_url"`
	NodeName             string   `json:"node_name"`
	EndpointHost         string   `json:"endpoint_host"`
	NodePort             int32    `json:"node_port"`
	CacheLimitGi         int32    `json:"cache_limit_gi"`
	CleanupIntervalHours int32    `json:"cleanup_interval_hours"`
	HTTPProxy            string   `json:"http_proxy"`
	HTTPSProxy           string   `json:"https_proxy"`
	NoProxy              string   `json:"no_proxy"`
	DNSServers           []string `json:"dns_servers"`
	ClearOutboundProxy   bool     `json:"clear_outbound_proxy"`
}

type registryProxyDiagnostic struct {
	Status      string   `json:"status"`
	ResolvedIPs []string `json:"resolved_ips"`
	HTTPStatus  string   `json:"http_status,omitempty"`
	ElapsedMS   int64    `json:"elapsed_ms"`
	Summary     string   `json:"summary"`
}

type registryProxyDiagnoser func(context.Context, *model.RegistryProxy) (registryProxyDiagnostic, error)

func NewRegistryProxyHandler(s *store.Store, encKey ...[]byte) *RegistryProxyHandler {
	h := &RegistryProxyHandler{store: s}
	if len(encKey) > 0 {
		h.encKey = encKey[0]
	}
	h.diagnoser = h.diagnoseFromProxyPod
	return h
}

func (h *RegistryProxyHandler) WithDiagnoser(diagnoser registryProxyDiagnoser) *RegistryProxyHandler {
	h.diagnoser = diagnoser
	return h
}

func (h *RegistryProxyHandler) Get(c *gin.Context) {
	proxy, err := h.store.GetRegistryProxy()
	if apierrors.IsNotFound(err) || err != nil && strings.Contains(err.Error(), "record not found") {
		model.Success(c, nil)
		return
	}
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取镜像代理失败")
		return
	}
	if h.normalize(proxy) {
		_ = h.store.SaveRegistryProxy(proxy)
	}
	h.refreshStatus(c.Request.Context(), proxy)
	h.redactProxy(proxy)
	model.Success(c, proxy)
}

func (h *RegistryProxyHandler) List(c *gin.Context) {
	proxies, err := h.store.ListRegistryProxies()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取镜像代理失败")
		return
	}
	for index := range proxies {
		if h.normalize(&proxies[index]) {
			_ = h.store.SaveRegistryProxy(&proxies[index])
		}
		h.refreshStatus(c.Request.Context(), &proxies[index])
		h.redactProxy(&proxies[index])
	}
	model.Success(c, proxies)
}

func (h *RegistryProxyHandler) Deploy(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	var req registryProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "代理配置无效")
		return
	}
	if isLegacyRegistryProxyRoute(c) {
		applyDockerHubProxyDefaults(&req)
	}
	if err := validateRegistryProxyRequest(req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	dnsServers, err := normalizeProxyDNSServers(req.DNSServers)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if _, err := K8s.Clientset.CoreV1().Nodes().Get(c.Request.Context(), req.NodeName, metav1.GetOptions{}); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "部署节点不存在或未加入集群")
		return
	}
	proxy, err := h.proxyForRequest(c)
	if err != nil {
		if c.Param("id") != "" {
			model.Error(c, http.StatusNotFound, model.CodeNotFound, "镜像代理不存在")
			return
		}
		proxy = &model.RegistryProxy{CreatedBy: getUserID(c)}
	} else {
		h.normalize(proxy)
	}
	if err := h.validateNodePortAvailable(proxy.ID, req.NodePort); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	proxy.Name, proxy.Registry, proxy.UpstreamURL = strings.TrimSpace(req.Name), normalizeRegistry(req.Registry), normalizedRegistryProxyUpstream(req)
	proxy.NodeName, proxy.EndpointHost, proxy.NodePort = req.NodeName, req.EndpointHost, req.NodePort
	proxy.CacheLimitGi, proxy.CleanupIntervalHours = req.CacheLimitGi, req.CleanupIntervalHours
	if len(dnsServers) == 0 {
		proxy.DNSResolvers = ""
	} else {
		encoded, _ := json.Marshal(dnsServers)
		proxy.DNSResolvers = string(encoded)
	}
	if err := h.setOutboundProxy(proxy, req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	proxy.Status, proxy.LastError = "deploying", ""
	if err := h.store.SaveRegistryProxy(proxy); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存代理配置失败")
		return
	}
	if proxy.ResourceName == "" {
		proxy.ResourceName = registryProxyName + "-" + strconv.Itoa(int(proxy.ID))
		if err := h.store.SaveRegistryProxy(proxy); err != nil {
			model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存镜像代理资源标识失败")
			return
		}
	}
	if err := h.apply(c.Request.Context(), proxy); err != nil {
		proxy.Status, proxy.LastError = "failed", err.Error()
		_ = h.store.SaveRegistryProxy(proxy)
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "部署镜像代理失败: "+err.Error())
		return
	}
	h.redactProxy(proxy)
	model.SuccessWithMessage(c, proxy, "镜像代理已提交部署，稍后可在节点镜像源中使用该地址")
}

func isLegacyRegistryProxyRoute(c *gin.Context) bool {
	return strings.HasPrefix(c.FullPath(), "/api/registry-proxy/")
}

func applyDockerHubProxyDefaults(req *registryProxyRequest) {
	if strings.TrimSpace(req.Name) == "" {
		req.Name = "Docker Hub 代理"
	}
	if strings.TrimSpace(req.Registry) == "" {
		req.Registry = "docker.io"
	}
}

func (h *RegistryProxyHandler) Cleanup(c *gin.Context) {
	proxy, err := h.proxyForRequest(c)
	if err != nil || K8s == nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "镜像代理不存在或集群未连接")
		return
	}
	if err := h.clearCache(c.Request.Context(), proxy, "手动清理"); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	model.SuccessWithMessage(c, proxy, "代理 Pod 已重建，临时缓存正在清理")
}

// Diagnose probes only the configured upstream from a Ready Pod belonging to
// this managed proxy. It accepts no caller-controlled network target or argv.
func (h *RegistryProxyHandler) Diagnose(c *gin.Context) {
	proxy, err := h.proxyForRequest(c)
	if err != nil || K8s == nil || K8s.Clientset == nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "镜像代理不存在或集群未连接")
		return
	}
	h.normalize(proxy)
	diagnostic, err := h.diagnoser(c.Request.Context(), proxy)
	if err != nil {
		proxy.LastDiagnosticStatus = "diagnostic_failed"
		proxy.LastDiagnosticError = "代理出网诊断失败"
		now := time.Now()
		proxy.LastDiagnosticAt = &now
		_ = h.store.SaveRegistryProxy(proxy)
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "代理出网诊断失败")
		return
	}
	proxy.LastDiagnosticStatus = diagnostic.Status
	proxy.LastDiagnosticError = truncateProxyDiagnosticText(diagnostic.Summary)
	now := time.Now()
	proxy.LastDiagnosticAt = &now
	if err := h.store.SaveRegistryProxy(proxy); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存代理诊断结果失败")
		return
	}
	model.Success(c, diagnostic)
}

// MigrateResourceName recreates the legacy Docker Hub resources using the per-instance naming scheme.
func (h *RegistryProxyHandler) MigrateResourceName(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	proxy, err := h.proxyForRequest(c)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "镜像代理不存在")
		return
	}
	h.normalize(proxy)
	legacyResourceName := proxyResourceName(proxy)
	newResourceName := registryProxyName + "-" + strconv.Itoa(int(proxy.ID))
	if legacyResourceName == newResourceName {
		model.SuccessWithMessage(c, proxy, "镜像代理已使用新资源命名")
		return
	}
	if legacyResourceName != registryProxyName || proxy.Registry != "docker.io" {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "仅支持迁移旧 Docker Hub 代理资源")
		return
	}

	ctx := c.Request.Context()
	if err := K8s.Clientset.CoreV1().Services(registryProxyNamespace).Delete(ctx, legacyResourceName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "删除旧代理 Service 失败: "+err.Error())
		return
	}
	if err := K8s.Clientset.AppsV1().Deployments(registryProxyNamespace).Delete(ctx, legacyResourceName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "删除旧代理 Deployment 失败: "+err.Error())
		return
	}
	proxy.ResourceName, proxy.Status, proxy.LastError = newResourceName, "deploying", ""
	if err := h.store.SaveRegistryProxy(proxy); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存迁移后的代理配置失败")
		return
	}
	if err := h.apply(ctx, proxy); err != nil {
		proxy.Status, proxy.LastError = "failed", "使用新资源名重建失败: "+err.Error()
		_ = h.store.SaveRegistryProxy(proxy)
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, proxy.LastError)
		return
	}
	model.SuccessWithMessage(c, proxy, "旧 Docker Hub 代理已按新资源名重建，等待新 Pod 就绪")
}

func (h *RegistryProxyHandler) Reconcile() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if K8s == nil {
			continue
		}
		proxies, err := h.store.ListRegistryProxies()
		if err == nil {
			for index := range proxies {
				h.refreshStatus(context.Background(), &proxies[index])
			}
		}
	}
}

func (h *RegistryProxyHandler) refreshStatus(ctx context.Context, proxy *model.RegistryProxy) {
	if K8s == nil {
		return
	}
	resourceName := proxyResourceName(proxy)
	deployment, err := K8s.Clientset.AppsV1().Deployments(registryProxyNamespace).Get(ctx, resourceName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		proxy.Status, proxy.LastError = "missing", "代理 Deployment 不存在"
	} else if err != nil {
		proxy.Status, proxy.LastError = "failed", "读取代理状态失败: "+err.Error()
	} else if deployment.Status.AvailableReplicas > 0 {
		proxy.Status, proxy.LastError = "ready", ""
		if proxy.LastCleanupAt == nil {
			now := time.Now()
			proxy.LastCleanupAt = &now
		} else if time.Since(*proxy.LastCleanupAt) >= time.Duration(proxy.CleanupIntervalHours)*time.Hour {
			_ = h.clearCache(ctx, proxy, "定期清理")
		}
	} else {
		proxy.Status = "deploying"
	}
	now := time.Now()
	proxy.LastCheckedAt = &now
	_ = h.store.SaveRegistryProxy(proxy)
}

func (h *RegistryProxyHandler) redactProxy(proxy *model.RegistryProxy) {
	proxy.OutboundProxyConfigured = proxy.EncryptedHTTPProxy != "" || proxy.EncryptedHTTPSProxy != ""
	proxy.DNSServers = proxyDNSServers(proxy)
	proxy.EncryptedHTTPProxy = ""
	proxy.EncryptedHTTPSProxy = ""
}

func proxyDNSServers(proxy *model.RegistryProxy) []string {
	if proxy.DNSResolvers == "" {
		return nil
	}
	var servers []string
	if json.Unmarshal([]byte(proxy.DNSResolvers), &servers) != nil {
		return nil
	}
	return servers
}

func normalizeProxyDNSServers(raw []string) ([]string, error) {
	if len(raw) > 3 {
		return nil, fmt.Errorf("代理 DNS 最多配置 3 个地址")
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(raw))
	for _, value := range raw {
		value = strings.TrimSpace(value)
		ip := net.ParseIP(value)
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
			return nil, fmt.Errorf("代理 DNS 必须是可路由的 IP 地址，不能使用 127.0.0.53")
		}
		value = ip.String()
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result, nil
}

func (h *RegistryProxyHandler) setOutboundProxy(proxy *model.RegistryProxy, req registryProxyRequest) error {
	if req.ClearOutboundProxy {
		proxy.EncryptedHTTPProxy, proxy.EncryptedHTTPSProxy, proxy.NoProxy = "", "", ""
		return nil
	}
	if strings.TrimSpace(req.HTTPProxy) != "" {
		if err := validateOutboundProxyURL(req.HTTPProxy); err != nil {
			return err
		}
		if len(h.encKey) != 32 {
			return fmt.Errorf("平台加密密钥不可用，无法保存出网代理")
		}
		encoded, err := crypto.Encrypt(h.encKey, strings.TrimSpace(req.HTTPProxy))
		if err != nil {
			return fmt.Errorf("加密 HTTP 出网代理失败")
		}
		proxy.EncryptedHTTPProxy = encoded
	}
	if strings.TrimSpace(req.HTTPSProxy) != "" {
		if err := validateOutboundProxyURL(req.HTTPSProxy); err != nil {
			return err
		}
		if len(h.encKey) != 32 {
			return fmt.Errorf("平台加密密钥不可用，无法保存出网代理")
		}
		encoded, err := crypto.Encrypt(h.encKey, strings.TrimSpace(req.HTTPSProxy))
		if err != nil {
			return fmt.Errorf("加密 HTTPS 出网代理失败")
		}
		proxy.EncryptedHTTPSProxy = encoded
	}
	if req.HTTPProxy == "" && req.HTTPSProxy == "" && proxy.ID == 0 {
		proxy.EncryptedHTTPProxy, proxy.EncryptedHTTPSProxy = "", ""
	}
	if len(req.NoProxy) > 1024 || strings.ContainsAny(req.NoProxy, "\r\n") {
		return fmt.Errorf("NO_PROXY 配置无效")
	}
	if req.NoProxy != "" || proxy.ID == 0 {
		proxy.NoProxy = strings.TrimSpace(req.NoProxy)
	}
	return nil
}

func validateOutboundProxyURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.RawQuery != "" || parsed.Fragment != "" || len(raw) > 512 {
		return fmt.Errorf("出网代理必须是无查询参数的 HTTP 或 HTTPS 地址")
	}
	return nil
}

func (h *RegistryProxyHandler) clearCache(ctx context.Context, proxy *model.RegistryProxy, reason string) error {
	if err := K8s.Clientset.CoreV1().Pods(registryProxyNamespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=" + proxyResourceName(proxy)}); err != nil {
		return fmt.Errorf("%s失败: %w", reason, err)
	}
	now := time.Now()
	proxy.LastCleanupAt, proxy.Status, proxy.LastError = &now, "deploying", reason+"后等待新 Pod 就绪"
	return h.store.SaveRegistryProxy(proxy)
}

func (h *RegistryProxyHandler) validateNodePortAvailable(proxyID uint, nodePort int32) error {
	proxies, err := h.store.ListRegistryProxies()
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

func (h *RegistryProxyHandler) apply(ctx context.Context, proxy *model.RegistryProxy) error {
	resourceName := proxyResourceName(proxy)
	labels := map[string]string{"app.kubernetes.io/managed-by": "cylism-manager", "app.kubernetes.io/name": resourceName, "cylism.io/registry": proxy.Registry}
	cacheLimit := resource.MustParse(strconv.Itoa(int(proxy.CacheLimitGi)) + "Gi")
	replicas := int32(1)
	env, err := h.proxyEnvironment(proxy)
	if err != nil {
		return err
	}
	podSpec := corev1.PodSpec{NodeSelector: map[string]string{corev1.LabelHostname: proxy.NodeName}, Volumes: []corev1.Volume{{Name: "cache", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: &cacheLimit}}}}, Containers: []corev1.Container{{Name: "registry", Image: "registry:2.8", Ports: []corev1.ContainerPort{{ContainerPort: 5000}}, Env: env, VolumeMounts: []corev1.VolumeMount{{Name: "cache", MountPath: "/var/lib/registry"}}, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("512Mi")}}, ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/v2/", Port: intstr.FromInt(5000)}}, InitialDelaySeconds: 3, PeriodSeconds: 5}}}}
	if dnsServers := proxyDNSServers(proxy); len(dnsServers) > 0 {
		ndots := "1"
		podSpec.DNSPolicy = corev1.DNSNone
		podSpec.DNSConfig = &corev1.PodDNSConfig{Nameservers: dnsServers, Options: []corev1.PodDNSConfigOption{{Name: "ndots", Value: &ndots}}}
	}
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: registryProxyNamespace, Labels: labels}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: labels}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: podSpec}}}
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: registryProxyNamespace, Labels: labels}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, Selector: labels, Ports: []corev1.ServicePort{{Name: "registry", Port: 5000, TargetPort: intstr.FromInt(5000), NodePort: proxy.NodePort}}}}
	if current, err := K8s.Clientset.AppsV1().Deployments(registryProxyNamespace).Get(ctx, resourceName, metav1.GetOptions{}); err == nil {
		deployment.ResourceVersion = current.ResourceVersion
		_, err = K8s.Clientset.AppsV1().Deployments(registryProxyNamespace).Update(ctx, deployment, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	} else if _, err := K8s.Clientset.AppsV1().Deployments(registryProxyNamespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
		return err
	}
	if current, err := K8s.Clientset.CoreV1().Services(registryProxyNamespace).Get(ctx, resourceName, metav1.GetOptions{}); err == nil {
		service.ResourceVersion = current.ResourceVersion
		_, err = K8s.Clientset.CoreV1().Services(registryProxyNamespace).Update(ctx, service, metav1.UpdateOptions{})
		return err
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	_, err = K8s.Clientset.CoreV1().Services(registryProxyNamespace).Create(ctx, service, metav1.CreateOptions{})
	return err
}

func (h *RegistryProxyHandler) proxyEnvironment(proxy *model.RegistryProxy) ([]corev1.EnvVar, error) {
	env := []corev1.EnvVar{{Name: "REGISTRY_PROXY_REMOTEURL", Value: proxy.UpstreamURL}, {Name: "REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY", Value: "/var/lib/registry"}}
	for _, configured := range []struct {
		name, value string
	}{{"HTTP_PROXY", proxy.EncryptedHTTPProxy}, {"HTTPS_PROXY", proxy.EncryptedHTTPSProxy}} {
		if configured.value == "" {
			continue
		}
		if len(h.encKey) != 32 {
			return nil, fmt.Errorf("平台加密密钥不可用，无法读取出网代理")
		}
		value, err := crypto.Decrypt(h.encKey, configured.value)
		if err != nil {
			return nil, fmt.Errorf("读取出网代理失败")
		}
		env = append(env, corev1.EnvVar{Name: configured.name, Value: value})
	}
	if proxy.NoProxy != "" {
		env = append(env, corev1.EnvVar{Name: "NO_PROXY", Value: proxy.NoProxy})
	}
	return env, nil
}

func (h *RegistryProxyHandler) diagnoseFromProxyPod(ctx context.Context, proxy *model.RegistryProxy) (registryProxyDiagnostic, error) {
	if K8s == nil || K8s.Clientset == nil || K8s.Config == nil {
		return registryProxyDiagnostic{}, fmt.Errorf("Kubernetes exec unavailable")
	}
	pods, err := K8s.Clientset.CoreV1().Pods(registryProxyNamespace).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=" + proxyResourceName(proxy)})
	if err != nil {
		return registryProxyDiagnostic{}, err
	}
	for index := range pods.Items {
		pod := &pods.Items[index]
		if !podReady(*pod) {
			continue
		}
		return h.execProxyDiagnostic(ctx, pod.Name, proxy.UpstreamURL)
	}
	return registryProxyDiagnostic{Status: "proxy_not_ready", Summary: "镜像代理没有就绪的 Pod"}, nil
}

func (h *RegistryProxyHandler) execProxyDiagnostic(ctx context.Context, podName, upstream string) (registryProxyDiagnostic, error) {
	parsed, err := url.Parse(upstream)
	if err != nil || parsed.Hostname() == "" {
		return registryProxyDiagnostic{}, fmt.Errorf("invalid upstream")
	}
	// The registry image includes BusyBox wget but not curl. Keep the command
	// fixed and pass the validated upstream host only as a positional argument.
	command := []string{"sh", "-c", `host=$1; started=$(date +%s%3N); ips=$(getent ahostsv4 "$host" 2>/dev/null | awk '{print $1}' | sort -u | head -8 | tr '\n' ','); tool=missing; output=; http=; if command -v curl >/dev/null 2>&1; then tool=curl; output=$(curl -ksS -o /dev/null -w '%{http_code}' --connect-timeout 5 --max-time 10 "https://$host/v2/" 2>&1); elif command -v wget >/dev/null 2>&1; then tool=wget; output=$(wget -S --no-check-certificate -T 10 -t 1 -O /dev/null "https://$host/v2/" 2>&1); http=$(printf '%s\n' "$output" | awk '/^  HTTP\// {code=$2} /^HTTP\// {code=$2} END {print code}'); fi; status=upstream_http_error; if [ "$tool" = missing ]; then status=command_missing; elif [ -z "$ips" ]; then status=dns_resolution_failed; elif [ "$http" = 200 ] || [ "$http" = 401 ]; then status=healthy; elif printf '%s' "$output" | grep -qiE 'timed out|connection timed out'; then status=upstream_connect_timeout; elif printf '%s' "$output" | grep -qiE 'certificate|tls|ssl'; then status=upstream_tls_failed; fi; elapsed=$(( $(date +%s%3N) - started )); printf 'status=%s;tool=%s;ips=%s;http=%s;elapsed=%s\n' "$status" "$tool" "$ips" "$http" "$elapsed"`, "diagnose", parsed.Hostname()}
	req := K8s.Clientset.CoreV1().RESTClient().Post().Resource("pods").Namespace(registryProxyNamespace).Name(podName).SubResource("exec").VersionedParams(&corev1.PodExecOptions{Container: "registry", Command: command, Stdout: true, Stderr: true}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(K8s.Config, http.MethodPost, req.URL())
	if err != nil {
		return registryProxyDiagnostic{}, err
	}
	var stdout, stderr bytes.Buffer
	started := time.Now()
	if err := executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: &stdout, Stderr: &stderr}); err != nil {
		return registryProxyDiagnostic{Status: "diagnostic_failed", ElapsedMS: time.Since(started).Milliseconds(), Summary: "代理 Pod 内探测命令失败"}, nil
	}
	return parseProxyDiagnostic(stdout.String(), time.Since(started).Milliseconds()), nil
}

func parseProxyDiagnostic(output string, fallbackElapsed int64) registryProxyDiagnostic {
	result := registryProxyDiagnostic{Status: "diagnostic_failed", ElapsedMS: fallbackElapsed, Summary: "代理 Pod 未返回有效诊断结果"}
	for _, field := range strings.Split(strings.TrimSpace(output), ";") {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "status":
			result.Status = strings.TrimSpace(parts[1])
		case "ips":
			for _, ip := range strings.Split(strings.TrimSuffix(parts[1], ","), ",") {
				if net.ParseIP(ip) != nil {
					result.ResolvedIPs = append(result.ResolvedIPs, ip)
				}
			}
		case "http":
			result.HTTPStatus = strings.TrimSpace(parts[1])
		case "elapsed":
			if value, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64); err == nil && value >= 0 {
				result.ElapsedMS = value
			}
		}
	}
	switch result.Status {
	case "healthy":
		result.Summary = "代理 Pod 可访问上游 Registry"
	case "dns_resolution_failed":
		result.Summary = "代理 Pod 未能解析上游 Registry"
	case "upstream_connect_timeout":
		result.Summary = "上游 Registry 连接超时或不可达"
	case "upstream_tls_failed":
		result.Summary = "上游 Registry TLS 握手或证书校验失败"
	case "command_missing":
		result.Summary = "代理镜像缺少可用的 HTTP 诊断命令"
	case "upstream_http_error":
		result.Summary = "上游 Registry 返回异常 HTTP 状态"
	default:
		result.Status, result.Summary = "diagnostic_failed", "代理 Pod 未返回有效诊断结果"
	}
	return result
}

func truncateProxyDiagnosticText(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 512 {
		return value[:512]
	}
	return value
}

func validateRegistryProxyRequest(req registryProxyRequest) error {
	if strings.TrimSpace(req.Name) == "" || len(strings.TrimSpace(req.Name)) > 128 {
		return fmt.Errorf("代理名称不能为空且不能超过 128 个字符")
	}
	registry := normalizeRegistry(req.Registry)
	if registry == "" || strings.Contains(registry, "/") || net.ParseIP(registry) != nil {
		return fmt.Errorf("Registry 必须是镜像仓库域名，例如 registry.k8s.io")
	}
	upstream, err := url.Parse(normalizedRegistryProxyUpstream(req))
	if err != nil || upstream.Scheme != "https" || upstream.Hostname() == "" || upstream.User != nil || upstream.RawQuery != "" || upstream.Fragment != "" || (upstream.Path != "" && upstream.Path != "/") {
		return fmt.Errorf("上游地址必须是无路径、无认证信息的 HTTPS Registry 地址")
	}
	if registry != "docker.io" && !strings.EqualFold(upstream.Hostname(), registry) {
		return fmt.Errorf("非 Docker Hub 代理的上游地址必须与 Registry 域名一致")
	}
	if strings.TrimSpace(req.NodeName) == "" || !privateOrTailnetIP(strings.TrimSpace(req.EndpointHost)) {
		return fmt.Errorf("代理地址必须是节点间可访问的私网或 Tailscale IP")
	}
	if req.NodePort < 30000 || req.NodePort > 32767 {
		return fmt.Errorf("NodePort 必须在 30000 到 32767 之间")
	}
	if req.CacheLimitGi < 1 || req.CacheLimitGi > 100 {
		return fmt.Errorf("临时缓存上限必须在 1 到 100 Gi 之间")
	}
	if req.CleanupIntervalHours < 1 || req.CleanupIntervalHours > 168 {
		return fmt.Errorf("清理周期必须在 1 到 168 小时之间")
	}
	return nil
}

func normalizeRegistry(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "index.docker.io" || value == "registry-1.docker.io" {
		return "docker.io"
	}
	return value
}

func normalizedRegistryProxyUpstream(req registryProxyRequest) string {
	if upstream := strings.TrimSpace(req.UpstreamURL); upstream != "" {
		return strings.TrimSuffix(upstream, "/")
	}
	if normalizeRegistry(req.Registry) == "docker.io" {
		return "https://registry-1.docker.io"
	}
	return "https://" + normalizeRegistry(req.Registry)
}

func (h *RegistryProxyHandler) proxyForRequest(c *gin.Context) (*model.RegistryProxy, error) {
	if rawID := strings.TrimSpace(c.Param("id")); rawID != "" {
		id, err := strconv.ParseUint(rawID, 10, 64)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("invalid proxy id")
		}
		return h.store.GetRegistryProxyByID(uint(id))
	}
	if !isLegacyRegistryProxyRoute(c) {
		return nil, fmt.Errorf("registry proxy id is required")
	}
	return h.store.GetRegistryProxy()
}

func (h *RegistryProxyHandler) normalize(proxy *model.RegistryProxy) bool {
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
		proxy.ResourceName, changed = registryProxyName, true
	}
	return changed
}

func proxyResourceName(proxy *model.RegistryProxy) string {
	if name := strings.TrimSpace(proxy.ResourceName); name != "" {
		return name
	}
	return registryProxyName + "-" + strconv.Itoa(int(proxy.ID))
}

func privateOrTailnetIP(value string) bool {
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
