package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	crypto "github.com/cylism/cylism-manager/internal/security"
	networkservice "github.com/cylism/cylism-manager/internal/service/network"

	"github.com/gin-gonic/gin"
)

// CertHandler Certificate 管理的 HTTP handler
type CertHandler struct {
	store   repository.CertificateRepository
	encKey  []byte
	network *networkservice.Service
	k8s     CertificateKubernetesAdapter
}

type CertificateKubernetesAdapter interface {
	KubernetesAvailable() bool
	InstallCertManagerContext(context.Context, string, string, string) (*k8s.CertManagerStatus, error)
}

// NewCertificateKubernetesAdapter exposes only the cert-manager capability
// needed by the HTTP handler; Bootstrap owns the concrete client.
func NewCertificateKubernetesAdapter(client *k8s.Client) CertificateKubernetesAdapter {
	if client == nil {
		return nil
	}
	return client
}

type DNSCredentialRequest struct {
	Name      string             `json:"name"`
	Namespace string             `json:"namespace"`
	Provider  string             `json:"provider"`
	Values    map[string]*string `json:"values"`
	Enabled   *bool              `json:"enabled"`
}

// NewCertHandlerWithComposedDependencies receives the configured network
// service and cert-manager port from Bootstrap without creating fallbacks.
func NewCertHandlerWithComposedDependencies(st repository.CertificateRepository, encKey []byte, network *networkservice.Service, client CertificateKubernetesAdapter) *CertHandler {
	return &CertHandler{store: st, encKey: append([]byte(nil), encKey...), network: network, k8s: client}
}

// Status reports cert-manager prerequisites before certificate resources are queried.
func (h *CertHandler) Status(c *gin.Context) {
	if h.k8s == nil || !h.k8s.KubernetesAvailable() {
		apiShared.Success(c, &k8s.CertManagerStatus{State: k8s.CertManagerStateUnavailable, Message: "Kubernetes 客户端未初始化"})
		return
	}
	status, err := h.network.CertificateManagerStatusContext(c.Request.Context())
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sUnavailable, err.Error())
		return
	}
	apiShared.Success(c, status)
}

// Install starts the fixed cert-manager HelmChart installation supported by K3s.
func (h *CertHandler) Install(c *gin.Context) {
	if h.k8s == nil || !h.k8s.KubernetesAvailable() {
		certK8sUnavailable(c)
		return
	}
	if h.store == nil {
		apiShared.InternalError(c, "Chart 仓库存储未初始化")
		return
	}
	repository, err := h.store.GetVerifiedCertManagerChartRepository()
	if err != nil {
		apiShared.ValidationError(c, "请先配置并验证可用的 cert-manager Chart 仓库")
		return
	}
	status, err := h.k8s.InstallCertManagerContext(c.Request.Context(), repository.Endpoint, repository.ChartName, repository.ChartVersion)
	if err != nil {
		apiShared.ErrorWithData(c, http.StatusBadRequest, apiShared.CodeK8sAPIError, err.Error(), status)
		return
	}
	apiShared.SuccessWithMessage(c, status, status.Message)
}

// ListCerts 列出所有 Certificate
func (h *CertHandler) ListCerts(c *gin.Context) {
	if h.k8s == nil || !h.k8s.KubernetesAvailable() {
		certK8sUnavailable(c)
		return
	}
	certs, err := h.network.ListCertificatesContext(c.Request.Context())
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	apiShared.Success(c, certs)
}

// CreateCert 创建 Certificate
func (h *CertHandler) CreateCert(c *gin.Context) {
	if !h.requireReady(c) {
		return
	}
	var request k8s.CreateCertificateRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Name == "" || request.Namespace == "" || request.IssuerRef == "" || len(request.Domains) == 0 {
		apiShared.BadRequest(c, "证书名称、命名空间、域名和 Issuer 必填")
		return
	}
	if request.IssuerKind != "" && request.IssuerKind != "Issuer" && request.IssuerKind != "ClusterIssuer" {
		apiShared.BadRequest(c, "Issuer 类型必须为 Issuer 或 ClusterIssuer")
		return
	}
	for _, domain := range request.Domains {
		if strings.TrimSpace(domain) == "" {
			apiShared.BadRequest(c, "域名不能为空")
			return
		}
	}
	certificate, err := h.network.CreateCertificateContext(c.Request.Context(), request)
	if err != nil {
		apiShared.Error(c, http.StatusInternalServerError, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	apiShared.Success(c, certificate)
}

func (h *CertHandler) ListIssuers(c *gin.Context) {
	if !h.requireReady(c) {
		return
	}
	issuers, err := h.network.ListIssuersContext(c.Request.Context())
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	apiShared.Success(c, issuers)
}

func (h *CertHandler) CreateIssuer(c *gin.Context) { h.saveIssuer(c, false) }
func (h *CertHandler) UpdateIssuer(c *gin.Context) { h.saveIssuer(c, true) }

type issuerRequest struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Kind         string `json:"kind"`
	Mode         string `json:"mode"`
	Email        string `json:"email"`
	Server       string `json:"server"`
	IngressClass string `json:"ingress_class"`
	CredentialID uint   `json:"credential_id"`
	DNSProvider  string `json:"dns_provider"`
}

func (h *CertHandler) saveIssuer(c *gin.Context, update bool) {
	if !h.requireReady(c) {
		return
	}
	var req issuerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "签发者定义无效")
		return
	}
	if update {
		req.Name, req.Namespace, req.Kind = c.Param("name"), c.Param("namespace"), c.Param("kind")
	}
	request := k8s.IssuerRequest{Name: req.Name, Namespace: req.Namespace, Kind: req.Kind, Mode: req.Mode, Email: req.Email, Server: req.Server, IngressClass: req.IngressClass, DNSProvider: req.DNSProvider}
	if req.Mode == "acme_dns01" {
		if h.store == nil {
			apiShared.InternalError(c, "DNS 凭据存储未初始化")
			return
		}
		credential, err := h.network.GetDNSCredential(req.CredentialID)
		if err != nil || !credential.Enabled {
			apiShared.ValidationError(c, "请选择已启用的 DNS 凭据")
			return
		}
		if req.DNSProvider == "" {
			req.DNSProvider = credential.Provider
		}
		if credential.Provider != req.DNSProvider {
			apiShared.ValidationError(c, "DNS Provider 与凭据不匹配")
			return
		}
		request.DNSProvider = credential.Provider
		webhook, webhookErr := h.network.DNSProviderStatusContext(c.Request.Context(), credential.Provider)
		if webhookErr != nil {
			apiShared.Error(c, http.StatusBadRequest, apiShared.CodeK8sAPIError, webhookErr.Error())
			return
		}
		if !webhook.Ready {
			apiShared.ErrorWithData(c, http.StatusBadRequest, apiShared.CodeK8sAPIError, "DNS Provider 未就绪: "+webhook.Message, webhook)
			return
		}
		if request.Kind == "Issuer" && credential.Namespace != request.Namespace {
			apiShared.ValidationError(c, "命名空间级 Issuer 必须使用同命名空间的 DNS 凭据")
			return
		}
		request.CredentialSecretName = credential.SecretName
	}
	var issuer *k8s.IssuerInfo
	var err error
	if update {
		issuer, err = h.network.UpdateIssuerContext(c.Request.Context(), request)
	} else {
		issuer, err = h.network.CreateIssuerContext(c.Request.Context(), request)
	}
	if err != nil {
		apiShared.Error(c, http.StatusBadRequest, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	apiShared.Success(c, issuer)
}

func (h *CertHandler) DeleteIssuer(c *gin.Context) {
	if !h.requireReady(c) {
		return
	}
	if err := h.network.DeleteIssuerContext(c.Request.Context(), c.Param("kind"), c.Param("namespace"), c.Param("name")); err != nil {
		apiShared.Error(c, http.StatusBadRequest, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, nil, "签发者已删除")
}

func (h *CertHandler) ListOperations(c *gin.Context) {
	if !h.requireReady(c) {
		return
	}
	operations, err := h.network.ListCertificateOperationsContext(c.Request.Context(), c.Param("namespace"), c.Param("name"))
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	apiShared.Success(c, operations)
}

type dnsCredentialRequest = DNSCredentialRequest

// DNSCredentialFromRequest exposes the validation/encryption boundary to
// compatibility callers without exposing the HTTP package.
func (h *CertHandler) DNSCredentialFromRequest(req DNSCredentialRequest, current *model.DNSCredential) (*model.DNSCredential, map[string]string, error) {
	return h.dnsCredentialFromRequest(req, current)
}

// certK8sUnavailable preserves the API's historical 200 response for an
// unavailable cluster while keeping this transport package self-contained.
func certK8sUnavailable(c *gin.Context) {
	apiShared.K8sUnavailable(c)
}

func requestUserID(c *gin.Context) uint {
	return apiShared.UserID(c)
}

type dnsProviderView struct {
	k8s.DNSProviderInfo
	Status *k8s.DNSProviderStatus `json:"status"`
}

func (h *CertHandler) ListDNSProviders(c *gin.Context) {
	providers := k8s.ListDNSProviders()
	result := make([]dnsProviderView, 0, len(providers))
	for _, provider := range providers {
		view := dnsProviderView{DNSProviderInfo: provider}
		if h.k8s != nil && h.k8s.KubernetesAvailable() {
			if status, err := h.network.DNSProviderStatusContext(c.Request.Context(), provider.ID); err == nil {
				view.Status = status
			}
		}
		result = append(result, view)
	}
	apiShared.Success(c, result)
}

func (h *CertHandler) DNSProviderStatus(c *gin.Context) {
	if h.k8s == nil || !h.k8s.KubernetesAvailable() {
		certK8sUnavailable(c)
		return
	}
	status, err := h.network.DNSProviderStatusContext(c.Request.Context(), c.Param("provider"))
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	apiShared.Success(c, status)
}

func (h *CertHandler) InstallDNSProvider(c *gin.Context) {
	if h.k8s == nil || !h.k8s.KubernetesAvailable() {
		certK8sUnavailable(c)
		return
	}
	status, err := h.network.InstallDNSProviderContext(c.Request.Context(), c.Param("provider"))
	if err != nil {
		apiShared.ErrorWithData(c, http.StatusBadRequest, apiShared.CodeK8sAPIError, err.Error(), status)
		return
	}
	apiShared.SuccessWithMessage(c, status, status.Message)
}

func (h *CertHandler) ListDNSCredentials(c *gin.Context) {
	if h.store == nil {
		apiShared.Success(c, []apiShared.DNSCredentialView{})
		return
	}
	credentials, err := h.network.ListDNSCredentials()
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	for index := range credentials {
		h.populateCredentialView(&credentials[index])
	}
	apiShared.Success(c, apiShared.DNSCredentialsDTO(credentials))
}

func (h *CertHandler) CreateDNSCredential(c *gin.Context) {
	if !h.requireReady(c) {
		return
	}
	var req dnsCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "DNS 凭据定义无效")
		return
	}
	credential, values, err := h.dnsCredentialFromRequest(req, nil)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	credential.CreatedBy = requestUserID(c)
	credential.SecretName = fmt.Sprintf("cylism-dns-pending-%d", time.Now().UnixNano())
	if err := h.network.CreateDNSCredential(credential); err != nil {
		apiShared.Conflict(c, "DNS 凭据名称无效或已存在")
		return
	}
	credential.SecretName = fmt.Sprintf("cylism-dns-%s-%d", credential.Provider, credential.ID)
	if err := h.network.UpdateDNSCredential(credential); err != nil {
		apiShared.DBError(c, "保存 DNS Secret 名称失败")
		return
	}
	if err := h.network.SyncDNSCredentialSecretContext(c.Request.Context(), credential, values); err != nil {
		_ = h.network.DeleteDNSCredential(credential.ID)
		apiShared.Error(c, http.StatusBadRequest, apiShared.CodeK8sAPIError, "创建 DNS Secret: "+err.Error())
		return
	}
	h.populateCredentialView(credential)
	apiShared.Success(c, apiShared.DNSCredentialDTO(credential))
}

func (h *CertHandler) UpdateDNSCredential(c *gin.Context) {
	if !h.requireReady(c) {
		return
	}
	id, err := apiShared.ParsePositiveID(strings.TrimSpace(c.Param("id")))
	if err != nil {
		apiShared.BadRequest(c, "DNS 凭据 ID 无效")
		return
	}
	current, err := h.network.GetDNSCredential(id)
	if err != nil {
		apiShared.NotFound(c, "DNS 凭据不存在")
		return
	}
	var req dnsCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "DNS 凭据定义无效")
		return
	}
	credential, values, err := h.dnsCredentialFromRequest(req, current)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.network.UpdateDNSCredential(credential); err != nil {
		apiShared.Conflict(c, "DNS 凭据名称无效或已存在")
		return
	}
	if err := h.network.SyncDNSCredentialSecretContext(c.Request.Context(), credential, values); err != nil {
		apiShared.Error(c, http.StatusBadRequest, apiShared.CodeK8sAPIError, "更新 DNS Secret: "+err.Error())
		return
	}
	if current.Namespace != credential.Namespace {
		if err := h.network.RemoveDNSCredentialSecretContext(c.Request.Context(), current.Namespace, current.SecretName); err != nil {
			apiShared.Error(c, http.StatusBadRequest, apiShared.CodeK8sAPIError, "清理旧 DNS Secret: "+err.Error())
			return
		}
	}
	h.populateCredentialView(credential)
	apiShared.Success(c, apiShared.DNSCredentialDTO(credential))
}

func (h *CertHandler) DeleteDNSCredential(c *gin.Context) {
	if !h.requireReady(c) {
		return
	}
	id, err := apiShared.ParsePositiveID(strings.TrimSpace(c.Param("id")))
	if err != nil {
		apiShared.BadRequest(c, "DNS 凭据 ID 无效")
		return
	}
	credential, err := h.network.GetDNSCredential(id)
	if err != nil {
		apiShared.NotFound(c, "DNS 凭据不存在")
		return
	}
	issuers, listErr := h.network.ListIssuersContext(c.Request.Context())
	if listErr != nil {
		apiShared.Error(c, http.StatusBadRequest, apiShared.CodeK8sAPIError, "检查签发者引用: "+listErr.Error())
		return
	}
	for _, issuer := range issuers {
		if issuer.CredentialSecretName == credential.SecretName {
			apiShared.Conflict(c, "DNS 凭据仍被签发者引用: "+issuer.Name)
			return
		}
	}
	if err := h.network.RemoveDNSCredentialSecretContext(c.Request.Context(), credential.Namespace, credential.SecretName); err != nil {
		apiShared.Error(c, http.StatusBadRequest, apiShared.CodeK8sAPIError, "删除 DNS Secret: "+err.Error())
		return
	}
	if err := h.network.DeleteDNSCredential(id); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	apiShared.Success(c, gin.H{"id": id})
}

func (h *CertHandler) dnsCredentialFromRequest(req dnsCredentialRequest, current *model.DNSCredential) (*model.DNSCredential, map[string]string, error) {
	if h.store == nil || len(h.encKey) == 0 {
		return nil, nil, fmt.Errorf("DNS 凭据加密未初始化")
	}
	name, namespace := strings.TrimSpace(req.Name), strings.TrimSpace(req.Namespace)
	providerID := strings.TrimSpace(req.Provider)
	if providerID == "" && current != nil {
		providerID = current.Provider
	}
	if providerID == "" {
		return nil, nil, fmt.Errorf("DNS Provider 必填")
	}
	provider, ok := k8s.GetDNSProvider(providerID)
	if !ok {
		return nil, nil, fmt.Errorf("不支持的 DNS Provider: %s", providerID)
	}
	if name == "" || namespace == "" {
		return nil, nil, fmt.Errorf("名称和命名空间必填")
	}
	values := map[string]string{}
	if current != nil {
		var err error
		values, err = h.decryptedCredentialValues(current)
		if err != nil {
			return nil, nil, err
		}
	}
	allowedFields := map[string]bool{}
	for _, field := range provider.Info().Fields {
		allowedFields[field.Key] = true
	}
	for key, value := range req.Values {
		if !allowedFields[key] {
			return nil, nil, fmt.Errorf("DNS Provider 不支持凭据字段: %s", key)
		}
		if value != nil && strings.TrimSpace(*value) != "" {
			values[key] = strings.TrimSpace(*value)
		}
	}
	if err := provider.ValidateValues(values); err != nil {
		return nil, nil, err
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return nil, nil, fmt.Errorf("编码 DNS 凭据失败")
	}
	encrypted, err := crypto.Encrypt(h.encKey, string(payload))
	if err != nil {
		return nil, nil, fmt.Errorf("DNS 凭据加密失败")
	}
	credential := &model.DNSCredential{Name: name, Provider: providerID, Namespace: namespace, Enabled: true, EncryptedValues: encrypted}
	if current != nil {
		credential.ID, credential.CreatedBy, credential.CreatedAt, credential.SecretName = current.ID, current.CreatedBy, current.CreatedAt, current.SecretName
		credential.Enabled = current.Enabled
		if req.Enabled != nil {
			credential.Enabled = *req.Enabled
		}
	}
	return credential, values, nil
}

func (h *CertHandler) decryptedCredentialValues(credential *model.DNSCredential) (map[string]string, error) {
	if credential.EncryptedValues != "" {
		plain, err := crypto.Decrypt(h.encKey, credential.EncryptedValues)
		if err != nil {
			return nil, fmt.Errorf("读取 DNS 凭据失败")
		}
		values := map[string]string{}
		if err := json.Unmarshal([]byte(plain), &values); err != nil {
			return nil, fmt.Errorf("读取 DNS 凭据失败")
		}
		return values, nil
	}
	return nil, fmt.Errorf("DNS 凭据数据不完整")
}

func (h *CertHandler) populateCredentialView(credential *model.DNSCredential) {
	credential.SecretConfigured = credential.EncryptedValues != ""
	provider, ok := k8s.GetDNSProvider(credential.Provider)
	if !ok {
		return
	}
	values, err := h.decryptedCredentialValues(credential)
	if err != nil {
		return
	}
	credential.ConfiguredFields = credential.ConfiguredFields[:0]
	for _, field := range provider.Info().Fields {
		if values[field.Key] != "" {
			credential.ConfiguredFields = append(credential.ConfiguredFields, field.Key)
		}
	}
}

// DeleteCert 删除 Certificate
func (h *CertHandler) DeleteCert(c *gin.Context) {
	if !h.requireReady(c) {
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	if err := h.network.DeleteCertificateContext(c.Request.Context(), ns, name); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, nil, "删除成功")
}

func (h *CertHandler) requireReady(c *gin.Context) bool {
	if h.k8s == nil || !h.k8s.KubernetesAvailable() {
		certK8sUnavailable(c)
		return false
	}
	status, err := h.network.CertificateManagerStatusContext(c.Request.Context())
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sUnavailable, err.Error())
		return false
	}
	if status.State == k8s.CertManagerStateReady {
		return true
	}
	apiShared.ErrorWithData(c, http.StatusOK, apiShared.CodeK8sAPIError, status.Message, status)
	return false
}
