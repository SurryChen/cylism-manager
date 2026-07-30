package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DomainHandler struct{ store *store.Store }

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

type managedDomainInfo struct {
	model.ManagedDomain
	Certificate      *k8s.CertInfo `json:"certificate,omitempty"`
	CertificateError string        `json:"certificate_error,omitempty"`
	ApplicationCount int64         `json:"application_count"`
}

func NewDomainHandler(s *store.Store) *DomainHandler { return &DomainHandler{store: s} }

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
		domains, err := h.store.ListUnassignedManagedDomains()
		if err != nil {
			model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
			return
		}
		result := make([]managedDomainInfo, 0, len(domains))
		for index := range domains {
			result = append(result, h.domainInfo(&domains[index]))
		}
		model.Success(c, result)
		return
	}
	environmentID, err := optionalQueryID(c, "environment_id")
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "环境 ID 无效")
		return
	}
	domains, err := h.store.ListManagedDomains(environmentID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	result := make([]managedDomainInfo, 0, len(domains))
	for index := range domains {
		result = append(result, h.domainInfo(&domains[index]))
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
	if err := validateManagedDomainPrerequisites(domain); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.store.CreateManagedDomain(domain); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "域名已存在")
		return
	}
	assignManagedCertificateNames(domain)
	if err := h.store.UpdateManagedDomain(domain); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	info := h.domainInfo(domain)
	if err := ensureManagedDomainCertificate(domain); err != nil {
		info.CertificateError = err.Error()
		model.SuccessWithMessage(c, info, "域名已创建，但证书申请尚未成功，可在域名列表中重试")
		return
	}
	info = h.domainInfo(domain)
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
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	certificates, err := K8s.ListCertificates()
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
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	certificate, err := K8s.GetCertificate(environment.Namespace, strings.TrimSpace(req.CertificateName))
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
	domain := &model.ManagedDomain{Hostname: strings.ToLower(certificate.Domains[0]), EnvironmentID: environment.ID, Namespace: environment.Namespace, CertificateName: certificate.Name, TLSSecretName: certificate.SecretName, IssuerRef: certificate.Issuer, IssuerKind: issuerKind, CertificateOwnership: "imported", Description: strings.TrimSpace(req.Description), Enabled: req.Enabled}
	if err := h.store.CreateManagedDomain(domain); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "域名已被平台管理")
		return
	}
	model.SuccessWithMessage(c, h.domainInfo(domain), "已接管现有证书，Certificate 与 TLS Secret 保持原样")
}

func (h *DomainHandler) Update(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "域名 ID 无效")
		return
	}
	current, err := h.store.GetManagedDomain(id)
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
		if err := validateManagedDomainPrerequisites(domain); err != nil {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
			return
		}
	}
	assignManagedCertificateNames(domain)
	if err := h.store.UpdateManagedDomain(domain); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "域名已存在")
		return
	}
	info := h.domainInfo(domain)
	if isImportedDomain(domain) {
		model.SuccessWithMessage(c, info, "导入域名已更新，Certificate 保持原样")
		return
	}
	if err := ensureManagedDomainCertificate(domain); err != nil {
		info.CertificateError = err.Error()
		model.SuccessWithMessage(c, info, "域名已更新，但证书申请尚未成功，可重试")
		return
	}
	model.SuccessWithMessage(c, h.domainInfo(domain), "受管域名已更新，正在同步证书")
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
	if err := validateManagedDomainPrerequisites(domain); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := ensureManagedDomainCertificate(domain); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "申请证书: "+err.Error())
		return
	}
	model.SuccessWithMessage(c, h.domainInfo(domain), "证书申请已重新提交")
}

func (h *DomainHandler) ListOperations(c *gin.Context) {
	domain, ok := h.managedDomain(c)
	if !ok {
		return
	}
	if K8s == nil || domain.Namespace == "" || domain.CertificateName == "" {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "域名尚未绑定证书")
		return
	}
	operations, err := K8s.ListCertificateOperations(domain.Namespace, domain.CertificateName)
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
	count, err := h.store.CountApplicationEndpointsByDomain(domain.ID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	if count > 0 {
		model.Error(c, http.StatusConflict, model.CodeConflict, "域名仍被应用入口引用，请先发布不使用该域名的新版本")
		return
	}
	if !isImportedDomain(domain) && K8s != nil && domain.Namespace != "" && domain.CertificateName != "" {
		if err := K8s.DeleteCertificate(domain.Namespace, domain.CertificateName); err != nil && !apierrors.IsNotFound(err) {
			model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, "删除域名证书: "+err.Error())
			return
		}
		if domain.TLSSecretName != "" {
			if err := K8s.Clientset.CoreV1().Secrets(domain.Namespace).Delete(K8s.Ctx(), domain.TLSSecretName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, "删除域名 TLS Secret: "+err.Error())
				return
			}
		}
	}
	if err := h.store.DeleteManagedDomain(domain.ID); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, gin.H{"id": domain.ID})
}

func (h *DomainHandler) managedDomain(c *gin.Context) (*model.ManagedDomain, bool) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "域名 ID 无效")
		return nil, false
	}
	domain, err := h.store.GetManagedDomain(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "域名不存在")
		return nil, false
	}
	return domain, true
}

func (h *DomainHandler) domainEnvironment(environmentID uint) (*model.Environment, error) {
	if environmentID == 0 {
		return nil, errInvalid("请选择域名所属环境")
	}
	environment, err := h.store.GetEnvironmentByID(environmentID)
	if err != nil {
		return nil, errInvalid("环境不存在")
	}
	if environment.NamespaceConflict {
		return nil, errInvalid("环境命名空间存在冲突，请先完成迁移")
	}
	conflicts, err := h.store.ListEnvironmentNamespaceConflicts()
	if err != nil {
		return nil, fmt.Errorf("检查命名空间归属: %w", err)
	}
	for _, conflict := range conflicts {
		if conflict.Namespace == environment.Namespace {
			return nil, errInvalid("环境命名空间存在冲突，请先完成迁移")
		}
	}
	return environment, nil
}

func (h *DomainHandler) domainInfo(domain *model.ManagedDomain) managedDomainInfo {
	info := managedDomainInfo{ManagedDomain: *domain}
	count, err := h.store.CountApplicationEndpointsByDomain(domain.ID)
	if err == nil {
		info.ApplicationCount = count
	}
	if K8s == nil || domain.Namespace == "" || domain.CertificateName == "" {
		if domain.Namespace == "" {
			info.CertificateError = "域名尚未绑定命名空间，请编辑后申请证书"
		}
		return info
	}
	certificate, err := K8s.GetCertificate(domain.Namespace, domain.CertificateName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			info.CertificateError = err.Error()
		} else {
			info.CertificateError = "证书尚未创建"
		}
		return info
	}
	info.Certificate = certificate
	return info
}

func domainFromRequest(req domainRequest, current *model.ManagedDomain, environment *model.Environment) (*model.ManagedDomain, error) {
	hostname := strings.ToLower(strings.TrimSpace(req.Hostname))
	if !validManagedHostname(hostname) {
		return nil, errInvalid("域名必须是合法的精确 DNS 名称，且不支持泛域名")
	}
	if environment == nil || environment.ID == 0 || strings.TrimSpace(environment.Namespace) == "" {
		return nil, errInvalid("请选择域名所属环境")
	}
	issuerRef := strings.TrimSpace(req.IssuerRef)
	if issuerRef == "" {
		return nil, errInvalid("请选择已就绪的 ClusterIssuer")
	}
	if current != nil && current.EnvironmentID != 0 && current.EnvironmentID != environment.ID {
		return nil, errInvalid("域名已绑定环境，不能直接修改")
	}
	domain := &model.ManagedDomain{Hostname: hostname, EnvironmentID: environment.ID, Namespace: environment.Namespace, CertificateName: currentCertificateName(current), TLSSecretName: currentTLSSecretName(current), IssuerRef: issuerRef, IssuerKind: "ClusterIssuer", CertificateOwnership: "managed", Description: strings.TrimSpace(req.Description), Enabled: req.Enabled}
	if current != nil {
		domain.ID = current.ID
		domain.CreatedAt = current.CreatedAt
		domain.CertificateOwnership = current.CertificateOwnership
		if isImportedDomain(current) {
			domain.IssuerRef = current.IssuerRef
			domain.IssuerKind = current.IssuerKind
		}
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

func currentCertificateName(domain *model.ManagedDomain) string {
	if domain == nil {
		return ""
	}
	return domain.CertificateName
}

func currentTLSSecretName(domain *model.ManagedDomain) string {
	if domain == nil {
		return ""
	}
	return domain.TLSSecretName
}

func assignManagedCertificateNames(domain *model.ManagedDomain) {
	if domain.CertificateName == "" {
		domain.CertificateName = fmt.Sprintf("cylism-domain-%d", domain.ID)
	}
	if domain.TLSSecretName == "" {
		domain.TLSSecretName = domain.CertificateName + "-tls"
	}
}

func validateManagedDomainPrerequisites(domain *model.ManagedDomain) error {
	if K8s == nil || K8s.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	if _, err := K8s.Clientset.CoreV1().Namespaces().Get(K8s.Ctx(), domain.Namespace, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("命名空间 %q 不存在", domain.Namespace)
		}
		return fmt.Errorf("检查命名空间: %w", err)
	}
	issuers, err := K8s.ListIssuers()
	if err != nil {
		return fmt.Errorf("读取签发者: %w", err)
	}
	for _, issuer := range issuers {
		if issuer.Kind == "ClusterIssuer" && issuer.Name == domain.IssuerRef && issuer.Ready {
			return nil
		}
	}
	return fmt.Errorf("ClusterIssuer %q 不存在或尚未就绪", domain.IssuerRef)
}

func ensureManagedDomainCertificate(domain *model.ManagedDomain) error {
	if K8s == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	_, err := K8s.EnsureCertificate(k8s.CreateCertificateRequest{Name: domain.CertificateName, Namespace: domain.Namespace, Domains: []string{domain.Hostname}, IssuerRef: domain.IssuerRef, IssuerKind: "ClusterIssuer", SecretName: domain.TLSSecretName})
	return err
}
