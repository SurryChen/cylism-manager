package delivery

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	registryservice "github.com/cylism/cylism-manager/internal/service/registry"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const registryProxyNamespace = "kube-system"
const registryProxyName = "cylism-registry-proxy"

type RegistryProxyHandler struct {
	store      *store.Store
	service    *registryservice.ProxyService
	reconciler *k8sclient.RegistryProxyReconciler
	encKey     []byte
	diagnoser  registryProxyDiagnoser
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

func proxyInput(req registryProxyRequest) registryservice.ProxyInput {
	return registryservice.ProxyInput{
		Name: req.Name, Registry: req.Registry, UpstreamURL: req.UpstreamURL,
		NodeName: req.NodeName, EndpointHost: req.EndpointHost, NodePort: req.NodePort,
		CacheLimitGi: req.CacheLimitGi, CleanupIntervalHours: req.CleanupIntervalHours,
		HTTPProxy: req.HTTPProxy, HTTPSProxy: req.HTTPSProxy, NoProxy: req.NoProxy,
		DNSServers: req.DNSServers, ClearOutboundProxy: req.ClearOutboundProxy,
	}
}

type registryProxyDiagnoser func(context.Context, *model.RegistryProxy) (k8sclient.RegistryProxyDiagnostic, error)

func NewRegistryProxyHandler(s *store.Store, encKey []byte, client *k8sclient.Client) *RegistryProxyHandler {
	h := &RegistryProxyHandler{store: s}
	h.encKey = encKey
	h.service = registryservice.NewProxyService(repository.NewRegistryProxyRepository(s), h.encKey)
	h.reconciler = k8sclient.NewRegistryProxyReconciler(client)
	h.diagnoser = h.reconciler.DiagnoseUpstream
	return h
}

func (h *RegistryProxyHandler) WithDiagnoser(diagnoser registryProxyDiagnoser) *RegistryProxyHandler {
	h.diagnoser = diagnoser
	return h
}

func (h *RegistryProxyHandler) Get(c *gin.Context) {
	proxy, err := h.service.GetLegacy()
	if apierrors.IsNotFound(err) || err != nil && strings.Contains(err.Error(), "record not found") {
		model.Success(c, nil)
		return
	}
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取镜像代理失败")
		return
	}
	h.refreshStatus(c.Request.Context(), proxy)
	h.redactProxy(proxy)
	model.Success(c, proxy)
}

func (h *RegistryProxyHandler) List(c *gin.Context) {
	proxies, err := h.service.List()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取镜像代理失败")
		return
	}
	for index := range proxies {
		h.refreshStatus(c.Request.Context(), &proxies[index])
		h.redactProxy(&proxies[index])
	}
	model.Success(c, proxies)
}

func (h *RegistryProxyHandler) Deploy(c *gin.Context) {
	if !h.reconciler.Available() {
		apiShared.K8sUnavailable(c)
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
	input := proxyInput(req)
	if err := registryservice.ValidateProxyInput(input); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.reconciler.EnsureNode(c.Request.Context(), req.NodeName); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "部署节点不存在或未加入集群")
		return
	}
	proxy, err := h.proxyForRequest(c)
	if err != nil {
		if c.Param("id") != "" {
			model.Error(c, http.StatusNotFound, model.CodeNotFound, "镜像代理不存在")
			return
		}
		proxy = &model.RegistryProxy{CreatedBy: apiShared.UserID(c)}
	}
	proxy, err = h.service.PrepareDeployment(input, proxy, apiShared.UserID(c))
	if err != nil {
		if strings.Contains(err.Error(), "读取现有镜像代理") {
			model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存代理配置失败")
		} else if strings.Contains(err.Error(), "保存") {
			model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存代理配置失败")
		} else {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		}
		return
	}
	if proxy == nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存代理配置失败")
		return
	}
	if err := h.apply(c.Request.Context(), proxy); err != nil {
		h.service.MarkFailed(proxy, err)
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
	if err != nil || !h.reconciler.Available() {
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
	if err != nil || !h.reconciler.Available() {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "镜像代理不存在或集群未连接")
		return
	}
	registryservice.NormalizeRegistryProxy(proxy)
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
	if !h.reconciler.Available() {
		apiShared.K8sUnavailable(c)
		return
	}
	proxy, err := h.proxyForRequest(c)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "镜像代理不存在")
		return
	}
	registryservice.NormalizeRegistryProxy(proxy)
	legacyResourceName := registryservice.ProxyResourceName(proxy)
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
	if err := h.reconciler.DeleteLegacyResources(ctx, legacyResourceName); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
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
		if !h.reconciler.Available() {
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
	if !h.reconciler.Available() {
		return
	}
	available, missing, err := h.reconciler.DeploymentAvailable(ctx, proxy)
	if missing {
		proxy.Status, proxy.LastError = "missing", "代理 Deployment 不存在"
	} else if err != nil {
		proxy.Status, proxy.LastError = "failed", "读取代理状态失败: "+err.Error()
	} else if available {
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
	_ = h.service.Save(proxy)
}

func (h *RegistryProxyHandler) redactProxy(proxy *model.RegistryProxy) {
	proxy.OutboundProxyConfigured = proxy.EncryptedHTTPProxy != "" || proxy.EncryptedHTTPSProxy != ""
	proxy.DNSServers = registryservice.ProxyDNSServers(proxy)
	proxy.EncryptedHTTPProxy = ""
	proxy.EncryptedHTTPSProxy = ""
}

func (h *RegistryProxyHandler) clearCache(ctx context.Context, proxy *model.RegistryProxy, reason string) error {
	if err := h.reconciler.ClearCache(ctx, proxy); err != nil {
		return fmt.Errorf("%s失败: %w", reason, err)
	}
	now := time.Now()
	proxy.LastCleanupAt, proxy.Status, proxy.LastError = &now, "deploying", reason+"后等待新 Pod 就绪"
	return h.service.Save(proxy)
}

func (h *RegistryProxyHandler) apply(ctx context.Context, proxy *model.RegistryProxy) error {
	environment, err := h.service.ProxyEnvironment(proxy)
	if err != nil {
		return err
	}
	return h.reconciler.Apply(ctx, proxy, environment)
}

func truncateProxyDiagnosticText(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 512 {
		return value[:512]
	}
	return value
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
