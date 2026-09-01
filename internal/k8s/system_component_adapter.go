package k8s

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SystemComponentKubernetesAdapter exposes only the Kubernetes operations
// required by the system-component Service. It keeps client wiring out of
// HTTP handlers and makes the boundary replaceable in tests.
type SystemComponentKubernetesAdapter struct{ Client *Client }

func (a SystemComponentKubernetesAdapter) Available() bool {
	return a.Client != nil && a.Client.Clientset != nil
}
func (a SystemComponentKubernetesAdapter) GetDeployment(ctx context.Context, ns, name string) (*appsv1.Deployment, error) {
	return a.Client.Clientset.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
}
func (a SystemComponentKubernetesAdapter) ListNodes(ctx context.Context) ([]corev1.Node, error) {
	l, e := a.Client.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if e != nil {
		return nil, e
	}
	return l.Items, nil
}
func (a SystemComponentKubernetesAdapter) ListPods(ctx context.Context, ns, selector string) ([]corev1.Pod, error) {
	l, e := a.Client.Clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if e != nil {
		return nil, e
	}
	return l.Items, nil
}
func (a SystemComponentKubernetesAdapter) HasDynamicClient() bool {
	return a.Client != nil && a.Client.DynamicClient != nil
}
func (a SystemComponentKubernetesAdapter) DetectSystemComponent(ctx context.Context, ns, name string) (SystemComponentDetection, error) {
	return a.Client.DetectSystemComponent(ctx, ns, name)
}
func (a SystemComponentKubernetesAdapter) GetNodeInfo(ctx context.Context, name string) (*NodeInfo, error) {
	return a.Client.GetNodeInfoContext(ctx, name)
}
func (a SystemComponentKubernetesAdapter) ApplyHelmChartConfig(ctx context.Context, ns, name, values string) error {
	return a.Client.ApplyHelmChartConfig(ctx, ns, name, values)
}
func (a SystemComponentKubernetesAdapter) DeleteHelmChartConfig(ctx context.Context, ns, name string) error {
	return a.Client.DeleteHelmChartConfig(ctx, ns, name)
}
func (a SystemComponentKubernetesAdapter) ApplyStaticDeploymentConfig(ctx context.Context, ns, name string, cfg StaticDeploymentConfig) error {
	return a.Client.ApplyStaticDeploymentConfig(ctx, ns, name, cfg)
}
func (a SystemComponentKubernetesAdapter) RestoreStaticDeploymentDefaults(ctx context.Context, ns, name string) error {
	return a.Client.RestoreStaticDeploymentDefaults(ctx, ns, name)
}
