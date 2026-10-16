package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	cloudservice "github.com/cylism/cylism-manager/internal/service/cloud"
	networkservice "github.com/cylism/cylism-manager/internal/service/network"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DomainHandler struct {
	network       *networkservice.Service
	k8s           DomainKubernetesAdapter
	clusterClient func(context.Context, uint) (*k8s.Client, error)
	cloud         domainCloudDNS
	servers       domainServerReader
}

type DomainKubernetesAdapter interface {
	KubernetesAvailable() bool
	GetCertificateContext(context.Context, string, string) (*k8s.CertInfo, error)
	DeleteTLSSecret(context.Context, string, string) error
	NamespaceExists(context.Context, string) (bool, error)
	ListIssuersContext(context.Context) ([]k8s.IssuerInfo, error)
	ReadyIngressEndpoint(context.Context, string, string) (bool, error)
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

func (a clientDomainAdapter) ReadyIngressEndpoint(ctx context.Context, namespace, hostname string) (bool, error) {
	ingresses, err := a.client.Clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	for _, ingress := range ingresses.Items {
		for _, rule := range ingress.Spec.Rules {
			if !strings.EqualFold(rule.Host, hostname) || rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service == nil {
					continue
				}
				endpoints, err := a.client.Clientset.CoreV1().Endpoints(namespace).Get(ctx, path.Backend.Service.Name, metav1.GetOptions{})
				if apierrors.IsNotFound(err) {
					continue
				}
				if err != nil {
					return false, err
				}
				for _, subset := range endpoints.Subsets {
					if len(subset.Addresses) > 0 {
						return true, nil
					}
				}
			}
		}
	}
	return false, nil
}

type domainRequest struct {
	Hostname          string `json:"hostname"`
	EnvironmentID     uint   `json:"environment_id"`
	IssuerRef         string `json:"issuer_ref"`
	Description       string `json:"description"`
	Enabled           bool   `json:"enabled"`
	CloudConnectionID uint   `json:"cloud_connection_id"`
	DNSZone           string `json:"dns_zone"`
	NodeIDs           []uint `json:"node_ids"`
}

type importCertificateRequest struct {
	EnvironmentID     uint   `json:"environment_id"`
	CertificateName   string `json:"certificate_name"`
	Description       string `json:"description"`
	Enabled           bool   `json:"enabled"`
	CloudConnectionID uint   `json:"cloud_connection_id"`
	DNSZone           string `json:"dns_zone"`
	NodeIDs           []uint `json:"node_ids"`
}

type ManagedDomainInfo struct {
	ID                   uint          `json:"id"`
	Hostname             string        `json:"hostname"`
	EnvironmentID        uint          `json:"environment_id,omitempty"`
	Namespace            string        `json:"namespace"`
	CertificateName      string        `json:"certificate_name"`
	TLSSecretName        string        `json:"tls_secret_name"`
	IssuerRef            string        `json:"issuer_ref"`
	IssuerKind           string        `json:"issuer_kind"`
	CertificateOwnership string        `json:"certificate_ownership"`
	Description          string        `json:"description"`
	Enabled              bool          `json:"enabled"`
	CreatedAt            time.Time     `json:"created_at"`
	UpdatedAt            time.Time     `json:"updated_at"`
	Certificate          *k8s.CertInfo `json:"certificate,omitempty"`
	CertificateError     string        `json:"certificate_error,omitempty"`
	ApplicationCount     int64         `json:"application_count"`
	CloudConnectionID    uint          `json:"cloud_connection_id"`
	DNSZone              string        `json:"dns_zone"`
	DNSNodeIDs           []uint        `json:"dns_node_ids"`
	DNSAddresses         []string      `json:"dns_addresses"`
	OriginalDNSAddresses []string      `json:"original_dns_addresses"`
	AdoptionStatus       string        `json:"adoption_status"`
	DNSStatus            string        `json:"dns_status"`
	DNSError             string        `json:"dns_error"`
}

// NewDomainHandlerWithDependencies uses the already configured network
// service and the narrow Kubernetes port supplied by Bootstrap.
func NewDomainHandlerWithDependencies(adapter DomainKubernetesAdapter, service *networkservice.Service) *DomainHandler {
	return &DomainHandler{k8s: adapter, network: service}
}

func (h *DomainHandler) WithClusterClientFactory(factory func(context.Context, uint) (*k8s.Client, error)) *DomainHandler {
	h.clusterClient = factory
	return h
}

func (h *DomainHandler) WithCloudDNS(cloud *cloudservice.Service, servers domainServerReader) *DomainHandler {
	h.cloud, h.servers = cloud, servers
	return h
}

func (h *DomainHandler) forEnvironment(ctx context.Context, environment *model.Environment) (*DomainHandler, error) {
	if h.clusterClient == nil {
		return h, nil
	}
	if environment == nil || environment.ClusterID == 0 {
		return nil, fmt.Errorf("环境尚未绑定集群")
	}
	client, err := h.clusterClient(ctx, environment.ClusterID)
	if err != nil {
		return nil, fmt.Errorf("连接环境所属集群失败: %w", err)
	}
	scoped := *h
	scoped.k8s = NewDomainKubernetesAdapter(client)
	scoped.network = h.network.ForCluster(client)
	scoped.clusterClient = nil
	return &scoped, nil
}

func (h *DomainHandler) forDomain(ctx context.Context, domain *model.ManagedDomain) (*DomainHandler, error) {
	if h.clusterClient == nil {
		return h, nil
	}
	if domain == nil || domain.EnvironmentID == 0 {
		return nil, fmt.Errorf("域名尚未关联集群环境")
	}
	environment, err := h.domainEnvironment(domain.EnvironmentID)
	if err != nil {
		return nil, err
	}
	return h.forEnvironment(ctx, environment)
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
		apiShared.Success(c, result)
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
	apiShared.Success(c, result)
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
	h, err = h.forEnvironment(c.Request.Context(), environment)
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
	if h.cloud != nil {
		if err := h.prepareCloudDomain(c.Request.Context(), domain, environment, req.CloudConnectionID, req.DNSZone, req.NodeIDs, true); err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
		if err := h.checkAvailableCloudRecord(c.Request.Context(), domain); err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
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
	if h.cloud != nil {
		if err := h.syncDNS(c.Request.Context(), domain); err != nil {
			apiShared.SuccessWithMessage(c, h.DomainInfo(c.Request.Context(), domain), "域名已保存，但 DNS 解析失败，请在列表中重试："+err.Error())
			return
		}
	}
	info := h.DomainInfo(c.Request.Context(), domain)
	if err := h.ensureManagedDomainCertificate(c.Request.Context(), domain); err != nil {
		info.CertificateError = err.Error()
		apiShared.SuccessWithMessage(c, info, "域名已创建，但证书申请尚未成功，可在域名列表中重试")
		return
	}
	info = h.DomainInfo(c.Request.Context(), domain)
	apiShared.SuccessWithMessage(c, info, "受管 HTTPS 域名已创建，正在申请证书")
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
	h, err = h.forEnvironment(c.Request.Context(), environment)
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
		apiShared.Error(c, http.StatusBadRequest, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	candidates := make([]k8s.CertInfo, 0)
	for _, certificate := range certificates {
		if certificate.Namespace != environment.Namespace || !importableCertificate(certificate) {
			continue
		}
		candidates = append(candidates, certificate)
	}
	apiShared.Success(c, candidates)
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
	h, err = h.forEnvironment(c.Request.Context(), environment)
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
		apiShared.Error(c, http.StatusBadRequest, apiShared.CodeK8sAPIError, err.Error())
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
	var existingRecords []cloudservice.DNSRecord
	if h.cloud != nil {
		existingRecords, err = h.prepareImportDNS(c.Request.Context(), environment, hostname, req.CloudConnectionID, req.DNSZone, req.NodeIDs)
		if err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
	}
	existing, err := h.network.GetManagedDomainByHostname(hostname)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.DBError(c, err.Error())
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		existing = nil
	}
	if existing != nil && (existing.EnvironmentID != 0 || existing.Namespace != environment.Namespace) {
		apiShared.Conflict(c, "域名已被平台管理")
		return
	}
	domain := existing
	if domain == nil {
		domain = &model.ManagedDomain{Hostname: hostname}
	}
	domain.EnvironmentID, domain.Namespace = environment.ID, environment.Namespace
	domain.CertificateName, domain.TLSSecretName = certificate.Name, certificate.SecretName
	domain.IssuerRef, domain.IssuerKind, domain.CertificateOwnership = certificate.Issuer, issuerKind, "imported"
	domain.Description, domain.Enabled = strings.TrimSpace(req.Description), req.Enabled
	if h.cloud != nil {
		if err := h.prepareCloudDomain(c.Request.Context(), domain, environment, req.CloudConnectionID, req.DNSZone, req.NodeIDs, false); err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
		domain.OriginalDNSRecords = encodeDNSRecords(existingRecords)
		domain.DNSRecords = ""
		domain.AdoptionStatus, domain.DNSStatus = "staged", "staged"
	}
	if existing == nil {
		if err := h.network.CreateManagedDomain(domain); err != nil {
			apiShared.Conflict(c, "域名已被平台管理")
			return
		}
	} else if err := h.network.UpdateManagedDomain(domain); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, h.DomainInfo(c.Request.Context(), domain), "已导入证书；原 DNS 解析未变，请绑定应用入口后切换解析")
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
	if current.AdoptionStatus == "cutover_failed" {
		apiShared.Conflict(c, "解析切换未完成，请先恢复原解析")
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
	if current.EnvironmentID != 0 && current.EnvironmentID != environment.ID {
		previous, previousErr := h.domainEnvironment(current.EnvironmentID)
		if previousErr != nil || previous.ClusterID != environment.ClusterID {
			apiShared.Conflict(c, "域名不能直接迁移到其他集群")
			return
		}
	}
	h, err = h.forEnvironment(c.Request.Context(), environment)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	domain, err := domainFromRequest(req, current, environment)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if h.cloud != nil && current.CloudConnectionID != 0 {
		if err := h.prepareCloudDomain(c.Request.Context(), domain, environment, current.CloudConnectionID, current.DNSZone, req.NodeIDs, false); err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
		if domain.AdoptionStatus == "staged" || domain.AdoptionStatus == "restored" {
			domain.DNSStatus = domain.AdoptionStatus
		}
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
	if h.cloud != nil && domain.CloudConnectionID != 0 && domain.AdoptionStatus != "staged" && domain.AdoptionStatus != "restored" {
		if err := h.syncDNS(c.Request.Context(), domain); err != nil {
			apiShared.SuccessWithMessage(c, h.DomainInfo(c.Request.Context(), domain), "域名已更新，但 DNS 解析更新失败，可重试："+err.Error())
			return
		}
	}
	info := h.DomainInfo(c.Request.Context(), domain)
	if isImportedDomain(domain) {
		apiShared.SuccessWithMessage(c, info, "导入域名已更新，Certificate 保持原样")
		return
	}
	if err := h.ensureManagedDomainCertificate(c.Request.Context(), domain); err != nil {
		info.CertificateError = err.Error()
		apiShared.SuccessWithMessage(c, info, "域名已更新，但证书申请尚未成功，可重试")
		return
	}
	apiShared.SuccessWithMessage(c, h.DomainInfo(c.Request.Context(), domain), "受管域名已更新，正在同步证书")
}

func (h *DomainHandler) RetryCertificate(c *gin.Context) {
	domain, ok := h.managedDomain(c)
	if !ok {
		return
	}
	var err error
	h, err = h.forDomain(c.Request.Context(), domain)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
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
	apiShared.SuccessWithMessage(c, h.DomainInfo(c.Request.Context(), domain), "证书申请已重新提交")
}

func (h *DomainHandler) ListOperations(c *gin.Context) {
	domain, ok := h.managedDomain(c)
	if !ok {
		return
	}
	var err error
	h, err = h.forDomain(c.Request.Context(), domain)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if h.k8s == nil || domain.Namespace == "" || domain.CertificateName == "" {
		apiShared.ValidationError(c, "域名尚未绑定证书")
		return
	}
	operations, err := h.network.ListCertificateOperationsContext(c.Request.Context(), domain.Namespace, domain.CertificateName)
	if err != nil {
		apiShared.Error(c, http.StatusBadRequest, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	apiShared.Success(c, operations)
}

func (h *DomainHandler) Delete(c *gin.Context) {
	domain, ok := h.managedDomain(c)
	if !ok {
		return
	}
	if h.clusterClient != nil && domain.EnvironmentID == 0 {
		apiShared.ValidationError(c, "历史域名尚未关联集群环境，请先关联后删除")
		return
	}
	if domain.EnvironmentID != 0 {
		var scopeErr error
		h, scopeErr = h.forDomain(c.Request.Context(), domain)
		if scopeErr != nil {
			apiShared.ValidationError(c, scopeErr.Error())
			return
		}
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
	if domain.AdoptionStatus == "active" || domain.AdoptionStatus == "cutover_failed" {
		apiShared.Conflict(c, "请先恢复原 DNS 解析，再删除接管的域名")
		return
	}
	if !isImportedDomain(domain) && h.k8s != nil && domain.Namespace != "" && domain.CertificateName != "" {
		if err := h.network.DeleteCertificateContext(c.Request.Context(), domain.Namespace, domain.CertificateName); err != nil && !apierrors.IsNotFound(err) {
			apiShared.Error(c, http.StatusBadRequest, apiShared.CodeK8sAPIError, "删除域名证书: "+err.Error())
			return
		}
		if domain.TLSSecretName != "" {
			if err := h.k8s.DeleteTLSSecret(c.Request.Context(), domain.Namespace, domain.TLSSecretName); err != nil && !apierrors.IsNotFound(err) {
				apiShared.Error(c, http.StatusBadRequest, apiShared.CodeK8sAPIError, "删除域名 TLS Secret: "+err.Error())
				return
			}
		}
	}
	if h.cloud != nil && domain.CloudConnectionID != 0 && !isImportedDomain(domain) && domain.AdoptionStatus == "" {
		records := decodeDNSRecords(domain.DNSRecords)
		for len(records) > 0 {
			record := records[0]
			if err := h.cloud.DeleteRecord(c.Request.Context(), domain.CloudConnectionID, record.ID, true); err != nil {
				apiShared.ValidationError(c, "删除云 DNS 记录失败: "+err.Error())
				return
			}
			records = records[1:]
			encoded, _ := json.Marshal(records)
			domain.DNSRecords = string(encoded)
			if err := h.network.UpdateManagedDomain(domain); err != nil {
				apiShared.DBError(c, "保存 DNS 清理进度失败: "+err.Error())
				return
			}
		}
	}
	if err := h.network.DeleteManagedDomain(domain.ID); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	apiShared.Success(c, gin.H{"id": domain.ID})
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
	if h.clusterClient != nil {
		scoped, err := h.forDomain(ctx, domain)
		if err != nil {
			info := ManagedDomainInfoFor(ctx, domain, nil, h.network)
			info.CertificateError = err.Error()
			return info
		}
		h = scoped
	}
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
	view := ManagedDomainInfo{ID: domain.ID, Hostname: domain.Hostname, EnvironmentID: domain.EnvironmentID, Namespace: domain.Namespace, CertificateName: domain.CertificateName, TLSSecretName: domain.TLSSecretName, IssuerRef: domain.IssuerRef, IssuerKind: domain.IssuerKind, CertificateOwnership: domain.CertificateOwnership, CloudConnectionID: domain.CloudConnectionID, DNSZone: domain.DNSZone, DNSStatus: domain.DNSStatus, DNSError: domain.DNSError, AdoptionStatus: domain.AdoptionStatus, Description: domain.Description, Enabled: domain.Enabled, CreatedAt: domain.CreatedAt, UpdatedAt: domain.UpdatedAt}
	_ = json.Unmarshal([]byte(domain.DNSNodeIDs), &view.DNSNodeIDs)
	for _, record := range decodeDNSRecords(domain.DNSRecords) {
		view.DNSAddresses = append(view.DNSAddresses, record.Value)
	}
	for _, record := range decodeDNSRecords(domain.OriginalDNSRecords) {
		view.OriginalDNSAddresses = append(view.OriginalDNSAddresses, record.Value)
	}
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
