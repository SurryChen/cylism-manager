package network

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	networkservice "github.com/cylism/cylism-manager/internal/service/network"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DomainHandler struct {
	network *networkservice.Service
	k8s     DomainKubernetesAdapter
}

type DomainKubernetesAdapter interface {
	KubernetesAvailable() bool
	GetCertificateContext(context.Context, string, string) (*k8s.CertInfo, error)
	DeleteTLSSecret(context.Context, string, string) error
	NamespaceExists(context.Context, string) (bool, error)
	ListIssuersContext(context.Context) ([]k8s.IssuerInfo, error)
}

type clientDomainAdapter struct{ client *k8s.Client }

// NewDomainKubernetesAdapter creates the narrow Kubernetes port used by the
// domain handler. The concrete client remains an infrastructure concern.
func NewDomainKubernetesAdapter(client *k8s.Client) DomainKubernetesAdapter {
	if client == nil {
		return nil
	}
	return clientDomainAdapter{client: client}
}

func (a clientDomainAdapter) KubernetesAvailable() bool {
	return a.client != nil && (a.client.Clientset != nil || a.client.DynamicClient != nil)
}
func (a clientDomainAdapter) GetCertificateContext(ctx context.Context, ns, name string) (*k8s.CertInfo, error) {
	return a.client.GetCertificateContext(ctx, ns, name)
}
func (a clientDomainAdapter) DeleteTLSSecret(ctx context.Context, ns, name string) error {
	return a.client.Clientset.CoreV1().Secrets(ns).Delete(ctx, name, metav1.DeleteOptions{})
}
func (a clientDomainAdapter) NamespaceExists(ctx context.Context, name string) (bool, error) {
	_, err := a.client.Clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}
func (a clientDomainAdapter) ListIssuersContext(ctx context.Context) ([]k8s.IssuerInfo, error) {
	return a.client.ListIssuersContext(ctx)
}

type domainRequest struct {
	Hostname      string `json:"hostname"`
	EnvironmentID uint   `json:"environment_id"`
	IssuerRef     string `json:"issuer_ref"`
	Description   string `json:"description"`
	Enabled       bool   `json:"enabled"`
}

type importCertificateRequest struct {
	EnvironmentID   uint   `json:"environment_id"`
	CertificateName string `json:"certificate_name"`
	Description     string `json:"description"`
	Enabled         bool   `json:"enabled"`
}

type claimDomainRequest struct {
	EnvironmentID uint `json:"environment_id"`
}

type ManagedDomainInfo struct {
	model.ManagedDomain
	Certificate      *k8s.CertInfo `json:"certificate,omitempty"`
	CertificateError string        `json:"certificate_error,omitempty"`
	ApplicationCount int64         `json:"application_count"`
}

// NewDomainHandlerWithDependencies uses the already configured network
// service and the narrow Kubernetes port supplied by Bootstrap.
func NewDomainHandlerWithDependencies(adapter DomainKubernetesAdapter, service *networkservice.Service) *DomainHandler {
	return &DomainHandler{k8s: adapter, network: service}
}

func (h *DomainHandler) List(c *gin.Context) {
	if c.Query("unassigned") == "true" {
		domains, err := h.network.ListUnassignedManagedDomains()
		if err != nil {
			apiShared.DBError(c, err.Error())
			return
		}
		result := make([]ManagedDomainInfo, 0, len(domains))
		for index := range domains {
			result = append(result, h.DomainInfo(c.Request.Context(), &domains[index]))
		}
		model.Success(c, result)
		return
	}
	environmentID, err := apiShared.OptionalID(strings.TrimSpace(c.Query("environment_id")))
	if err != nil {
		apiShared.BadRequest(c, "环境 ID 无效")
		return
	}
	domains, err := h.network.ListManagedDomains(environmentID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	result := make([]ManagedDomainInfo, 0, len(domains))
	for index := range domains {
		result = append(result, h.DomainInfo(c.Request.Context(), &domains[index]))
	}
	model.Success(c, result)
}

func (h *DomainHandler) Create(c *gin.Context) {
	var req domainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "域名定义无效")
		return
	}
	environment, err := h.domainEnvironment(req.EnvironmentID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	domain, err := domainFromRequest(req, nil, environment)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.validateManagedDomainPrerequisites(c.Request.Context(), domain); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.network.CreateManagedDomain(domain); err != nil {
		apiShared.Conflict(c, "域名已存在")
		return
	}
	assignManagedCertificateNames(domain)
	if err := h.network.UpdateManagedDomain(domain); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	info := h.DomainInfo(c.Request.Context(), domain)
	if err := h.ensureManagedDomainCertificate(c.Request.Context(), domain); err != nil {
		info.CertificateError = err.Error()
		model.SuccessWithMessage(c, info, "域名已创建，但证书申请尚未成功，可在域名列表中重试")
		return
	}
	info = h.DomainInfo(c.Request.Context(), domain)
	model.SuccessWithMessage(c, info, "受管 HTTPS 域名已创建，正在申请证书")
}

func (h *DomainHandler) ListImportableCertificates(c *gin.Context) {
	environmentID, err := apiShared.OptionalID(strings.TrimSpace(c.Query("environment_id")))
	if err != nil || environmentID == 0 {
		apiShared.BadRequest(c, "环境 ID 必填且必须有效")
		return
	}
	environment, err := h.domainEnvironment(environmentID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if h.k8s == nil || !h.k8s.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
	certificates, err := h.network.ListCertificatesContext(c.Request.Context())
	if err != nil {
		apiShared.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	candidates := make([]k8s.CertInfo, 0)
	for _, certificate := range certificates {
		if certificate.Namespace != environment.Namespace || !importableCertificate(certificate) {
			continue
		}
		candidates = append(candidates, certificate)
	}
	model.Success(c, candidates)
}

func (h *DomainHandler) ListClaimable(c *gin.Context) {
	environmentID, err := apiShared.OptionalID(strings.TrimSpace(c.Query("environment_id")))
	if err != nil || environmentID == 0 {
		apiShared.BadRequest(c, "环境 ID 必填且必须有效")
		return
	}
	environment, err := h.domainEnvironment(environmentID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	domains, err := h.network.ListClaimableManagedDomains(environment.Namespace)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	result := make([]ManagedDomainInfo, 0, len(domains))
	for index := range domains {
		result = append(result, h.DomainInfo(c.Request.Context(), &domains[index]))
	}
	model.Success(c, result)
}

func (h *DomainHandler) ImportCertificate(c *gin.Context) {
	var req importCertificateRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.CertificateName) == "" {
		apiShared.BadRequest(c, "环境和 Certificate 名称必填")
		return
	}
	environment, err := h.domainEnvironment(req.EnvironmentID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if h.k8s == nil || !h.k8s.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
	certificate, err := h.network.GetCertificateContext(c.Request.Context(), environment.Namespace, strings.TrimSpace(req.CertificateName))
	if err != nil {
		if apierrors.IsNotFound(err) {
			apiShared.NotFound(c, "Certificate 不存在")
			return
		}
		apiShared.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	if !importableCertificate(*certificate) {
		apiShared.ValidationError(c, "Certificate 必须包含一个精确域名、签发者和 TLS Secret")
		return
	}
	issuerKind := certificate.IssuerKind
	if issuerKind == "" {
		issuerKind = "ClusterIssuer"
	}
	hostname := strings.ToLower(certificate.Domains[0])
	if existing, err := h.network.GetManagedDomainByHostname(hostname); err == nil {
		if existing.EnvironmentID == 0 && existing.Namespace == environment.Namespace {
			apiShared.Conflict(c, "域名存在未关联的历史记录，请使用“关联历史域名”")
			return
		}
		apiShared.Conflict(c, "域名已被平台管理")
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.DBError(c, err.Error())
		return
	}
	domain := &model.ManagedDomain{Hostname: hostname, EnvironmentID: environment.ID, Namespace: environment.Namespace, CertificateName: certificate.Name, TLSSecretName: certificate.SecretName, IssuerRef: certificate.Issuer, IssuerKind: issuerKind, CertificateOwnership: "imported", Description: strings.TrimSpace(req.Description), Enabled: req.Enabled}
	if err := h.network.CreateManagedDomain(domain); err != nil {
		apiShared.Conflict(c, "域名已被平台管理")
		return
	}
	model.SuccessWithMessage(c, h.DomainInfo(c.Request.Context(), domain), "已接管现有证书，Certificate 与 TLS Secret 保持原样")
}

func (h *DomainHandler) Claim(c *gin.Context) {
	domain, ok := h.managedDomain(c)
	if !ok {
		return
	}
	var req claimDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "域名关联定义无效")
		return
	}
	environment, err := h.domainEnvironment(req.EnvironmentID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if domain.EnvironmentID != 0 {
		apiShared.Conflict(c, "域名已关联环境")
		return
	}
	if domain.Namespace == "" || domain.Namespace != environment.Namespace {
		apiShared.Conflict(c, "历史域名命名空间与目标环境不一致，不能关联")
		return
	}
	domain.EnvironmentID = environment.ID
	if err := h.network.UpdateManagedDomain(domain); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	model.SuccessWithMessage(c, h.DomainInfo(c.Request.Context(), domain), "已关联历史域名，Certificate 与 TLS Secret 保持原样")
}

func (h *DomainHandler) Update(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(strings.TrimSpace(c.Param("id")))
	if err != nil {
		apiShared.BadRequest(c, "域名 ID 无效")
		return
	}
	current, err := h.network.GetManagedDomain(id)
	if err != nil {
		apiShared.NotFound(c, "域名不存在")
		return
	}
	var req domainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "域名定义无效")
		return
	}
	environment, err := h.domainEnvironment(req.EnvironmentID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	domain, err := domainFromRequest(req, current, environment)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if !isImportedDomain(domain) {
		if err := h.validateManagedDomainPrerequisites(c.Request.Context(), domain); err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
	}
	assignManagedCertificateNames(domain)
	if err := h.network.UpdateManagedDomain(domain); err != nil {
		apiShared.Conflict(c, "域名已存在")
		return
	}
	info := h.DomainInfo(c.Request.Context(), domain)
	if isImportedDomain(domain) {
		model.SuccessWithMessage(c, info, "导入域名已更新，Certificate 保持原样")
		return
	}
	if err := h.ensureManagedDomainCertificate(c.Request.Context(), domain); err != nil {
		info.CertificateError = err.Error()
		model.SuccessWithMessage(c, info, "域名已更新，但证书申请尚未成功，可重试")
		return
	}
	model.SuccessWithMessage(c, h.DomainInfo(c.Request.Context(), domain), "受管域名已更新，正在同步证书")
}

func (h *DomainHandler) RetryCertificate(c *gin.Context) {
	domain, ok := h.managedDomain(c)
	if !ok {
		return
	}
	if isImportedDomain(domain) {
		apiShared.ValidationError(c, "导入证书由原有 cert-manager 配置维护，不能从平台重新签发")
		return
	}
	if err := h.validateManagedDomainPrerequisites(c.Request.Context(), domain); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.ensureManagedDomainCertificate(c.Request.Context(), domain); err != nil {
		apiShared.ValidationError(c, "申请证书: "+err.Error())
		return
	}
	model.SuccessWithMessage(c, h.DomainInfo(c.Request.Context(), domain), "证书申请已重新提交")
}

func (h *DomainHandler) ListOperations(c *gin.Context) {
	domain, ok := h.managedDomain(c)
	if !ok {
		return
	}
	if h.k8s == nil || domain.Namespace == "" || domain.CertificateName == "" {
		apiShared.ValidationError(c, "域名尚未绑定证书")
		return
	}
	operations, err := h.network.ListCertificateOperationsContext(c.Request.Context(), domain.Namespace, domain.CertificateName)
	if err != nil {
		apiShared.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, operations)
}

func (h *DomainHandler) Delete(c *gin.Context) {
	domain, ok := h.managedDomain(c)
	if !ok {
		return
	}
	count, err := h.network.CountApplicationEndpointsByDomain(domain.ID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if err := networkservice.ValidateDomainDeletion(count); err != nil {
		apiShared.Conflict(c, err.Error())
		return
	}
	if !isImportedDomain(domain) && h.k8s != nil && domain.Namespace != "" && domain.CertificateName != "" {
		if err := h.network.DeleteCertificateContext(c.Request.Context(), domain.Namespace, domain.CertificateName); err != nil && !apierrors.IsNotFound(err) {
			apiShared.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, "删除域名证书: "+err.Error())
			return
		}
		if domain.TLSSecretName != "" {
			if err := h.k8s.DeleteTLSSecret(c.Request.Context(), domain.Namespace, domain.TLSSecretName); err != nil && !apierrors.IsNotFound(err) {
				apiShared.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, "删除域名 TLS Secret: "+err.Error())
				return
			}
		}
	}
	if err := h.network.DeleteManagedDomain(domain.ID); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	model.Success(c, gin.H{"id": domain.ID})
}

func (h *DomainHandler) managedDomain(c *gin.Context) (*model.ManagedDomain, bool) {
	id, err := apiShared.ParsePositiveID(strings.TrimSpace(c.Param("id")))
	if err != nil {
		apiShared.BadRequest(c, "域名 ID 无效")
		return nil, false
	}
	domain, err := h.network.GetManagedDomain(id)
	if err != nil {
		apiShared.NotFound(c, "域名不存在")
		return nil, false
	}
	return domain, true
}

func (h *DomainHandler) domainEnvironment(environmentID uint) (*model.Environment, error) {
	if h.network == nil {
		return nil, errInvalid("网络服务未初始化")
	}
	environment, err := h.network.ResolveEnvironment(environmentID)
	if err != nil {
		return nil, errInvalid(err.Error())
	}
	return environment, nil
}

func (h *DomainHandler) DomainInfo(ctx context.Context, domain *model.ManagedDomain) ManagedDomainInfo {
	return ManagedDomainInfoFor(ctx, domain, h.k8s, h.network)
}

// ManagedDomainInfoFor builds the read-only domain view from the two
// capabilities it actually needs: endpoint-reference counting and optional
// certificate lookup. It lets workspace views reuse the DTO without creating
// a full domain lifecycle service.
func ManagedDomainInfoFor(ctx context.Context, domain *model.ManagedDomain, client interface {
	GetCertificateContext(context.Context, string, string) (*k8s.CertInfo, error)
}, references interface {
	CountApplicationEndpointsByDomain(uint) (int64, error)
}) ManagedDomainInfo {
	if domain == nil {
		return ManagedDomainInfo{}
	}
	view := ManagedDomainInfo{ManagedDomain: *domain}
	if references != nil {
		if count, err := references.CountApplicationEndpointsByDomain(domain.ID); err == nil {
			view.ApplicationCount = count
		}
	}
	if client == nil || strings.TrimSpace(domain.Namespace) == "" || strings.TrimSpace(domain.CertificateName) == "" {
		if strings.TrimSpace(domain.Namespace) == "" {
			view.CertificateError = "域名尚未绑定命名空间，请编辑后申请证书"
		}
		return view
	}
	certificate, err := client.GetCertificateContext(ctx, domain.Namespace, domain.CertificateName)
	if err != nil {
		view.CertificateError = normalizeCertificateViewError(err.Error())
		return view
	}
	view.Certificate = certificate
	return view
}

func normalizeCertificateViewError(message string) string {
	if strings.Contains(strings.ToLower(message), "not found") || strings.Contains(message, "不存在") {
		return "证书尚未创建"
	}
	return message
}

func domainFromRequest(req domainRequest, current *model.ManagedDomain, environment *model.Environment) (*model.ManagedDomain, error) {
	domain, err := networkservice.BuildManagedDomain(networkservice.DomainInput{
		Hostname: req.Hostname, EnvironmentID: req.EnvironmentID, IssuerRef: req.IssuerRef,
		Description: req.Description, Enabled: req.Enabled,
	}, current, environment)
	if err != nil {
		return nil, errInvalid(err.Error())
	}
	return domain, nil
}

func importableCertificate(certificate k8s.CertInfo) bool {
	return len(certificate.Domains) == 1 && validManagedHostname(strings.ToLower(certificate.Domains[0])) && certificate.Issuer != "" && certificate.SecretName != ""
}

func isImportedDomain(domain *model.ManagedDomain) bool {
	return domain != nil && domain.CertificateOwnership == "imported"
}

func validManagedHostname(hostname string) bool {
	if len(hostname) < 3 || len(hostname) > 253 || !strings.Contains(hostname, ".") || strings.HasPrefix(hostname, "*.") {
		return false
	}
	for _, label := range strings.Split(hostname, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-') {
				return false
			}
		}
	}
	return true
}

func assignManagedCertificateNames(domain *model.ManagedDomain) {
	if domain.CertificateName == "" {
		domain.CertificateName = fmt.Sprintf("cylism-domain-%d", domain.ID)
	}
	if domain.TLSSecretName == "" {
		domain.TLSSecretName = domain.CertificateName + "-tls"
	}
}

func (h *DomainHandler) validateManagedDomainPrerequisites(ctx context.Context, domain *model.ManagedDomain) error {
	if h.k8s == nil || !h.k8s.KubernetesAvailable() {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	return h.network.ValidateManagedDomainPrerequisites(ctx, domain, networkservice.DomainPrerequisiteAdapter{
		NamespaceExists: func(ctx context.Context, namespace string) (bool, error) {
			return h.k8s.NamespaceExists(ctx, namespace)
		},
		ListIssuers: func(ctx context.Context) ([]networkservice.Issuer, error) {
			issuers, err := h.k8s.ListIssuersContext(ctx)
			if err != nil {
				return nil, err
			}
			result := make([]networkservice.Issuer, 0, len(issuers))
			for _, issuer := range issuers {
				result = append(result, networkservice.Issuer{Name: issuer.Name, Kind: issuer.Kind, Ready: issuer.Ready})
			}
			return result, nil
		},
	})
}

func (h *DomainHandler) ensureManagedDomainCertificate(ctx context.Context, domain *model.ManagedDomain) error {
	if h.k8s == nil || !h.k8s.KubernetesAvailable() {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	_, err := h.network.EnsureCertificateContext(ctx, k8s.CreateCertificateRequest{Name: domain.CertificateName, Namespace: domain.Namespace, Domains: []string{domain.Hostname}, IssuerRef: domain.IssuerRef, IssuerKind: "ClusterIssuer", SecretName: domain.TLSSecretName})
	return err
}

func errInvalid(message string) error { return errors.New(message) }
