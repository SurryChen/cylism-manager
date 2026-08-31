// Package network contains reusable infrastructure network workflows.
package network

import (
	"errors"
	"strings"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
)

// DomainStore is the persistence boundary used by domain lifecycle workflows.
type DomainStore interface {
	GetEnvironmentByID(id uint) (*model.Environment, error)
	GetManagedDomain(id uint) (*model.ManagedDomain, error)
	GetManagedDomainByHostname(hostname string) (*model.ManagedDomain, error)
	ListManagedDomains(environmentID ...uint) ([]model.ManagedDomain, error)
	ListUnassignedManagedDomains() ([]model.ManagedDomain, error)
	ListClaimableManagedDomains(namespace string) ([]model.ManagedDomain, error)
	CreateManagedDomain(domain *model.ManagedDomain) error
	UpdateManagedDomain(domain *model.ManagedDomain) error
	DeleteManagedDomain(id uint) error
	CountApplicationEndpointsByDomain(id uint) (int64, error)
}

// DNSCredentialStore is intentionally separate from domain state: credential
// values have a different lifecycle and should not require a concrete Store.
type DNSCredentialStore interface {
	ListDNSCredentials() ([]model.DNSCredential, error)
	GetDNSCredential(uint) (*model.DNSCredential, error)
	CreateDNSCredential(*model.DNSCredential) error
	UpdateDNSCredential(*model.DNSCredential) error
	DeleteDNSCredential(uint) error
}

// DomainInput is the transport-neutral input for a managed domain.
type DomainInput struct {
	Hostname      string
	EnvironmentID uint
	IssuerRef     string
	Description   string
	Enabled       bool
}

// ManagedDomainView is the transport-neutral aggregate used by domain HTTP
// and background callers. Certificate lookup failures are represented as a
// message so a stale cert-manager resource does not hide the domain record.
type ManagedDomainView struct {
	Domain           model.ManagedDomain
	Certificate      *k8sclient.CertInfo
	CertificateError string
	ApplicationCount int64
}

type DomainPrerequisiteAdapter struct {
	NamespaceExists func(string) (bool, error)
	ListIssuers     func() ([]Issuer, error)
}

type Issuer struct {
	Name, Kind string
	Ready      bool
}

type Service struct {
	domains         DomainStore
	credentials     DNSCredentialStore
	ingress         IngressAdapter
	dns             DNSAdapter
	cert            CertificateAdapter
	standardIngress StandardIngressAdapter
}

// IngressAdapter is the Kubernetes boundary for Traefik IngressRoute
// inventory and deletion. Resource rendering stays in the adapter so the
// service can be reused by HTTP and background callers.
type IngressAdapter interface {
	ListIngressRoutes() ([]k8sclient.IngressRouteInfo, error)
	DeleteIngressRoute(namespace, name string) error
}

// DNSAdapter is the Kubernetes boundary for DNS webhook installation and
// credential Secret synchronization.
type DNSAdapter interface {
	DNSProviderStatus(provider string) *k8sclient.DNSProviderStatus
	InstallDNSProvider(provider string) (*k8sclient.DNSProviderStatus, error)
	UpsertDNSCredentialSecret(provider, namespace, secretName string, values map[string]string) error
	DeleteDNSCredentialSecret(namespace, secretName string) error
}

// CertificateAdapter is the Kubernetes boundary for cert-manager resources.
type CertificateAdapter interface {
	CertManagerStatus() *k8sclient.CertManagerStatus
	ListCertificates() ([]k8sclient.CertInfo, error)
	GetCertificate(namespace, name string) (*k8sclient.CertInfo, error)
	EnsureCertificate(k8sclient.CreateCertificateRequest) (*k8sclient.CertInfo, error)
	ListIssuers() ([]k8sclient.IssuerInfo, error)
	CreateIssuer(k8sclient.IssuerRequest) (*k8sclient.IssuerInfo, error)
	UpdateIssuer(k8sclient.IssuerRequest) (*k8sclient.IssuerInfo, error)
	DeleteIssuer(kind, namespace, name string) error
	ListCertificateOperations(namespace, name string) ([]k8sclient.CertificateOperation, error)
	CreateCertificate(k8sclient.CreateCertificateRequest) (*k8sclient.CertInfo, error)
	DeleteCertificate(namespace, name string) error
}

type StandardIngressAdapter interface {
	ListIngresses(namespace string) ([]k8sclient.IngressStdInfo, error)
	GetIngress(namespace, name string) (*k8sclient.IngressStdDetail, error)
	CreateIngress(namespace, name, host, path, serviceName, servicePort string) (*k8sclient.IngressStdDetail, error)
	DeleteIngress(namespace, name string) error
	DetectIngressController() (*k8sclient.IngressControllerStatus, error)
}

func NewService(domains DomainStore, credentials ...DNSCredentialStore) *Service {
	service := &Service{domains: domains}
	if len(credentials) > 0 {
		service.credentials = credentials[0]
	}
	return service
}

func (s *Service) WithIngressAdapter(adapter IngressAdapter) *Service {
	s.ingress = adapter
	return s
}

func (s *Service) WithDNSAdapter(adapter DNSAdapter) *Service {
	s.dns = adapter
	return s
}

func (s *Service) WithCertificateAdapter(adapter CertificateAdapter) *Service {
	s.cert = adapter
	return s
}

func (s *Service) WithStandardIngressAdapter(adapter StandardIngressAdapter) *Service {
	s.standardIngress = adapter
	return s
}

func (s *Service) ListStandardIngresses(namespace string) ([]k8sclient.IngressStdInfo, error) {
	if s.standardIngress == nil {
		return nil, errors.New("标准 Ingress 适配器未初始化")
	}
	return s.standardIngress.ListIngresses(strings.TrimSpace(namespace))
}
func (s *Service) GetStandardIngress(namespace, name string) (*k8sclient.IngressStdDetail, error) {
	if s.standardIngress == nil {
		return nil, errors.New("标准 Ingress 适配器未初始化")
	}
	return s.standardIngress.GetIngress(strings.TrimSpace(namespace), strings.TrimSpace(name))
}
func (s *Service) CreateStandardIngress(namespace, name, host, ingressPath, serviceName, servicePort string) (*k8sclient.IngressStdDetail, error) {
	if s.standardIngress == nil {
		return nil, errors.New("标准 Ingress 适配器未初始化")
	}
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(host) == "" || strings.TrimSpace(serviceName) == "" || strings.TrimSpace(servicePort) == "" {
		return nil, errors.New("namespace, name, host, service_name, service_port 必填")
	}
	if strings.TrimSpace(ingressPath) == "" {
		ingressPath = "/"
	}
	return s.standardIngress.CreateIngress(strings.TrimSpace(namespace), strings.TrimSpace(name), strings.TrimSpace(host), ingressPath, strings.TrimSpace(serviceName), strings.TrimSpace(servicePort))
}
func (s *Service) DeleteStandardIngress(namespace, name string) error {
	if s.standardIngress == nil {
		return errors.New("标准 Ingress 适配器未初始化")
	}
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(name) == "" {
		return errors.New("Ingress 命名空间和名称必填")
	}
	return s.standardIngress.DeleteIngress(strings.TrimSpace(namespace), strings.TrimSpace(name))
}
func (s *Service) DetectIngressController() (*k8sclient.IngressControllerStatus, error) {
	if s.standardIngress == nil {
		return nil, errors.New("标准 Ingress 适配器未初始化")
	}
	return s.standardIngress.DetectIngressController()
}

func (s *Service) CertificateManagerStatus() (*k8sclient.CertManagerStatus, error) {
	if s.cert == nil {
		return nil, errors.New("证书管理适配器未初始化")
	}
	return s.cert.CertManagerStatus(), nil
}
func (s *Service) ListCertificates() ([]k8sclient.CertInfo, error) {
	if s.cert == nil {
		return nil, errors.New("证书管理适配器未初始化")
	}
	return s.cert.ListCertificates()
}
func (s *Service) GetCertificate(namespace, name string) (*k8sclient.CertInfo, error) {
	if s.cert == nil {
		return nil, errors.New("证书管理适配器未初始化")
	}
	return s.cert.GetCertificate(namespace, name)
}
func (s *Service) EnsureCertificate(req k8sclient.CreateCertificateRequest) (*k8sclient.CertInfo, error) {
	if s.cert == nil {
		return nil, errors.New("证书管理适配器未初始化")
	}
	return s.cert.EnsureCertificate(req)
}
func (s *Service) ListIssuers() ([]k8sclient.IssuerInfo, error) {
	if s.cert == nil {
		return nil, errors.New("证书管理适配器未初始化")
	}
	return s.cert.ListIssuers()
}
func (s *Service) CreateIssuer(req k8sclient.IssuerRequest) (*k8sclient.IssuerInfo, error) {
	if s.cert == nil {
		return nil, errors.New("证书管理适配器未初始化")
	}
	return s.cert.CreateIssuer(req)
}
func (s *Service) UpdateIssuer(req k8sclient.IssuerRequest) (*k8sclient.IssuerInfo, error) {
	if s.cert == nil {
		return nil, errors.New("证书管理适配器未初始化")
	}
	return s.cert.UpdateIssuer(req)
}
func (s *Service) DeleteIssuer(kind, namespace, name string) error {
	if s.cert == nil {
		return errors.New("证书管理适配器未初始化")
	}
	return s.cert.DeleteIssuer(kind, namespace, name)
}
func (s *Service) ListCertificateOperations(namespace, name string) ([]k8sclient.CertificateOperation, error) {
	if s.cert == nil {
		return nil, errors.New("证书管理适配器未初始化")
	}
	return s.cert.ListCertificateOperations(namespace, name)
}
func (s *Service) CreateCertificate(req k8sclient.CreateCertificateRequest) (*k8sclient.CertInfo, error) {
	if s.cert == nil {
		return nil, errors.New("证书管理适配器未初始化")
	}
	return s.cert.CreateCertificate(req)
}
func (s *Service) DeleteCertificate(namespace, name string) error {
	if s.cert == nil {
		return errors.New("证书管理适配器未初始化")
	}
	return s.cert.DeleteCertificate(namespace, name)
}

func (s *Service) DNSProviderStatus(provider string) (*k8sclient.DNSProviderStatus, error) {
	if s.dns == nil {
		return nil, errors.New("DNS Provider 适配器未初始化")
	}
	return s.dns.DNSProviderStatus(strings.TrimSpace(provider)), nil
}

func (s *Service) InstallDNSProvider(provider string) (*k8sclient.DNSProviderStatus, error) {
	if s.dns == nil {
		return nil, errors.New("DNS Provider 适配器未初始化")
	}
	return s.dns.InstallDNSProvider(strings.TrimSpace(provider))
}

func (s *Service) SyncDNSCredentialSecret(credential *model.DNSCredential, values map[string]string) error {
	if s.dns == nil {
		return errors.New("DNS Provider 适配器未初始化")
	}
	if credential == nil || strings.TrimSpace(credential.Provider) == "" || strings.TrimSpace(credential.Namespace) == "" || strings.TrimSpace(credential.SecretName) == "" {
		return errors.New("DNS 凭据定义不完整")
	}
	return s.dns.UpsertDNSCredentialSecret(credential.Provider, credential.Namespace, credential.SecretName, values)
}

func (s *Service) RemoveDNSCredentialSecret(namespace, secretName string) error {
	if s.dns == nil {
		return errors.New("DNS Provider 适配器未初始化")
	}
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(secretName) == "" {
		return errors.New("DNS Secret 名称和命名空间必填")
	}
	return s.dns.DeleteDNSCredentialSecret(strings.TrimSpace(namespace), strings.TrimSpace(secretName))
}

func (s *Service) ListIngressRoutes() ([]k8sclient.IngressRouteInfo, error) {
	if s.ingress == nil {
		return nil, errors.New("IngressRoute 适配器未初始化")
	}
	return s.ingress.ListIngressRoutes()
}

func (s *Service) DeleteIngressRoute(namespace, name string) error {
	if s.ingress == nil {
		return errors.New("IngressRoute 适配器未初始化")
	}
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(name) == "" {
		return errors.New("IngressRoute 命名空间和名称必填")
	}
	return s.ingress.DeleteIngressRoute(strings.TrimSpace(namespace), strings.TrimSpace(name))
}

func (s *Service) requireCredentials() (DNSCredentialStore, error) {
	if s.credentials == nil {
		return nil, errors.New("DNS 凭据存储未初始化")
	}
	return s.credentials, nil
}

func (s *Service) ListDNSCredentials() ([]model.DNSCredential, error) {
	records, err := s.requireCredentials()
	if err != nil {
		return nil, err
	}
	return records.ListDNSCredentials()
}
func (s *Service) GetDNSCredential(id uint) (*model.DNSCredential, error) {
	records, err := s.requireCredentials()
	if err != nil {
		return nil, err
	}
	return records.GetDNSCredential(id)
}
func (s *Service) CreateDNSCredential(credential *model.DNSCredential) error {
	records, err := s.requireCredentials()
	if err != nil {
		return err
	}
	return records.CreateDNSCredential(credential)
}
func (s *Service) UpdateDNSCredential(credential *model.DNSCredential) error {
	records, err := s.requireCredentials()
	if err != nil {
		return err
	}
	return records.UpdateDNSCredential(credential)
}
func (s *Service) DeleteDNSCredential(id uint) error {
	records, err := s.requireCredentials()
	if err != nil {
		return err
	}
	return records.DeleteDNSCredential(id)
}

// ResolveEnvironment verifies that a network resource is scoped to a known
// application environment and returns the namespace used for Kubernetes calls.
func (s *Service) ResolveEnvironment(environmentID uint) (*model.Environment, error) {
	if environmentID == 0 {
		return nil, errors.New("环境 ID 必填且必须有效")
	}
	if s.domains == nil {
		return nil, errors.New("数据存储未初始化")
	}
	env, err := s.domains.GetEnvironmentByID(environmentID)
	if err != nil {
		return nil, errors.New("环境不存在")
	}
	if env.NamespaceConflict {
		return nil, errors.New("环境命名空间存在冲突，请先完成迁移")
	}
	// Store implementations that expose conflict details get an additional
	// defensive check. The optional shape avoids forcing every test double to
	// implement a persistence-only method.
	if conflicts, ok := any(s.domains).(interface{ IsEnvironmentNamespaceConflicted(string) (bool, error) }); ok {
		conflicted, conflictErr := conflicts.IsEnvironmentNamespaceConflicted(env.Namespace)
		if conflictErr != nil {
			return nil, errors.New("检查命名空间归属失败")
		}
		if conflicted {
			return nil, errors.New("环境命名空间存在冲突，请先完成迁移")
		}
	}
	return env, nil
}

// Domain persistence methods keep HTTP and background callers on the same
// storage boundary. They deliberately return store errors unchanged so the
// transport layer can preserve its existing status and message mapping.
func (s *Service) GetManagedDomain(id uint) (*model.ManagedDomain, error) {
	if s.domains == nil {
		return nil, errors.New("数据存储未初始化")
	}
	return s.domains.GetManagedDomain(id)
}

func (s *Service) GetManagedDomainByHostname(hostname string) (*model.ManagedDomain, error) {
	if s.domains == nil {
		return nil, errors.New("数据存储未初始化")
	}
	return s.domains.GetManagedDomainByHostname(strings.TrimSpace(hostname))
}

func (s *Service) ListManagedDomains(environmentID ...uint) ([]model.ManagedDomain, error) {
	if s.domains == nil {
		return nil, errors.New("数据存储未初始化")
	}
	return s.domains.ListManagedDomains(environmentID...)
}

func (s *Service) ListUnassignedManagedDomains() ([]model.ManagedDomain, error) {
	if s.domains == nil {
		return nil, errors.New("数据存储未初始化")
	}
	return s.domains.ListUnassignedManagedDomains()
}

func (s *Service) ListClaimableManagedDomains(namespace string) ([]model.ManagedDomain, error) {
	if s.domains == nil {
		return nil, errors.New("数据存储未初始化")
	}
	return s.domains.ListClaimableManagedDomains(strings.TrimSpace(namespace))
}

func (s *Service) CreateManagedDomain(domain *model.ManagedDomain) error {
	if s.domains == nil {
		return errors.New("数据存储未初始化")
	}
	return s.domains.CreateManagedDomain(domain)
}

func (s *Service) UpdateManagedDomain(domain *model.ManagedDomain) error {
	if s.domains == nil {
		return errors.New("数据存储未初始化")
	}
	return s.domains.UpdateManagedDomain(domain)
}

func (s *Service) DeleteManagedDomain(id uint) error {
	if s.domains == nil {
		return errors.New("数据存储未初始化")
	}
	return s.domains.DeleteManagedDomain(id)
}

func (s *Service) CountApplicationEndpointsByDomain(id uint) (int64, error) {
	if s.domains == nil {
		return 0, errors.New("数据存储未初始化")
	}
	return s.domains.CountApplicationEndpointsByDomain(id)
}

func (s *Service) BuildManagedDomainView(domain *model.ManagedDomain) ManagedDomainView {
	view := ManagedDomainView{}
	if domain == nil {
		return view
	}
	view.Domain = *domain
	if count, err := s.CountApplicationEndpointsByDomain(domain.ID); err == nil {
		view.ApplicationCount = count
	}
	if s.cert == nil || strings.TrimSpace(domain.Namespace) == "" || strings.TrimSpace(domain.CertificateName) == "" {
		if strings.TrimSpace(domain.Namespace) == "" {
			view.CertificateError = "域名尚未绑定命名空间，请编辑后申请证书"
		}
		return view
	}
	certificate, err := s.GetCertificate(domain.Namespace, domain.CertificateName)
	if err != nil {
		view.CertificateError = err.Error()
		return view
	}
	view.Certificate = certificate
	return view
}

// BuildManagedDomain applies the shared normalization and immutable binding
// rules used by create and update endpoints.
func (s *Service) BuildManagedDomain(input DomainInput, current *model.ManagedDomain, environment *model.Environment) (*model.ManagedDomain, error) {
	hostname := strings.ToLower(strings.TrimSpace(input.Hostname))
	if err := ValidateHostname(hostname); err != nil || strings.HasPrefix(hostname, "*.") {
		return nil, errors.New("域名必须是合法的精确 DNS 名称，且不支持泛域名")
	}
	if environment == nil || environment.ID == 0 || strings.TrimSpace(environment.Namespace) == "" {
		return nil, errors.New("请选择域名所属环境")
	}
	issuer := strings.TrimSpace(input.IssuerRef)
	if issuer == "" {
		return nil, errors.New("请选择已就绪的 ClusterIssuer")
	}
	if current != nil && current.EnvironmentID != 0 && current.EnvironmentID != environment.ID {
		return nil, errors.New("域名已绑定环境，不能直接修改")
	}
	domain := &model.ManagedDomain{
		Hostname: hostname, EnvironmentID: environment.ID, Namespace: environment.Namespace,
		CertificateName: currentCertificateName(current), TLSSecretName: currentTLSSecretName(current),
		IssuerRef: issuer, IssuerKind: "ClusterIssuer", CertificateOwnership: "managed",
		Description: strings.TrimSpace(input.Description), Enabled: input.Enabled,
	}
	if current != nil {
		domain.ID, domain.CreatedAt, domain.CertificateOwnership = current.ID, current.CreatedAt, current.CertificateOwnership
		if current.CertificateOwnership == "imported" {
			domain.IssuerRef, domain.IssuerKind = current.IssuerRef, current.IssuerKind
		}
	}
	return domain, nil
}

func (s *Service) ValidateManagedDomainPrerequisites(domain *model.ManagedDomain, adapter DomainPrerequisiteAdapter) error {
	if domain == nil || strings.TrimSpace(domain.Namespace) == "" {
		return errors.New("域名命名空间不能为空")
	}
	if adapter.NamespaceExists == nil || adapter.ListIssuers == nil {
		return errors.New("网络资源检查器未初始化")
	}
	exists, err := adapter.NamespaceExists(domain.Namespace)
	if err != nil {
		return errors.New("检查命名空间: " + err.Error())
	}
	if !exists {
		return errors.New("命名空间 \"" + domain.Namespace + "\" 不存在")
	}
	issuers, err := adapter.ListIssuers()
	if err != nil {
		return errors.New("读取签发者: " + err.Error())
	}
	for _, issuer := range issuers {
		if issuer.Kind == "ClusterIssuer" && issuer.Name == domain.IssuerRef && issuer.Ready {
			return nil
		}
	}
	return errors.New("ClusterIssuer \"" + domain.IssuerRef + "\" 不存在或尚未就绪")
}

// ValidateDomainDeletion keeps reference protection consistent across all
// entry points that can remove a managed domain.
func ValidateDomainDeletion(applicationCount int64) error {
	if applicationCount > 0 {
		return errors.New("域名仍被应用入口引用，请先发布不使用该域名的新版本")
	}
	return nil
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

// ValidateHostname performs the service-level validation shared by managed
// domain creation and update. Detailed certificate checks remain in the K8s adapter.
func ValidateHostname(hostname string) error {
	hostname = strings.TrimSpace(strings.ToLower(hostname))
	if len(hostname) < 3 || len(hostname) > 253 || !strings.Contains(hostname, ".") {
		return errors.New("域名不能为空")
	}
	for _, part := range strings.Split(hostname, ".") {
		if len(part) == 0 || len(part) > 63 || strings.HasPrefix(part, "-") || strings.HasSuffix(part, "-") {
			return errors.New("域名格式无效")
		}
		for _, char := range part {
			if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-') {
				return errors.New("域名格式无效")
			}
		}
	}
	return nil
}
