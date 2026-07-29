package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"

	"github.com/gin-gonic/gin"
)

// CertHandler Certificate 管理的 HTTP handler
type CertHandler struct {
	store  *store.Store
	encKey []byte
}

// NewCertHandler 创建 CertHandler
func NewCertHandler(st ...*store.Store) *CertHandler {
	h := &CertHandler{}
	if len(st) > 0 {
		h.store = st[0]
	}
	return h
}

// NewCertHandlerWithEncryption constructs certificate management with encrypted DNS credentials.
func NewCertHandlerWithEncryption(st *store.Store, encKey []byte) *CertHandler {
	return &CertHandler{store: st, encKey: encKey}
}

// Status reports cert-manager prerequisites before certificate resources are queried.
func (h *CertHandler) Status(c *gin.Context) {
	if K8s == nil {
		model.Success(c, &k8s.CertManagerStatus{State: k8s.CertManagerStateUnavailable, Message: "Kubernetes 客户端未初始化"})
		return
	}
	model.Success(c, K8s.CertManagerStatus())
}

// Install starts the fixed cert-manager HelmChart installation supported by K3s.
func (h *CertHandler) Install(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	if h.store == nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "Chart 仓库存储未初始化")
		return
	}
	repository, err := h.store.GetVerifiedCertManagerChartRepository()
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "请先配置并验证可用的 cert-manager Chart 仓库")
		return
	}
	status, err := K8s.InstallCertManager(repository.Endpoint, repository.ChartName, repository.ChartVersion)
	if err != nil {
		model.ErrorWithData(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error(), status)
		return
	}
	model.SuccessWithMessage(c, status, status.Message)
}

// ListCerts 列出所有 Certificate
func (h *CertHandler) ListCerts(c *gin.Context) {
	if !h.requireReady(c) {
		return
	}
	certs, err := K8s.ListCertificates()
	if err != nil {
		model.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, certs)
}

// CreateCert 创建 Certificate
func (h *CertHandler) CreateCert(c *gin.Context) {
	if !h.requireReady(c) {
		return
	}
	var request k8s.CreateCertificateRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Name == "" || request.Namespace == "" || request.IssuerRef == "" || len(request.Domains) == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "证书名称、命名空间、域名和 Issuer 必填")
		return
	}
	if request.IssuerKind != "" && request.IssuerKind != "Issuer" && request.IssuerKind != "ClusterIssuer" {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "Issuer 类型必须为 Issuer 或 ClusterIssuer")
		return
	}
	for _, domain := range request.Domains {
		if strings.TrimSpace(domain) == "" {
			model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "域名不能为空")
			return
		}
	}
	certificate, err := K8s.CreateCertificate(request)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, certificate)
}

func (h *CertHandler) ListIssuers(c *gin.Context) {
	if !h.requireReady(c) {
		return
	}
	issuers, err := K8s.ListIssuers()
	if err != nil {
		model.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, issuers)
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
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "签发者定义无效")
		return
	}
	if update {
		req.Name, req.Namespace, req.Kind = c.Param("name"), c.Param("namespace"), c.Param("kind")
	}
	request := k8s.IssuerRequest{Name: req.Name, Namespace: req.Namespace, Kind: req.Kind, Mode: req.Mode, Email: req.Email, Server: req.Server, IngressClass: req.IngressClass, DNSProvider: req.DNSProvider}
	if req.Mode == "acme_dns01" {
		if h.store == nil {
			model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "DNS 凭据存储未初始化")
			return
		}
		credential, err := h.store.GetDNSCredential(req.CredentialID)
		if err != nil || !credential.Enabled {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "请选择已启用的 DNS 凭据")
			return
		}
		if req.DNSProvider == "" {
			req.DNSProvider = credential.Provider
		}
		if credential.Provider != req.DNSProvider {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "DNS Provider 与凭据不匹配")
			return
		}
		request.DNSProvider = credential.Provider
		webhook := K8s.DNSProviderStatus(credential.Provider)
		if !webhook.Ready {
			model.ErrorWithData(c, http.StatusBadRequest, model.CodeK8sAPIError, "DNS Provider 未就绪: "+webhook.Message, webhook)
			return
		}
		if request.Kind == "Issuer" && credential.Namespace != request.Namespace {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "命名空间级 Issuer 必须使用同命名空间的 DNS 凭据")
			return
		}
		request.CredentialSecretName = credential.SecretName
	}
	var issuer *k8s.IssuerInfo
	var err error
	if update {
		issuer, err = K8s.UpdateIssuer(request)
	} else {
		issuer, err = K8s.CreateIssuer(request)
	}
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, issuer)
}

func (h *CertHandler) DeleteIssuer(c *gin.Context) {
	if !h.requireReady(c) {
		return
	}
	if err := K8s.DeleteIssuer(c.Param("kind"), c.Param("namespace"), c.Param("name")); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	model.SuccessWithMessage(c, nil, "签发者已删除")
}

func (h *CertHandler) ListOperations(c *gin.Context) {
	if !h.requireReady(c) {
		return
	}
	operations, err := K8s.ListCertificateOperations(c.Param("namespace"), c.Param("name"))
	if err != nil {
		model.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, operations)
}

type dnsCredentialRequest struct {
	Name      string             `json:"name"`
	Namespace string             `json:"namespace"`
	Provider  string             `json:"provider"`
	Values    map[string]*string `json:"values"`
	Enabled   *bool              `json:"enabled"`
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
		if K8s != nil {
			view.Status = K8s.DNSProviderStatus(provider.ID)
		}
		result = append(result, view)
	}
	model.Success(c, result)
}

func (h *CertHandler) DNSProviderStatus(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	model.Success(c, K8s.DNSProviderStatus(c.Param("provider")))
}

func (h *CertHandler) InstallDNSProvider(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	status, err := K8s.InstallDNSProvider(c.Param("provider"))
	if err != nil {
		model.ErrorWithData(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error(), status)
		return
	}
	model.SuccessWithMessage(c, status, status.Message)
}

func (h *CertHandler) ListDNSCredentials(c *gin.Context) {
	if h.store == nil {
		model.Success(c, []model.DNSCredential{})
		return
	}
	credentials, err := h.store.ListDNSCredentials()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	for index := range credentials {
		h.populateCredentialView(&credentials[index])
	}
	model.Success(c, credentials)
}

func (h *CertHandler) CreateDNSCredential(c *gin.Context) {
	if !h.requireReady(c) {
		return
	}
	var req dnsCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "DNS 凭据定义无效")
		return
	}
	credential, values, err := h.dnsCredentialFromRequest(req, nil)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	credential.CreatedBy = getUserID(c)
	credential.SecretName = fmt.Sprintf("cylism-dns-pending-%d", time.Now().UnixNano())
	if err := h.store.CreateDNSCredential(credential); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "DNS 凭据名称无效或已存在")
		return
	}
	credential.SecretName = fmt.Sprintf("cylism-dns-%s-%d", credential.Provider, credential.ID)
	if err := h.store.UpdateDNSCredential(credential); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存 DNS Secret 名称失败")
		return
	}
	if err := K8s.UpsertDNSCredentialSecret(credential.Provider, credential.Namespace, credential.SecretName, values); err != nil {
		_ = h.store.DeleteDNSCredential(credential.ID)
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, "创建 DNS Secret: "+err.Error())
		return
	}
	h.populateCredentialView(credential)
	model.Success(c, credential)
}

func (h *CertHandler) UpdateDNSCredential(c *gin.Context) {
	if !h.requireReady(c) {
		return
	}
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "DNS 凭据 ID 无效")
		return
	}
	current, err := h.store.GetDNSCredential(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "DNS 凭据不存在")
		return
	}
	var req dnsCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "DNS 凭据定义无效")
		return
	}
	credential, values, err := h.dnsCredentialFromRequest(req, current)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.store.UpdateDNSCredential(credential); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "DNS 凭据名称无效或已存在")
		return
	}
	if err := K8s.UpsertDNSCredentialSecret(credential.Provider, credential.Namespace, credential.SecretName, values); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, "更新 DNS Secret: "+err.Error())
		return
	}
	if current.Namespace != credential.Namespace {
		if err := K8s.DeleteDNSCredentialSecret(current.Namespace, current.SecretName); err != nil {
			model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, "清理旧 DNS Secret: "+err.Error())
			return
		}
	}
	h.populateCredentialView(credential)
	model.Success(c, credential)
}

func (h *CertHandler) DeleteDNSCredential(c *gin.Context) {
	if !h.requireReady(c) {
		return
	}
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "DNS 凭据 ID 无效")
		return
	}
	credential, err := h.store.GetDNSCredential(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "DNS 凭据不存在")
		return
	}
	issuers, listErr := K8s.ListIssuers()
	if listErr != nil {
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, "检查签发者引用: "+listErr.Error())
		return
	}
	for _, issuer := range issuers {
		if issuer.CredentialSecretName == credential.SecretName {
			model.Error(c, http.StatusConflict, model.CodeConflict, "DNS 凭据仍被签发者引用: "+issuer.Name)
			return
		}
	}
	if err := K8s.DeleteDNSCredentialSecret(credential.Namespace, credential.SecretName); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, "删除 DNS Secret: "+err.Error())
		return
	}
	if err := h.store.DeleteDNSCredential(id); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, gin.H{"id": id})
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
	if err := K8s.DeleteCertificate(ns, name); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
		return
	}
	model.SuccessWithMessage(c, nil, "删除成功")
}

func (h *CertHandler) requireReady(c *gin.Context) bool {
	if K8s == nil {
		k8sUnavailable(c)
		return false
	}
	status := K8s.CertManagerStatus()
	if status.State == k8s.CertManagerStateReady {
		return true
	}
	model.ErrorWithData(c, http.StatusOK, model.CodeK8sAPIError, status.Message, status)
	return false
}
