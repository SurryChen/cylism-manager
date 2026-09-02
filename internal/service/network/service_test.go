package network

import (
	"context"
	"errors"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
)

type fakeIngress struct{ deleted string }

func (f *fakeIngress) ListIngressRoutes() ([]k8sclient.IngressRouteInfo, error) {
	return []k8sclient.IngressRouteInfo{{Name: "route", Namespace: "default"}}, nil
}
func (f *fakeIngress) DeleteIngressRoute(namespace, name string) error {
	f.deleted = namespace + "/" + name
	return nil
}
func (f *fakeIngress) ListIngressRoutesContext(ctx context.Context) ([]k8sclient.IngressRouteInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.ListIngressRoutes()
}
func (f *fakeIngress) DeleteIngressRouteContext(ctx context.Context, ns, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return f.DeleteIngressRoute(ns, name)
}

type fakeStandardIngress struct{ deleted string }

func (f *fakeStandardIngress) ListIngresses(string) ([]k8sclient.IngressStdInfo, error) {
	return []k8sclient.IngressStdInfo{{Name: "web", Namespace: "default"}}, nil
}
func (f *fakeStandardIngress) GetIngress(namespace, name string) (*k8sclient.IngressStdDetail, error) {
	return &k8sclient.IngressStdDetail{Name: name, Namespace: namespace}, nil
}
func (f *fakeStandardIngress) CreateIngress(namespace, name, host, path, serviceName, servicePort string) (*k8sclient.IngressStdDetail, error) {
	return &k8sclient.IngressStdDetail{Name: name, Namespace: namespace}, nil
}
func (f *fakeStandardIngress) DeleteIngress(namespace, name string) error {
	f.deleted = namespace + "/" + name
	return nil
}
func (f *fakeStandardIngress) DetectIngressController() (*k8sclient.IngressControllerStatus, error) {
	return &k8sclient.IngressControllerStatus{Type: "Traefik", Running: true}, nil
}
func (f *fakeStandardIngress) ListIngressesContext(ctx context.Context, ns string) ([]k8sclient.IngressStdInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.ListIngresses(ns)
}
func (f *fakeStandardIngress) GetIngressContext(ctx context.Context, ns, name string) (*k8sclient.IngressStdDetail, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.GetIngress(ns, name)
}
func (f *fakeStandardIngress) CreateIngressContext(ctx context.Context, ns, name, host, path, svc, port string) (*k8sclient.IngressStdDetail, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.CreateIngress(ns, name, host, path, svc, port)
}
func (f *fakeStandardIngress) DeleteIngressContext(ctx context.Context, ns, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return f.DeleteIngress(ns, name)
}
func (f *fakeStandardIngress) DetectIngressControllerContext(ctx context.Context) (*k8sclient.IngressControllerStatus, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.DetectIngressController()
}

type fakeCertificates struct{ deleted string }

func (f *fakeCertificates) CertManagerStatusContext(ctx context.Context) *k8sclient.CertManagerStatus {
	if err := ctx.Err(); err != nil {
		return &k8sclient.CertManagerStatus{State: k8sclient.CertManagerStateUnavailable, Message: err.Error()}
	}
	return &k8sclient.CertManagerStatus{}
}
func (f *fakeCertificates) ListCertificatesContext(ctx context.Context) ([]k8sclient.CertInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []k8sclient.CertInfo{{Name: "cert"}}, nil
}
func (f *fakeCertificates) GetCertificateContext(ctx context.Context, ns, name string) (*k8sclient.CertInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &k8sclient.CertInfo{Name: "cert"}, nil
}
func (f *fakeCertificates) EnsureCertificateContext(ctx context.Context, req k8sclient.CreateCertificateRequest) (*k8sclient.CertInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &k8sclient.CertInfo{Name: "cert"}, nil
}
func (f *fakeCertificates) ListIssuersContext(ctx context.Context) ([]k8sclient.IssuerInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []k8sclient.IssuerInfo{{Name: "issuer"}}, nil
}
func (f *fakeCertificates) CreateIssuerContext(ctx context.Context, req k8sclient.IssuerRequest) (*k8sclient.IssuerInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &k8sclient.IssuerInfo{Name: "issuer"}, nil
}
func (f *fakeCertificates) UpdateIssuerContext(ctx context.Context, req k8sclient.IssuerRequest) (*k8sclient.IssuerInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &k8sclient.IssuerInfo{Name: "issuer"}, nil
}
func (f *fakeCertificates) DeleteIssuerContext(ctx context.Context, kind, ns, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}
func (f *fakeCertificates) ListCertificateOperationsContext(ctx context.Context, ns, name string) ([]k8sclient.CertificateOperation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, nil
}
func (f *fakeCertificates) CreateCertificateContext(ctx context.Context, req k8sclient.CreateCertificateRequest) (*k8sclient.CertInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &k8sclient.CertInfo{Name: "cert"}, nil
}
func (f *fakeCertificates) DeleteCertificateContext(ctx context.Context, ns, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.deleted = ns + "/" + name
	return nil
}

type fakeDomainStore struct{ environment *model.Environment }

func (f fakeDomainStore) GetManagedDomain(uint) (*model.ManagedDomain, error) {
	return nil, errors.New("missing")
}
func (f fakeDomainStore) GetManagedDomainByHostname(string) (*model.ManagedDomain, error) {
	return nil, errors.New("missing")
}
func (f fakeDomainStore) ListUnassignedManagedDomains() ([]model.ManagedDomain, error) {
	return nil, nil
}
func (f fakeDomainStore) ListClaimableManagedDomains(string) ([]model.ManagedDomain, error) {
	return nil, nil
}
func (f fakeDomainStore) CountApplicationEndpointsByDomain(uint) (int64, error) { return 0, nil }

func (f fakeDomainStore) GetEnvironmentByID(uint) (*model.Environment, error) {
	if f.environment == nil {
		return nil, errors.New("missing")
	}
	return f.environment, nil
}
func (fakeDomainStore) ListManagedDomains(...uint) ([]model.ManagedDomain, error) { return nil, nil }
func (fakeDomainStore) CreateManagedDomain(*model.ManagedDomain) error            { return nil }
func (fakeDomainStore) UpdateManagedDomain(*model.ManagedDomain) error            { return nil }
func (fakeDomainStore) DeleteManagedDomain(uint) error                            { return nil }

func TestResolveEnvironment(t *testing.T) {
	env, err := NewService(fakeDomainStore{environment: &model.Environment{Namespace: "app"}}).ResolveEnvironment(1)
	if err != nil || env.Namespace != "app" {
		t.Fatalf("unexpected environment: %#v, %v", env, err)
	}
}

func TestDomainPersistenceUsesServiceBoundary(t *testing.T) {
	service := NewService(fakeDomainStore{environment: &model.Environment{Namespace: "app"}})
	if _, err := service.ListManagedDomains(1); err != nil {
		t.Fatalf("list domains: %v", err)
	}
	if _, err := service.GetManagedDomain(1); err == nil {
		t.Fatal("expected fake lookup error")
	}
	if count, err := service.CountApplicationEndpointsByDomain(1); err != nil || count != 0 {
		t.Fatalf("domain references = %d, %v", count, err)
	}
}

func TestBuildManagedDomainViewAggregatesCertificateAndReferences(t *testing.T) {
	s := NewService(fakeDomainStore{environment: &model.Environment{Namespace: "app"}}).WithCertificateAdapter(&fakeCertificates{})
	view := s.BuildManagedDomainView(context.Background(), &model.ManagedDomain{ID: 7, Namespace: "app", CertificateName: "cert"})
	if view.Certificate == nil || view.Certificate.Name != "cert" {
		t.Fatalf("certificate view = %#v", view.Certificate)
	}
}

func TestValidateHostname(t *testing.T) {
	if err := ValidateHostname("api.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateHostname("https://api.example.com/path"); err == nil {
		t.Fatal("expected invalid hostname")
	}
}

func TestBuildManagedDomainNormalizesAndPreservesImportedCertificate(t *testing.T) {
	s := NewService(nil)
	env := &model.Environment{ID: 7, Namespace: "production"}
	managed, err := s.BuildManagedDomain(DomainInput{Hostname: "API.Example.com", EnvironmentID: 7, IssuerRef: "prod"}, nil, env)
	if err != nil || managed.Hostname != "api.example.com" || managed.IssuerKind != "ClusterIssuer" {
		t.Fatalf("unexpected managed domain: %#v, %v", managed, err)
	}
	current := &model.ManagedDomain{ID: 9, EnvironmentID: 7, Namespace: "production", CertificateOwnership: "imported", CertificateName: "legacy", TLSSecretName: "legacy-tls", IssuerRef: "legacy-issuer", IssuerKind: "Issuer"}
	imported, err := s.BuildManagedDomain(DomainInput{Hostname: "api.example.com", EnvironmentID: 7, IssuerRef: "ignored"}, current, env)
	if err != nil || imported.CertificateOwnership != "imported" || imported.IssuerRef != "legacy-issuer" || imported.IssuerKind != "Issuer" {
		t.Fatalf("imported certificate ownership changed: %#v, %v", imported, err)
	}
}

func TestValidateManagedDomainPrerequisites(t *testing.T) {
	s := NewService(nil)
	domain := &model.ManagedDomain{Namespace: "production", IssuerRef: "prod"}
	adapter := DomainPrerequisiteAdapter{
		NamespaceExists: func(context.Context, string) (bool, error) { return true, nil },
		ListIssuers: func(context.Context) ([]Issuer, error) {
			return []Issuer{{Name: "prod", Kind: "ClusterIssuer", Ready: true}}, nil
		},
	}
	if err := s.ValidateManagedDomainPrerequisites(context.Background(), domain, adapter); err != nil {
		t.Fatal(err)
	}
	adapter.ListIssuers = func(context.Context) ([]Issuer, error) {
		return []Issuer{{Name: "prod", Kind: "ClusterIssuer", Ready: false}}, nil
	}
	if err := s.ValidateManagedDomainPrerequisites(context.Background(), domain, adapter); err == nil {
		t.Fatal("expected unready issuer to be rejected")
	}
}

func TestIngressWorkflowUsesAdapterAndValidatesIdentity(t *testing.T) {
	fake := &fakeIngress{}
	s := NewService(nil).WithIngressAdapter(fake)
	routes, err := s.ListIngressRoutesContext(context.Background())
	if err != nil || len(routes) != 1 {
		t.Fatalf("unexpected routes: %#v, %v", routes, err)
	}
	if err := s.DeleteIngressRouteContext(context.Background(), "default", "route"); err != nil || fake.deleted != "default/route" {
		t.Fatalf("delete = %q, %v", fake.deleted, err)
	}
	if err := s.DeleteIngressRouteContext(context.Background(), "", "route"); err == nil {
		t.Fatal("expected missing namespace validation")
	}
}

func TestStandardIngressWorkflowValidatesAndUsesAdapter(t *testing.T) {
	fake := &fakeStandardIngress{}
	s := NewService(nil).WithStandardIngressAdapter(fake)
	if _, err := s.CreateStandardIngressContext(context.Background(), "", "web", "example.com", "/", "svc", "80"); err == nil {
		t.Fatal("expected required field validation")
	}
	detail, err := s.CreateStandardIngressContext(context.Background(), "default", "web", "example.com", "", "svc", "80")
	if err != nil || detail.Name != "web" {
		t.Fatalf("create standard ingress = %#v, %v", detail, err)
	}
	if err := s.DeleteStandardIngressContext(context.Background(), "default", "web"); err != nil || fake.deleted != "default/web" {
		t.Fatalf("delete standard ingress = %q, %v", fake.deleted, err)
	}
	status, err := s.DetectIngressControllerContext(context.Background())
	if err != nil || !status.Running {
		t.Fatalf("controller status = %#v, %v", status, err)
	}
}

func TestCertificateWorkflowUsesAdapter(t *testing.T) {
	fake := &fakeCertificates{}
	s := NewService(nil).WithCertificateAdapter(fake)
	certs, err := s.ListCertificatesContext(context.Background())
	if err != nil || len(certs) != 1 {
		t.Fatalf("unexpected certificates: %#v, %v", certs, err)
	}
	if err := s.DeleteCertificateContext(context.Background(), "default", "cert"); err != nil || fake.deleted != "default/cert" {
		t.Fatalf("delete = %q, %v", fake.deleted, err)
	}
}
