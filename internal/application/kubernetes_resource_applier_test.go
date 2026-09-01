package application

import (
	"context"
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type fakeApplicationResourceApplier struct {
	calls []string
	err   error
}

func (f *fakeApplicationResourceApplier) record(name string) error {
	f.calls = append(f.calls, name)
	return f.err
}
func (f *fakeApplicationResourceApplier) ApplyConfigMap(context.Context, *corev1.ConfigMap) error {
	return f.record("configmap")
}
func (f *fakeApplicationResourceApplier) ApplySecret(context.Context, *corev1.Secret) error {
	return f.record("secret")
}
func (f *fakeApplicationResourceApplier) ApplyDeployment(context.Context, *appsv1.Deployment) error {
	return f.record("deployment")
}
func (f *fakeApplicationResourceApplier) ApplyStatefulSet(context.Context, *appsv1.StatefulSet) error {
	return f.record("statefulset")
}
func (f *fakeApplicationResourceApplier) ApplyService(context.Context, *corev1.Service) error {
	return f.record("service")
}
func (f *fakeApplicationResourceApplier) ApplyIngress(context.Context, *networkingv1.Ingress) error {
	return f.record("ingress")
}
func (f *fakeApplicationResourceApplier) ApplyCertificate(context.Context, *unstructured.Unstructured) error {
	return f.record("certificate")
}

func TestKubernetesApplierApplyDelegatesResourcesInDependencyOrder(t *testing.T) {
	fake := &fakeApplicationResourceApplier{}
	applier := &KubernetesApplier{resources: fake}
	resources := &RenderedResources{
		ImagePullSecret: &corev1.Secret{}, ConfigMap: &corev1.ConfigMap{}, Secret: &corev1.Secret{},
		Deployment: &appsv1.Deployment{}, StatefulSet: &appsv1.StatefulSet{}, Service: &corev1.Service{},
		Certificate: &unstructured.Unstructured{}, Ingress: &networkingv1.Ingress{},
	}
	if err := applier.Apply(context.Background(), resources); err != nil {
		t.Fatal(err)
	}
	want := []string{"secret", "configmap", "secret", "deployment", "statefulset", "service", "certificate", "ingress"}
	if len(fake.calls) != len(want) {
		t.Fatalf("calls = %#v", fake.calls)
	}
	for i := range want {
		if fake.calls[i] != want[i] {
			t.Fatalf("calls = %#v, want %#v", fake.calls, want)
		}
	}
}

func TestKubernetesApplierApplyPropagatesResourceError(t *testing.T) {
	wantErr := errors.New("apply failed")
	fake := &fakeApplicationResourceApplier{err: wantErr}
	applier := &KubernetesApplier{resources: fake}
	err := applier.Apply(context.Background(), &RenderedResources{Service: &corev1.Service{}})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}
