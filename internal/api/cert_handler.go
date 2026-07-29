package api

import (
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
	request := k8s.IssuerRequest{Name: req.Name, Namespace: req.Namespace, Kind: req.Kind, Mode: req.Mode, Email: req.Email, Server: req.Server, IngressClass: req.IngressClass}
	if req.Mode == "acme_alidns" {
		if h.store == nil {
			model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "DNS 凭据存储未初始化")
			return
		}
		credential, err := h.store.GetDNSCredential(req.CredentialID)
		if err != nil || !credential.Enabled || credential.Provider != "alidns" {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "请选择已启用的 AliDNS 凭据")
			return
		}
		webhook := K8s.AliDNSWebhookStatus()
		if !webhook.Ready {
			model.ErrorWithData(c, http.StatusBadRequest, model.CodeK8sAPIError, "AliDNS Webhook 未就绪: "+webhook.Message, webhook)
			return
		}
		if request.Kind == "Issuer" && credential.Namespace != request.Namespace {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "命名空间级 Issuer 必须使用同命名空间的 AliDNS 凭据")
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

func (h *CertHandler) AliDNSWebhookStatus(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	model.Success(c, K8s.AliDNSWebhookStatus())
}
func (h *CertHandler) InstallAliDNSWebhook(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	status, err := K8s.InstallAliDNSWebhook()
	if err != nil {
		model.ErrorWithData(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error(), status)
		return
	}
	model.SuccessWithMessage(c, status, status.Message)
}

type dnsCredentialRequest struct {
	Name            string  `json:"name"`
	Namespace       string  `json:"namespace"`
	AccessKeyID     string  `json:"access_key_id"`
	AccessKeySecret *string `json:"access_key_secret"`
	Enabled         *bool   `json:"enabled"`
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
	credential, secret, err := h.dnsCredentialFromRequest(req, nil)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	credential.CreatedBy = getUserID(c)
	credential.SecretName = fmt.Sprintf("cylism-alidns-pending-%d", time.Now().UnixNano())
	if err := h.store.CreateDNSCredential(credential); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "DNS 凭据名称无效或已存在")
		return
	}
	credential.SecretName = fmt.Sprintf("cylism-alidns-%d", credential.ID)
	if err := h.store.UpdateDNSCredential(credential); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存 DNS Secret 名称失败")
		return
	}
	if err := K8s.UpsertAliDNSCredentialSecret(credential.Namespace, credential.SecretName, credential.AccessKeyID, secret); err != nil {
		_ = h.store.DeleteDNSCredential(credential.ID)
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, "创建 AliDNS Secret: "+err.Error())
		return
	}
	credential.SecretConfigured = true
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
	credential, secret, err := h.dnsCredentialFromRequest(req, current)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.store.UpdateDNSCredential(credential); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "DNS 凭据名称无效或已存在")
		return
	}
	if err := K8s.UpsertAliDNSCredentialSecret(credential.Namespace, credential.SecretName, credential.AccessKeyID, secret); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, "更新 AliDNS Secret: "+err.Error())
		return
	}
	if current.Namespace != credential.Namespace {
		if err := K8s.DeleteAliDNSCredentialSecret(current.Namespace, current.SecretName); err != nil {
			model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, "清理旧 AliDNS Secret: "+err.Error())
			return
		}
	}
	credential.SecretConfigured = true
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
	if err := K8s.DeleteAliDNSCredentialSecret(credential.Namespace, credential.SecretName); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, "删除 AliDNS Secret: "+err.Error())
		return
	}
	if err := h.store.DeleteDNSCredential(id); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, gin.H{"id": id})
}

func (h *CertHandler) dnsCredentialFromRequest(req dnsCredentialRequest, current *model.DNSCredential) (*model.DNSCredential, string, error) {
	if h.store == nil || len(h.encKey) == 0 {
		return nil, "", fmt.Errorf("DNS 凭据加密未初始化")
	}
	name, namespace, accessKeyID := strings.TrimSpace(req.Name), strings.TrimSpace(req.Namespace), strings.TrimSpace(req.AccessKeyID)
	if name == "" || namespace == "" || accessKeyID == "" {
		return nil, "", fmt.Errorf("名称、命名空间和 AccessKey ID 必填")
	}
	credential := &model.DNSCredential{Name: name, Provider: "alidns", Namespace: namespace, AccessKeyID: accessKeyID, Enabled: true}
	var secret string
	if current != nil {
		credential.ID, credential.CreatedBy, credential.CreatedAt, credential.SecretName, credential.AccessKeySecret = current.ID, current.CreatedBy, current.CreatedAt, current.SecretName, current.AccessKeySecret
		credential.Enabled = current.Enabled
		if req.Enabled != nil {
			credential.Enabled = *req.Enabled
		}
		var err error
		secret, err = crypto.Decrypt(h.encKey, current.AccessKeySecret)
		if err != nil {
			return nil, "", fmt.Errorf("读取 DNS 凭据失败")
		}
	}
	if req.AccessKeySecret != nil && strings.TrimSpace(*req.AccessKeySecret) != "" {
		secret = strings.TrimSpace(*req.AccessKeySecret)
		encrypted, err := crypto.Encrypt(h.encKey, secret)
		if err != nil {
			return nil, "", fmt.Errorf("DNS 凭据加密失败")
		}
		credential.AccessKeySecret = encrypted
	}
	if secret == "" || credential.AccessKeySecret == "" {
		return nil, "", fmt.Errorf("AccessKey Secret 必填")
	}
	return credential, secret, nil
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
