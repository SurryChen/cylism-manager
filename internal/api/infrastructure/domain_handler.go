package infrastructure

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	networkservice "github.com/cylism/cylism-manager/internal/service/network"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DomainHandler struct {
	store   *store.Store
	network *networkservice.Service
	k8s     *k8s.Client
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

func NewDomainHandler(s *store.Store, client *k8s.Client) *DomainHandler {
	return &DomainHandler{store: s, k8s: client, network: networkservice.NewService(s).WithCertificateAdapter(client).WithDNSAdapter(client).WithIngressAdapter(client)}
}

func NewDomainHandlerWithService(s *store.Store, client *k8s.Client, service *networkservice.Service) *DomainHandler {
	if service == nil {
		service = networkservice.NewService(s)
	}
	service.WithCertificateAdapter(client).WithDNSAdapter(client).WithIngressAdapter(client)
	return &DomainHandler{store: s, k8s: client, network: service}
}

func optionalQueryID(c *gin.Context, key string) (uint, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseUint(raw, 10, 0)
	return uint(value), err
}

func (h *DomainHandler) List(c *gin.Context) {
	if c.Query("unassigned") == "true" {
		domains, err := h.network.ListUnassignedManagedDomains()
		if err != nil {
			model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
			return
		}
		result := make([]ManagedDomainInfo, 0, len(domains))
		for index := range domains {
			result = append(result, h.DomainInfo(&domains[index]))
		}
		model.Success(c, result)
		return
	}
	environmentID, err := optionalQueryID(c, "environment_id")
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "环境 ID 无效")
		return
	}
	domains, err := h.network.ListManagedDomains(environmentID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	result := make([]ManagedDomainInfo, 0, len(domains))
	for index := range domains {
		result = append(result, h.DomainInfo(&domains[index]))
	}
	model.Success(c, result)
}

func (h *DomainHandler) Create(c *gin.Context) {
	var req domainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "域名定义无效")
		return
	}
	environment, err := h.domainEnvironment(req.EnvironmentID)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	domain, err := domainFromRequest(req, nil, environment)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.validateManagedDomainPrerequisites(domain); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.network.CreateManagedDomain(domain); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "域名已存在")
		return
	}
	assignManagedCertificateNames(domain)
	if err := h.network.UpdateManagedDomain(domain); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	info := h.DomainInfo(domain)
	if err := h.ensureManagedDomainCertificate(domain); err != nil {
		info.CertificateError = err.Error()
		model.SuccessWithMessage(c, info, "域名已创建，但证书申请尚未成功，可在域名列表中重试")
		return
	}
	info = h.DomainInfo(domain)
	model.SuccessWithMessage(c, info, "受管 HTTPS 域名已创建，正在申请证书")
}

func (h *DomainHandler) ListImportableCertificates(c *gin.Context) {
	environmentID, err := optionalQueryID(c, "environment_id")
	if err != nil || environmentID == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "环境 ID 必填且必须有效")
		return
	}
	environment, err := h.domainEnvironment(environmentID)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if h.k8s == nil {
		model.Error(c, http.StatusOK, model.CodeK8sUnavailable, "K8s 集群未连接")
		return
	}
	certificates, err := h.network.ListCertificates()
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
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
	environmentID, err := optionalQueryID(c, "environment_id")
	if err != nil || environmentID == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "环境 ID 必填且必须有效")
		return
	}
	environment, err := h.domainEnvironment(environmentID)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	domains, err := h.network.ListClaimableManagedDomains(environment.Namespace)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	result := make([]ManagedDomainInfo, 0, len(domains))
	for index := range domains {
		result = append(result, h.DomainInfo(&domains[index]))
	}
	model.Success(c, result)
}

func (h *DomainHandler) ImportCertificate(c *gin.Context) {
	var req importCertificateRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.CertificateName) == "" {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "环境和 Certificate 名称必填")
		return
	}
	environment, err := h.domainEnvironment(req.EnvironmentID)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if h.k8s == nil {
		model.Error(c, http.StatusOK, model.CodeK8sUnavailable, "K8s 集群未连接")
		return
	}
	certificate, err := h.network.GetCertificate(environment.Namespace, strings.TrimSpace(req.CertificateName))
	if err != nil {
		if apierrors.IsNotFound(err) {
			model.Error(c, http.StatusNotFound, model.CodeNotFound, "Certificate 不存在")
			return
		}
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	if !importableCertificate(*certificate) {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "Certificate 必须包含一个精确域名、签发者和 TLS Secret")
		return
	}
	issuerKind := certificate.IssuerKind
	if issuerKind == "" {
		issuerKind = "ClusterIssuer"
	}
	hostname := strings.ToLower(certificate.Domains[0])
	if existing, err := h.network.GetManagedDomainByHostname(hostname); err == nil {
		if existing.EnvironmentID == 0 && existing.Namespace == environment.Namespace {
			model.Error(c, http.StatusConflict, model.CodeConflict, "域名存在未关联的历史记录，请使用“关联历史域名”")
			return
		}
		model.Error(c, http.StatusConflict, model.CodeConflict, "域名已被平台管理")
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	domain := &model.ManagedDomain{Hostname: hostname, EnvironmentID: environment.ID, Namespace: environment.Namespace, CertificateName: certificate.Name, TLSSecretName: certificate.SecretName, IssuerRef: certificate.Issuer, IssuerKind: issuerKind, CertificateOwnership: "imported", Description: strings.TrimSpace(req.Description), Enabled: req.Enabled}
	if err := h.network.CreateManagedDomain(domain); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "域名已被平台管理")
		return
	}
	model.SuccessWithMessage(c, h.DomainInfo(domain), "已接管现有证书，Certificate 与 TLS Secret 保持原样")
}

func (h *DomainHandler) Claim(c *gin.Context) {
	domain, ok := h.managedDomain(c)
	if !ok {
		return
	}
	var req claimDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "域名关联定义无效")
		return
	}
	environment, err := h.domainEnvironment(req.EnvironmentID)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if domain.EnvironmentID != 0 {
		model.Error(c, http.StatusConflict, model.CodeConflict, "域名已关联环境")
		return
	}
	if domain.Namespace == "" || domain.Namespace != environment.Namespace {
		model.Error(c, http.StatusConflict, model.CodeConflict, "历史域名命名空间与目标环境不一致，不能关联")
		return
	}
	domain.EnvironmentID = environment.ID
	if err := h.network.UpdateManagedDomain(domain); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.SuccessWithMessage(c, h.DomainInfo(domain), "已关联历史域名，Certificate 与 TLS Secret 保持原样")
}

func (h *DomainHandler) Update(c *gin.Context) {
	id, err := parseDomainID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "域名 ID 无效")
		return
	}
	current, err := h.network.GetManagedDomain(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "域名不存在")
		return
	}
	var req domainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "域名定义无效")
		return
	}
	environment, err := h.domainEnvironment(req.EnvironmentID)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	domain, err := domainFromRequest(req, current, environment)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if !isImportedDomain(domain) {
		if err := h.validateManagedDomainPrerequisites(domain); err != nil {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
			return
		}
	}
	assignManagedCertificateNames(domain)
	if err := h.network.UpdateManagedDomain(domain); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "域名已存在")
		return
	}
	info := h.DomainInfo(domain)
	if isImportedDomain(domain) {
		model.SuccessWithMessage(c, info, "导入域名已更新，Certificate 保持原样")
		return
	}
	if err := h.ensureManagedDomainCertificate(domain); err != nil {
		info.CertificateError = err.Error()
		model.SuccessWithMessage(c, info, "域名已更新，但证书申请尚未成功，可重试")
		return
	}
	model.SuccessWithMessage(c, h.DomainInfo(domain), "受管域名已更新，正在同步证书")
}

func (h *DomainHandler) RetryCertificate(c *gin.Context) {
	domain, ok := h.managedDomain(c)
	if !ok {
		return
	}
	if isImportedDomain(domain) {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "导入证书由原有 cert-manager 配置维护，不能从平台重新签发")
		return
	}
	if err := h.validateManagedDomainPrerequisites(domain); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.ensureManagedDomainCertificate(domain); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "申请证书: "+err.Error())
		return
	}
	model.SuccessWithMessage(c, h.DomainInfo(domain), "证书申请已重新提交")
}

func (h *DomainHandler) ListOperations(c *gin.Context) {
	domain, ok := h.managedDomain(c)
	if !ok {
		return
	}
	if h.k8s == nil || domain.Namespace == "" || domain.CertificateName == "" {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "域名尚未绑定证书")
		return
	}
	operations, err := h.network.ListCertificateOperations(domain.Namespace, domain.CertificateName)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
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
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	if err := networkservice.ValidateDomainDeletion(count); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, err.Error())
		return
	}
	if !isImportedDomain(domain) && h.k8s != nil && domain.Namespace != "" && domain.CertificateName != "" {
		if err := h.network.DeleteCertificate(domain.Namespace, domain.CertificateName); err != nil && !apierrors.IsNotFound(err) {
			model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, "删除域名证书: "+err.Error())
			return
		}
		if domain.TLSSecretName != "" {
			if err := h.k8s.Clientset.CoreV1().Secrets(domain.Namespace).Delete(h.k8s.Ctx(), domain.TLSSecretName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, "删除域名 TLS Secret: "+err.Error())
				return
			}
		}
	}
	if err := h.network.DeleteManagedDomain(domain.ID); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, gin.H{"id": domain.ID})
}

func (h *DomainHandler) managedDomain(c *gin.Context) (*model.ManagedDomain, bool) {
	id, err := parseDomainID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "域名 ID 无效")
		return nil, false
	}
	domain, err := h.network.GetManagedDomain(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "域名不存在")
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

func (h *DomainHandler) DomainInfo(domain *model.ManagedDomain) ManagedDomainInfo {
	if h.k8s == nil {
		view := ManagedDomainInfo{ManagedDomain: *domain}
		if count, err := h.network.CountApplicationEndpointsByDomain(domain.ID); err == nil {
			view.ApplicationCount = count
		}
		if domain.Namespace == "" {
			view.CertificateError = "域名尚未绑定命名空间，请编辑后申请证书"
		}
		return view
	}
	view := h.network.BuildManagedDomainView(domain)
	return ManagedDomainInfo{
		ManagedDomain:    view.Domain,
		Certificate:      view.Certificate,
		CertificateError: normalizeCertificateViewError(view.CertificateError),
		ApplicationCount: view.ApplicationCount,
	}
}

func normalizeCertificateViewError(message string) string {
	if strings.Contains(strings.ToLower(message), "not found") || strings.Contains(message, "不存在") {
		return "证书尚未创建"
	}
	return message
}

func domainFromRequest(req domainRequest, current *model.ManagedDomain, environment *model.Environment) (*model.ManagedDomain, error) {
	service := networkservice.NewService(nil)
	domain, err := service.BuildManagedDomain(networkservice.DomainInput{
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

func (h *DomainHandler) validateManagedDomainPrerequisites(domain *model.ManagedDomain) error {
	if h.k8s == nil || h.k8s.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	return h.network.ValidateManagedDomainPrerequisites(domain, networkservice.DomainPrerequisiteAdapter{
		NamespaceExists: func(namespace string) (bool, error) {
			_, err := h.k8s.Clientset.CoreV1().Namespaces().Get(h.k8s.Ctx(), namespace, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return err == nil, err
		},
		ListIssuers: func() ([]networkservice.Issuer, error) {
			issuers, err := h.k8s.ListIssuers()
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

func (h *DomainHandler) ensureManagedDomainCertificate(domain *model.ManagedDomain) error {
	if h.k8s == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	_, err := h.network.EnsureCertificate(k8s.CreateCertificateRequest{Name: domain.CertificateName, Namespace: domain.Namespace, Domains: []string{domain.Hostname}, IssuerRef: domain.IssuerRef, IssuerKind: "ClusterIssuer", SecretName: domain.TLSSecretName})
	return err
}

func parseDomainID(value string) (uint, error) {
	id, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	return uint(id), err
}

func errInvalid(message string) error { return errors.New(message) }
