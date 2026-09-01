package system

import (
	"context"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	systemcomponentservice "github.com/cylism/cylism-manager/internal/service/system_component"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type fakeSystemComponentAdapter struct{ client *fake.Clientset }

func (f fakeSystemComponentAdapter) Available() bool { return f.client != nil }
func (f fakeSystemComponentAdapter) GetDeployment(context.Context, string, string) (*appsv1.Deployment, error) {
	return &appsv1.Deployment{}, nil
}
func (f fakeSystemComponentAdapter) ListNodes(context.Context) ([]corev1.Node, error) {
	return nil, nil
}
func (f fakeSystemComponentAdapter) ListPods(context.Context, string, string) ([]corev1.Pod, error) {
	return nil, nil
}
func (f fakeSystemComponentAdapter) HasDynamicClient() bool   { return false }
func (f fakeSystemComponentAdapter) Context() context.Context { return context.Background() }
func (f fakeSystemComponentAdapter) DetectSystemComponent(context.Context, string, string) (k8s.SystemComponentDetection, error) {
	return k8s.SystemComponentDetection{}, nil
}
func (f fakeSystemComponentAdapter) GetNodeInfo(_ context.Context, name string) (*k8s.NodeInfo, error) {
	return &k8s.NodeInfo{Name: name, Ready: true}, nil
}
func (f fakeSystemComponentAdapter) ApplyHelmChartConfig(context.Context, string, string, string) error {
	return nil
}
func (f fakeSystemComponentAdapter) DeleteHelmChartConfig(context.Context, string, string) error {
	return nil
}
func (f fakeSystemComponentAdapter) ApplyStaticDeploymentConfig(context.Context, string, string, k8s.StaticDeploymentConfig) error {
	return nil
}
func (f fakeSystemComponentAdapter) RestoreStaticDeploymentDefaults(context.Context, string, string) error {
	return nil
}

func TestSystemComponentHandlerUsesInjectedAdapterForNodeValidation(t *testing.T) {
	client := fake.NewSimpleClientset()
	h := (&SystemComponentHandler{}).WithAdapter(fakeSystemComponentAdapter{client: client})
	if err := systemcomponentservice.ValidateNode(context.Background(), h.adapter, "node-a"); err != nil {
		t.Fatal(err)
	}
}
