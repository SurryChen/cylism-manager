package applicationapi

import (
	"context"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// CertificateReader is the certificate capability used by application views.
type CertificateReader interface {
	GetCertificateContext(context.Context, string, string) (*k8sclient.CertInfo, error)
}

// NewNamespaceClient narrows a Kubernetes client to namespace lifecycle
// operations before it is injected into the application handler.
func NewNamespaceClient(client *k8sclient.Client) NamespaceClient {
	if client == nil {
		return nil
	}
	return client
}

// KubernetesAdapter is the smallest application-facing Kubernetes port.
// Concrete clients are assembled outside the handler and never exposed to it.
type KubernetesAdapter interface {
	applicationservice.ResourceApplier
	applicationservice.RuntimeReader
	NamespaceClient
	CertificateReader
	SyncApplicationEndpoints(context.Context, applicationservice.ApplicationContext, []model.ApplicationEndpoint, int32) error
	InspectReleasePods(context.Context, applicationservice.ApplicationContext) (*model.ReleaseRuntime, error)
	ValidatePersistentVolumeClaims(context.Context, applicationservice.ApplicationContext, applicationservice.ReleaseSpec) error
	MigrateWorkloadKind(context.Context, applicationservice.ApplicationContext, applicationservice.ReleaseSpec, string) error
	KubernetesAvailable() bool
}

type kubernetesDependencies struct {
	applier      applicationservice.ResourceApplier
	runtime      applicationservice.RuntimeReader
	namespaces   NamespaceClient
	certificates CertificateReader
	endpoints    interface {
		SyncApplicationEndpoints(context.Context, applicationservice.ApplicationContext, []model.ApplicationEndpoint, int32) error
	}
	inspector interface {
		InspectReleasePods(context.Context, applicationservice.ApplicationContext) (*model.ReleaseRuntime, error)
	}
	pvc interface {
		ValidatePersistentVolumeClaims(context.Context, applicationservice.ApplicationContext, applicationservice.ReleaseSpec) error
	}
	migrator interface {
		MigrateWorkloadKind(context.Context, applicationservice.ApplicationContext, applicationservice.ReleaseSpec, string) error
	}
	available bool
}

func (d *kubernetesDependencies) VerifyImage(ctx context.Context, spec applicationservice.ReleaseSpec) error {
	return d.applier.VerifyImage(ctx, spec)
}
func (d *kubernetesDependencies) Preflight(ctx context.Context, app applicationservice.ApplicationContext, spec applicationservice.ReleaseSpec) error {
	return d.applier.Preflight(ctx, app, spec)
}
func (d *kubernetesDependencies) Apply(ctx context.Context, resources *applicationservice.RenderedResources) error {
	return d.applier.Apply(ctx, resources)
}
func (d *kubernetesDependencies) WaitReady(ctx context.Context, app applicationservice.ApplicationContext, spec applicationservice.ReleaseSpec) error {
	return d.applier.WaitReady(ctx, app, spec)
}
func (d *kubernetesDependencies) ListRuntimeServices(ctx context.Context, namespace string) ([]corev1.Service, error) {
	return d.runtime.ListRuntimeServices(ctx, namespace)
}
func (d *kubernetesDependencies) ListRuntimeDeployments(ctx context.Context, namespace string) ([]appsv1.Deployment, error) {
	return d.runtime.ListRuntimeDeployments(ctx, namespace)
}
func (d *kubernetesDependencies) ListRuntimeStatefulSets(ctx context.Context, namespace string) ([]appsv1.StatefulSet, error) {
	return d.runtime.ListRuntimeStatefulSets(ctx, namespace)
}
func (d *kubernetesDependencies) ListRuntimePods(ctx context.Context, namespace, selector string) ([]corev1.Pod, error) {
	return d.runtime.ListRuntimePods(ctx, namespace, selector)
}
func (d *kubernetesDependencies) GetNamespace(ctx context.Context, name string) (*corev1.Namespace, error) {
	return d.namespaces.GetNamespace(ctx, name)
}
func (d *kubernetesDependencies) UpdateNamespace(ctx context.Context, ns *corev1.Namespace) (*corev1.Namespace, error) {
	return d.namespaces.UpdateNamespace(ctx, ns)
}
func (d *kubernetesDependencies) CreateNamespace(ctx context.Context, ns *corev1.Namespace) (*corev1.Namespace, error) {
	return d.namespaces.CreateNamespace(ctx, ns)
}
func (d *kubernetesDependencies) GetCertificateContext(ctx context.Context, namespace, name string) (*k8sclient.CertInfo, error) {
	return d.certificates.GetCertificateContext(ctx, namespace, name)
}
func (d *kubernetesDependencies) SyncApplicationEndpoints(ctx context.Context, app applicationservice.ApplicationContext, endpoints []model.ApplicationEndpoint, port int32) error {
	return d.endpoints.SyncApplicationEndpoints(ctx, app, endpoints, port)
}
func (d *kubernetesDependencies) InspectReleasePods(ctx context.Context, app applicationservice.ApplicationContext) (*model.ReleaseRuntime, error) {
	return d.inspector.InspectReleasePods(ctx, app)
}
func (d *kubernetesDependencies) ValidatePersistentVolumeClaims(ctx context.Context, app applicationservice.ApplicationContext, spec applicationservice.ReleaseSpec) error {
	return d.pvc.ValidatePersistentVolumeClaims(ctx, app, spec)
}
func (d *kubernetesDependencies) MigrateWorkloadKind(ctx context.Context, app applicationservice.ApplicationContext, spec applicationservice.ReleaseSpec, target string) error {
	return d.migrator.MigrateWorkloadKind(ctx, app, spec, target)
}
func (d *kubernetesDependencies) KubernetesAvailable() bool { return d.available }

// NewKubernetesAdapter is the only application adapter factory that
// accepts the concrete Kubernetes client. Handlers receive only the port.
func NewKubernetesAdapter(client *k8sclient.Client) KubernetesAdapter {
	if client == nil || client.Clientset == nil {
		return nil
	}
	applier := applicationservice.NewKubernetesApplier(client)
	return &kubernetesDependencies{
		applier: applier, runtime: client, namespaces: client, certificates: client,
		endpoints: applier, inspector: applier, pvc: applier, migrator: applier,
		available: true,
	}
}
