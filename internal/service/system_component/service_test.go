package system_component

import (
	"context"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"testing"
)

type fakeAdapter struct{ applied, restored string }

func (f *fakeAdapter) ApplyHelmChartConfig(_ context.Context, _, name, _ string) error {
	f.applied = "helm:" + name
	return nil
}
func (f *fakeAdapter) DeleteHelmChartConfig(_ context.Context, _, name string) error {
	f.restored = "helm:" + name
	return nil
}
func (f *fakeAdapter) ApplyStaticDeploymentConfig(_ context.Context, _, name string, _ k8s.StaticDeploymentConfig) error {
	f.applied = "static:" + name
	return nil
}
func (f *fakeAdapter) RestoreStaticDeploymentDefaults(_ context.Context, _, name string) error {
	f.restored = "static:" + name
	return nil
}

func TestNamespaceReturnsCopyAndWhitelist(t *testing.T) {
	if ns, ok := Namespace(" traefik "); !ok || ns != "kube-system" {
		t.Fatalf("unexpected namespace %q %v", ns, ok)
	}
	charts := Charts()
	charts["custom"] = "x"
	if _, ok := Namespace("custom"); ok {
		t.Fatal("whitelist leaked mutable map")
	}
}

func TestValidateReadTimeout(t *testing.T) {
	if err := ValidateReadTimeout("5m"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReadTimeout(""); err == nil {
		t.Fatal("expected empty timeout rejection")
	}
}

func TestApplyAndRestoreUseWhitelistedNamespace(t *testing.T) {
	f := &fakeAdapter{}
	if err := Apply(context.Background(), f, "traefik", "values", nil); err != nil || f.applied != "helm:traefik" {
		t.Fatalf("apply: %v %s", err, f.applied)
	}
	if err := Restore(context.Background(), f, "traefik", false); err != nil || f.restored != "helm:traefik" {
		t.Fatalf("restore: %v %s", err, f.restored)
	}
}

type fakeUpdateRepo struct{ config *model.SystemComponentConfig }

func (r *fakeUpdateRepo) UpsertSystemComponentConfig(c *model.SystemComponentConfig) error {
	r.config = c
	return nil
}
func (r *fakeUpdateRepo) DeleteSystemComponentConfig(string) error { return nil }

func TestApplyUpdatePersistsSuccess(t *testing.T) {
	f, repo := &fakeAdapter{}, &fakeUpdateRepo{}
	c := &model.SystemComponentConfig{ChartName: "traefik", Namespace: "kube-system", ValuesContent: "{}"}
	if err := ApplyUpdate(context.Background(), repo, f, c, k8s.HelmChartMode, nil); err != nil {
		t.Fatal(err)
	}
	if repo.config != c || c.ApplyStatus != "succeeded" || f.applied != "helm:traefik" {
		t.Fatalf("unexpected update: %#v %s", repo.config, f.applied)
	}
}

type fakeNodePodReader struct {
	nodes []corev1.Node
	pods  []corev1.Pod
}

func (f fakeNodePodReader) ListNodes(context.Context) ([]corev1.Node, error) { return f.nodes, nil }
func (f fakeNodePodReader) ListPods(context.Context, string, string) ([]corev1.Pod, error) {
	return f.pods, nil
}
func TestValidateHAIncreaseRejectsImagePullFailure(t *testing.T) {
	node := corev1.Node{Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}, Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("1Gi")}}}
	pod := corev1.Pod{Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}}}}}
	if err := ValidateHAIncrease(context.Background(), fakeNodePodReader{nodes: []corev1.Node{node, node}, pods: []corev1.Pod{pod}}, "coredns", "kube-system", 1, 1, 2, ""); err == nil {
		t.Fatal("expected image pull preflight error")
	}
}

func TestConfiguredReadTimeoutRequiresBothEntrypoints(t *testing.T) {
	content := "additionalArguments:\n  - --entryPoints.web.transport.respondingTimeouts.readTimeout=5m\n  - --entryPoints.websecure.transport.respondingTimeouts.readTimeout=5m\n"
	if got := ConfiguredReadTimeout(content); got != "5m" {
		t.Fatalf("got %q", got)
	}
	if got := ConfiguredReadTimeout("additionalArguments: []\n"); got != "" {
		t.Fatalf("expected empty timeout, got %q", got)
	}
}
