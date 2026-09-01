package delivery

import (
	"context"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
)

type countingManagedRegistryResources struct{ applied, deleted int }

func (f *countingManagedRegistryResources) EnsureDataNode(context.Context, string) error { return nil }
func (f *countingManagedRegistryResources) EnsureStorageClass(context.Context, string) error {
	return nil
}
func (f *countingManagedRegistryResources) ResolvePVC(context.Context, *model.ManagedOCIRegistry) error {
	return nil
}
func (f *countingManagedRegistryResources) EnsureTLSCertificate(context.Context, *model.ManagedOCIRegistry) error {
	return nil
}
func (f *countingManagedRegistryResources) EnsureResourcesAvailable(context.Context, string, string) error {
	return nil
}
func (f *countingManagedRegistryResources) Apply(context.Context, *model.ManagedOCIRegistry, string, string) error {
	f.applied++
	return nil
}
func (f *countingManagedRegistryResources) DeleteResources(context.Context, *model.ManagedOCIRegistry) {
	f.deleted++
}

func TestManagedRegistryHandlerDelegatesApplyAndDeleteToResourceBoundary(t *testing.T) {
	fake := &countingManagedRegistryResources{}
	h := (&ManagedOCIRegistryHandler{}).WithResourceReconciler(fake)
	if err := h.applyResources(context.Background(), &model.ManagedOCIRegistry{}, "registry.test", "secret"); err != nil {
		t.Fatal(err)
	}
	h.resources.DeleteResources(context.Background(), &model.ManagedOCIRegistry{})
	if fake.applied != 1 || fake.deleted != 1 {
		t.Fatalf("resource calls apply=%d delete=%d", fake.applied, fake.deleted)
	}
}

type fakeManagedRegistryReconciler struct{}

func (fakeManagedRegistryReconciler) Available() bool { return true }
func (fakeManagedRegistryReconciler) StoragePreflight(context.Context, string) ([]string, error) {
	return []string{"node-a"}, nil
}
func (fakeManagedRegistryReconciler) ListReadyDataNodes(context.Context) ([]string, error) {
	return []string{"node-a"}, nil
}
func (fakeManagedRegistryReconciler) EnsureDataNode(context.Context, string) error     { return nil }
func (fakeManagedRegistryReconciler) EnsureStorageClass(context.Context, string) error { return nil }
func (fakeManagedRegistryReconciler) ResolvePVC(context.Context, *model.ManagedOCIRegistry) error {
	return nil
}
func (fakeManagedRegistryReconciler) ListEligiblePVCs(context.Context, string) ([]k8s.ManagedRegistryPVCOption, error) {
	return nil, nil
}
func (fakeManagedRegistryReconciler) ListMatchingCertificates(context.Context, string, string) ([]k8s.ManagedRegistryCertificateOption, error) {
	return nil, nil
}
func (fakeManagedRegistryReconciler) EnsureTLSCertificate(context.Context, *model.ManagedOCIRegistry) error {
	return nil
}
func (fakeManagedRegistryReconciler) DeleteResources(context.Context, *model.ManagedOCIRegistry) {}
func (fakeManagedRegistryReconciler) EnsureResourcesAvailable(context.Context, string, string) error {
	return nil
}
func (fakeManagedRegistryReconciler) Apply(context.Context, *model.ManagedOCIRegistry, string, string) error {
	return nil
}
func (fakeManagedRegistryReconciler) LegacyHostPath(context.Context, *model.ManagedOCIRegistry) (bool, error) {
	return false, nil
}
func (fakeManagedRegistryReconciler) ManagedRegistryStatus(context.Context, *model.ManagedOCIRegistry) (string, string, string) {
	return "ready", "", ""
}

type fakeRegistryProxyReconciler struct{}

func (fakeRegistryProxyReconciler) Available() bool                          { return true }
func (fakeRegistryProxyReconciler) EnsureNode(context.Context, string) error { return nil }
func (fakeRegistryProxyReconciler) Apply(context.Context, *model.RegistryProxy, map[string]string) error {
	return nil
}
func (fakeRegistryProxyReconciler) ClearCache(context.Context, *model.RegistryProxy) error {
	return nil
}
func (fakeRegistryProxyReconciler) DeploymentAvailable(context.Context, *model.RegistryProxy) (bool, bool, error) {
	return true, true, nil
}
func (fakeRegistryProxyReconciler) DeleteLegacyResources(context.Context, string) error { return nil }
func (fakeRegistryProxyReconciler) DiagnoseUpstream(context.Context, *model.RegistryProxy) (k8s.RegistryProxyDiagnostic, error) {
	return k8s.RegistryProxyDiagnostic{Status: "ok"}, nil
}

func TestManagedRegistryHandlerAcceptsFakeReconciler(t *testing.T) {
	fake := fakeManagedRegistryReconciler{}
	h := (&ManagedOCIRegistryHandler{}).WithResourceReconciler(fake).WithStatusReader(fake)
	if h.resources == nil || h.status == nil || !h.status.Available() {
		t.Fatal("fake managed registry reconciler was not injected")
	}
}

func TestRegistryProxyHandlerAcceptsFakeReconciler(t *testing.T) {
	fake := fakeRegistryProxyReconciler{}
	h := (&RegistryProxyHandler{}).WithResourceReconciler(fake).WithDiagnostics(fake)
	if h.resources == nil || h.diagnostics == nil || !h.diagnostics.Available() {
		t.Fatal("fake registry proxy reconciler was not injected")
	}
}
