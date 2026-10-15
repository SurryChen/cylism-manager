package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	security "github.com/cylism/cylism-manager/internal/security"
	registryservice "github.com/cylism/cylism-manager/internal/service/registry"
	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const registryProxyNamespace = "kube-system"
const registryProxyName = "cylism-registry-proxy"

type RegistryProxyHandler struct {
	store         repository.RegistryProxyRepository
	service       *registryservice.ProxyService
	resources     k8sclient.RegistryProxyResourceReconciler
	diagnostics   k8sclient.RegistryProxyDiagnostics
	reconciler    *registryservice.ProxyReconciler
	encKey        []byte
	clientFactory func(context.Context, uint) (*k8sclient.Client, error)
}

type registryProxyRequest struct {
	ClusterID            uint     `json:"cluster_id"`
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

func proxyInput(req registryProxyRequest) registryservice.ProxyInput {
	return registryservice.ProxyInput{
		ClusterID: req.ClusterID,
		Name:      req.Name, Registry: req.Registry, UpstreamURL: req.UpstreamURL,
		NodeName: req.NodeName, EndpointHost: req.EndpointHost, NodePort: req.NodePort,
		CacheLimitGi: req.CacheLimitGi, CleanupIntervalHours: req.CleanupIntervalHours,
		HTTPProxy: req.HTTPProxy, HTTPSProxy: req.HTTPSProxy, NoProxy: req.NoProxy,
		DNSServers: req.DNSServers, ClearOutboundProxy: req.ClearOutboundProxy,
	}
}

type registryProxyImportRequest struct {
	ClusterID    uint   `json:"cluster_id"`
	Name         string `json:"name"`
	ResourceName string `json:"resource_name"`
	EndpointHost string `json:"endpoint_host"`
	Registry     string `json:"registry"`
}

// NewRegistryProxyHandlerWithDependencies uses the service and adapters
// composed by Bootstrap and never creates a fallback service.
func NewRegistryProxyHandlerWithDependencies(repo repository.RegistryProxyRepository, encKey []byte, resources k8sclient.RegistryProxyResourceReconciler, diagnostics k8sclient.RegistryProxyDiagnostics, service *registryservice.ProxyService) *RegistryProxyHandler {
	return &RegistryProxyHandler{store: repo, encKey: encKey, service: service, resources: resources, diagnostics: diagnostics}
}

// WithReconciler injects the Bootstrap-owned status and cache reconciler.
func (h *RegistryProxyHandler) WithReconciler(reconciler *registryservice.ProxyReconciler) *RegistryProxyHandler {
	h.reconciler = reconciler
	return h
}

func (h *RegistryProxyHandler) WithClusterClientFactory(factory func(context.Context, uint) (*k8sclient.Client, error)) *RegistryProxyHandler {
	h.clientFactory = factory
	return h
}

func (h *RegistryProxyHandler) adapters(ctx context.Context, clusterID uint) (k8sclient.RegistryProxyResourceReconciler, k8sclient.RegistryProxyDiagnostics, error) {
	if clusterID == 0 {
		if !h.k8sReady() {
			return nil, nil, errors.New("默认集群未连接")
		}
		return h.resources, h.diagnostics, nil
	}
	if h.clientFactory == nil {
		return nil, nil, errors.New("集群 Agent 访问未配置")
	}
	client, err := h.clientFactory(ctx, clusterID)
	if err != nil {
		return nil, nil, err
	}
	adapter := k8sclient.NewRegistryProxyReconciler(client)
	return adapter, adapter, nil
}

// ImportExisting registers an existing workload in observation mode. It never
// applies or deletes Kubernetes resources.
func (h *RegistryProxyHandler) ImportExisting(c *gin.Context) {
	var req registryProxyImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "纳管信息无效")
		return
	}
	req.Name, req.ResourceName, req.EndpointHost = strings.TrimSpace(req.Name), strings.TrimSpace(req.ResourceName), strings.TrimSpace(req.EndpointHost)
	if req.ClusterID == 0 || req.Name == "" || len(req.Name) > 128 || len(req.ResourceName) > 128 || !validProxyResourceName(req.ResourceName) || !registryservice.PrivateOrSharedAddressIP(req.EndpointHost) {
		apiShared.ValidationError(c, "请选择集群，并填写有效的代理名称、资源名和私网或共享入口 IP")
		return
	}
	if h.clientFactory == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	client, err := h.clientFactory(c.Request.Context(), req.ClusterID)
	if err != nil {
		apiShared.ValidationError(c, "读取目标集群失败: "+err.Error())
		return
	}
	adapter := k8sclient.NewRegistryProxyReconciler(client)
	snapshot, err := adapter.InspectExisting(c.Request.Context(), req.ResourceName)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	upstream, err := url.Parse(snapshot.UpstreamURL)
	if err != nil {
		apiShared.ValidationError(c, "现有代理上游地址无效")
		return
	}
	registry := registryservice.NormalizeRegistryHost(req.Registry)
	if registry == "" {
		registry = registryservice.NormalizeRegistryHost(upstream.Hostname())
	}
	if registry != registryservice.NormalizeRegistryHost(upstream.Hostname()) {
		apiShared.ValidationError(c, "Registry 与现有代理的上游地址不匹配")
		return
	}
	input := registryservice.ProxyInput{Name: req.Name, Registry: registry, UpstreamURL: snapshot.UpstreamURL, NodeName: snapshot.NodeName, EndpointHost: req.EndpointHost, NodePort: snapshot.NodePort, CacheLimitGi: snapshot.CacheLimitGi, CleanupIntervalHours: 24}
	if err := registryservice.ValidateProxyInput(input); err != nil {
		apiShared.ValidationError(c, "现有代理配置不符合平台要求: "+err.Error())
		return
	}
	if err := adapter.EnsureNode(c.Request.Context(), snapshot.NodeName); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	proxies, err := h.service.List()
	if err != nil {
		apiShared.DBError(c, "读取代理列表失败")
		return
	}
	for _, existing := range proxies {
		if existing.ClusterID == req.ClusterID && existing.ResourceName == req.ResourceName {
			apiShared.ValidationError(c, "该集群的代理资源已纳管")
			return
		}
		if existing.ClusterID == req.ClusterID && existing.NodePort == snapshot.NodePort {
			apiShared.ValidationError(c, fmt.Sprintf("NodePort %d 已被镜像代理 %q 使用", snapshot.NodePort, existing.Name))
			return
		}
	}
	proxy := &model.RegistryProxy{ClusterID: req.ClusterID, ManagementMode: "observe", ImportedOutboundProxy: snapshot.OutboundProxyConfigured, Name: req.Name, Registry: registry, UpstreamURL: snapshot.UpstreamURL, ResourceName: req.ResourceName, NodeName: snapshot.NodeName, EndpointHost: req.EndpointHost, NodePort: snapshot.NodePort, CacheLimitGi: snapshot.CacheLimitGi, CleanupIntervalHours: 24, NoProxy: snapshot.NoProxy, Status: "deploying", CreatedBy: apiShared.UserID(c)}
	if snapshot.Available {
		proxy.Status = "ready"
	}
	if len(snapshot.DNSServers) > 0 {
		encoded, _ := json.Marshal(snapshot.DNSServers)
		proxy.DNSResolvers = string(encoded)
	}
	if err := h.service.Save(proxy); err != nil {
		apiShared.DBError(c, "保存纳管记录失败")
		return
	}
	h.redactProxy(proxy)
	apiShared.Success(c, apiShared.RegistryProxyDTO(proxy))
}

func validProxyResourceName(name string) bool {
	if name == "" || name[0] == '-' || name[len(name)-1] == '-' {
		return false
	}
	for _, ch := range name {
		if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-') {
			return false
		}
	}
	return true
}

func (h *RegistryProxyHandler) TakeOver(c *gin.Context) {
	var req struct {
		HTTPProxy  string `json:"http_proxy"`
		HTTPSProxy string `json:"https_proxy"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "接管信息无效")
		return
	}
	proxy, err := h.proxyForRequest(c)
	if err != nil {
		apiShared.NotFound(c, "镜像代理不存在")
		return
	}
	if proxy.ManagementMode != "observe" {
		apiShared.ValidationError(c, "代理已经由平台管理")
		return
	}
	resources, _, err := h.adapters(c.Request.Context(), proxy.ClusterID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	adapter, ok := resources.(*k8sclient.RegistryProxyReconciler)
	if !ok {
		apiShared.K8sUnavailable(c)
		return
	}
	snapshot, err := adapter.InspectExisting(c.Request.Context(), proxy.ResourceName)
	if err != nil || snapshot.NodePort != proxy.NodePort || snapshot.UpstreamURL != proxy.UpstreamURL || snapshot.NodeName != proxy.NodeName {
		apiShared.ValidationError(c, "现有资源已变化，请核对后重新纳管")
		return
	}
	if snapshot.OutboundProxyConfigured && (strings.TrimSpace(req.HTTPProxy) == "" || strings.TrimSpace(req.HTTPSProxy) == "") {
		apiShared.ValidationError(c, "现有代理使用出网代理；接管前请重新录入 HTTP_PROXY 和 HTTPS_PROXY")
		return
	}
	if err := h.service.TakeOver(proxy, req.HTTPProxy, req.HTTPSProxy); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	h.redactProxy(proxy)
	apiShared.Success(c, apiShared.RegistryProxyDTO(proxy))
}

func (h *RegistryProxyHandler) RemoveObservation(c *gin.Context) {
	proxy, err := h.proxyForRequest(c)
	if err != nil {
		apiShared.NotFound(c, "镜像代理不存在")
		return
	}
	if proxy.ManagementMode != "observe" {
		apiShared.ValidationError(c, "只能取消观察中代理的纳管")
		return
	}
	if relays, ok := h.store.(repository.RegistryProxyRelayRepository); ok {
		items, err := relays.ListRegistryProxyRelays(proxy.ID)
		if err != nil {
			apiShared.DBError(c, "检查中转入口失败")
			return
		}
		if len(items) > 0 {
			apiShared.ValidationError(c, "请先移除关联的中转入口")
			return
		}
		routes, err := relays.ListRegistryProxyNodeRoutes(proxy.ID)
		if err != nil {
			apiShared.DBError(c, "检查节点入口失败")
			return
		}
		if len(routes) > 0 {
			apiShared.ValidationError(c, "请先解除节点入口配置")
			return
		}
	}
	if err := h.store.DeleteRegistryProxy(proxy.ID); err != nil {
		apiShared.DBError(c, "取消纳管失败")
		return
	}
	apiShared.Success(c, nil)
}

// WithResourceReconciler replaces only mutating proxy convergence actions.
func (h *RegistryProxyHandler) WithResourceReconciler(resources k8sclient.RegistryProxyResourceReconciler) *RegistryProxyHandler {
	h.resources = resources
	return h
}

// WithDiagnostics replaces only proxy readiness and upstream diagnostics.
func (h *RegistryProxyHandler) WithDiagnostics(diagnostics k8sclient.RegistryProxyDiagnostics) *RegistryProxyHandler {
	h.diagnostics = diagnostics
	return h
}

func (h *RegistryProxyHandler) Get(c *gin.Context) {
	proxy, err := h.service.GetLegacy()
	if apierrors.IsNotFound(err) || err != nil && strings.Contains(err.Error(), "record not found") {
		apiShared.Success(c, nil)
		return
	}
	if err != nil {
		apiShared.DBError(c, "读取镜像代理失败")
		return
	}
	h.refreshStatus(c.Request.Context(), proxy)
	h.redactProxy(proxy)
	apiShared.Success(c, apiShared.RegistryProxyDTO(proxy))
}

func (h *RegistryProxyHandler) List(c *gin.Context) {
	proxies, err := h.service.List()
	if err != nil {
		apiShared.DBError(c, "读取镜像代理失败")
		return
	}
	for index := range proxies {
		h.redactProxy(&proxies[index])
	}
	views := make([]apiShared.RegistryProxyView, 0, len(proxies))
	for i := range proxies {
		if view := apiShared.RegistryProxyDTO(&proxies[i]); view != nil {
			views = append(views, *view)
		}
	}
	apiShared.Success(c, views)
}

func (h *RegistryProxyHandler) Deploy(c *gin.Context) {
	var req registryProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "代理配置无效")
		return
	}
	if isLegacyRegistryProxyRoute(c) {
		applyDockerHubProxyDefaults(&req)
	}
	proxy, err := h.proxyForRequest(c)
	if err != nil {
		if c.Param("id") != "" {
			apiShared.NotFound(c, "镜像代理不存在")
			return
		}
		proxy = &model.RegistryProxy{CreatedBy: apiShared.UserID(c)}
	}
	if proxy.ID != 0 {
		if proxy.ManagementMode == "observe" {
			apiShared.ValidationError(c, "观察中的代理不能修改配置，请先接管")
			return
		}
		req.ClusterID = proxy.ClusterID
	}
	input := proxyInput(req)
	if err := registryservice.ValidateProxyInput(input); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	resources, _, err := h.adapters(c.Request.Context(), req.ClusterID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := resources.EnsureNode(c.Request.Context(), req.NodeName); err != nil {
		apiShared.ValidationError(c, "部署节点不存在或未加入集群")
		return
	}
	proxy, err = h.service.PrepareDeployment(input, proxy, apiShared.UserID(c))
	if err != nil {
		if strings.Contains(err.Error(), "读取现有镜像代理") {
			apiShared.DBError(c, "保存代理配置失败")
		} else if strings.Contains(err.Error(), "保存") {
			apiShared.DBError(c, "保存代理配置失败")
		} else {
			apiShared.ValidationError(c, err.Error())
		}
		return
	}
	if proxy == nil {
		apiShared.DBError(c, "保存代理配置失败")
		return
	}
	if err := h.apply(c.Request.Context(), resources, proxy); err != nil {
		h.service.MarkFailed(proxy, err)
		apiShared.ValidationError(c, "部署镜像代理失败: "+err.Error())
		return
	}
	h.redactProxy(proxy)
	apiShared.SuccessWithMessage(c, apiShared.RegistryProxyDTO(proxy), "镜像代理已提交部署，稍后可在节点镜像源中使用该地址")
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
	if err != nil {
		apiShared.NotFound(c, "镜像代理不存在")
		return
	}
	if proxy.ManagementMode == "observe" {
		apiShared.ValidationError(c, "观察中的代理不能清理缓存，请先接管")
		return
	}
	resources, _, err := h.adapters(c.Request.Context(), proxy.ClusterID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.clearCache(c.Request.Context(), resources, proxy, "手动清理"); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, apiShared.RegistryProxyDTO(proxy), "代理 Pod 已重建，临时缓存正在清理")
}

// Diagnose probes only the configured upstream from a Ready Pod belonging to
// this managed proxy. It accepts no caller-controlled network target or argv.
func (h *RegistryProxyHandler) Diagnose(c *gin.Context) {
	proxy, err := h.proxyForRequest(c)
	if err != nil {
		apiShared.NotFound(c, "镜像代理不存在")
		return
	}
	_, diagnostics, err := h.adapters(c.Request.Context(), proxy.ClusterID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	registryservice.NormalizeRegistryProxy(proxy)
	diagnostic, err := diagnostics.DiagnoseUpstream(c.Request.Context(), proxy)
	if err != nil {
		proxy.LastDiagnosticStatus = "diagnostic_failed"
		proxy.LastDiagnosticError = "代理出网诊断失败"
		now := time.Now()
		proxy.LastDiagnosticAt = &now
		_ = h.store.SaveRegistryProxy(proxy)
		apiShared.K8sAPIError(c, "代理出网诊断失败")
		return
	}
	proxy.LastDiagnosticStatus = diagnostic.Status
	proxy.LastDiagnosticError = security.Truncate(strings.TrimSpace(diagnostic.Summary), 512)
	now := time.Now()
	proxy.LastDiagnosticAt = &now
	if err := h.store.SaveRegistryProxy(proxy); err != nil {
		apiShared.DBError(c, "保存代理诊断结果失败")
		return
	}
	apiShared.Success(c, diagnostic)
}

// MigrateResourceName recreates the legacy Docker Hub resources using the per-instance naming scheme.
func (h *RegistryProxyHandler) MigrateResourceName(c *gin.Context) {
	proxy, err := h.proxyForRequest(c)
	if err != nil {
		apiShared.NotFound(c, "镜像代理不存在")
		return
	}
	if proxy.ManagementMode == "observe" {
		apiShared.ValidationError(c, "观察中的代理不能迁移资源，请先接管")
		return
	}
	resources, _, err := h.adapters(c.Request.Context(), proxy.ClusterID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	registryservice.NormalizeRegistryProxy(proxy)
	legacyResourceName := registryservice.ProxyResourceName(proxy)
	newResourceName := registryProxyName + "-" + strconv.Itoa(int(proxy.ID))
	if legacyResourceName == newResourceName {
		apiShared.SuccessWithMessage(c, apiShared.RegistryProxyDTO(proxy), "镜像代理已使用新资源命名")
		return
	}
	if legacyResourceName != registryProxyName || proxy.Registry != "docker.io" {
		apiShared.ValidationError(c, "仅支持迁移旧 Docker Hub 代理资源")
		return
	}

	ctx := c.Request.Context()
	if err := resources.DeleteLegacyResources(ctx, legacyResourceName); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	proxy.ResourceName, proxy.Status, proxy.LastError = newResourceName, "deploying", ""
	if err := h.store.SaveRegistryProxy(proxy); err != nil {
		apiShared.DBError(c, "保存迁移后的代理配置失败")
		return
	}
	if err := h.apply(ctx, resources, proxy); err != nil {
		proxy.Status, proxy.LastError = "failed", "使用新资源名重建失败: "+err.Error()
		_ = h.store.SaveRegistryProxy(proxy)
		apiShared.ValidationError(c, proxy.LastError)
		return
	}
	apiShared.SuccessWithMessage(c, apiShared.RegistryProxyDTO(proxy), "旧 Docker Hub 代理已按新资源名重建，等待新 Pod 就绪")
}

func (h *RegistryProxyHandler) refreshStatus(ctx context.Context, proxy *model.RegistryProxy) {
	if h.reconciler != nil {
		_ = h.reconciler.Refresh(ctx, proxy)
		return
	}
	resources, diagnostics, err := h.adapters(ctx, proxy.ClusterID)
	if err != nil {
		return
	}
	available, missing, err := diagnostics.DeploymentAvailable(ctx, proxy)
	if missing {
		proxy.Status, proxy.LastError = "missing", "代理 Deployment 不存在"
	} else if err != nil {
		proxy.Status, proxy.LastError = "failed", "读取代理状态失败: "+err.Error()
	} else if available {
		proxy.Status, proxy.LastError = "ready", ""
		if proxy.ManagementMode == "observe" {
			// Existing workloads are read only until explicitly taken over.
		} else if proxy.LastCleanupAt == nil {
			now := time.Now()
			proxy.LastCleanupAt = &now
		} else if time.Since(*proxy.LastCleanupAt) >= time.Duration(proxy.CleanupIntervalHours)*time.Hour {
			_ = h.clearCache(ctx, resources, proxy, "定期清理")
		}
	} else {
		proxy.Status = "deploying"
	}
	now := time.Now()
	proxy.LastCheckedAt = &now
	_ = h.service.Save(proxy)
}

func (h *RegistryProxyHandler) redactProxy(proxy *model.RegistryProxy) {
	proxy.OutboundProxyConfigured = proxy.EncryptedHTTPProxy != "" || proxy.EncryptedHTTPSProxy != ""
	proxy.DNSServers = registryservice.ProxyDNSServers(proxy)
	proxy.EncryptedHTTPProxy = ""
	proxy.EncryptedHTTPSProxy = ""
}

func (h *RegistryProxyHandler) clearCache(ctx context.Context, resources k8sclient.RegistryProxyResourceReconciler, proxy *model.RegistryProxy, reason string) error {
	if err := resources.ClearCache(ctx, proxy); err != nil {
		return fmt.Errorf("%s失败: %w", reason, err)
	}
	now := time.Now()
	proxy.LastCleanupAt, proxy.Status, proxy.LastError = &now, "deploying", reason+"后等待新 Pod 就绪"
	return h.service.Save(proxy)
}

func (h *RegistryProxyHandler) apply(ctx context.Context, resources k8sclient.RegistryProxyResourceReconciler, proxy *model.RegistryProxy) error {
	environment, err := h.service.ProxyEnvironment(proxy)
	if err != nil {
		return err
	}
	return resources.Apply(ctx, proxy, environment)
}

func (h *RegistryProxyHandler) k8sReady() bool {
	return h.resources != nil && h.diagnostics != nil && h.diagnostics.Available()
}

func (h *RegistryProxyHandler) proxyForRequest(c *gin.Context) (*model.RegistryProxy, error) {
	if rawID := strings.TrimSpace(c.Param("id")); rawID != "" {
		id, err := apiShared.ParsePositiveID(rawID)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy id")
		}
		return h.store.GetRegistryProxyByID(uint(id))
	}
	if !isLegacyRegistryProxyRoute(c) {
		return nil, fmt.Errorf("registry proxy id is required")
	}
	return h.store.GetRegistryProxy()
}
