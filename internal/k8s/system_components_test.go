package k8s

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func helmChartConfigScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "helm.cattle.io", Version: "v1", Kind: "HelmChart"}, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "helm.cattle.io", Version: "v1", Kind: "HelmChartList"}, &unstructured.UnstructuredList{})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "helm.cattle.io", Version: "v1", Kind: "HelmChartConfig"}, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "helm.cattle.io", Version: "v1", Kind: "HelmChartConfigList"}, &unstructured.UnstructuredList{})
	return scheme
}

func TestDetectSystemComponentUsesRuntimeControlSource(t *testing.T) {
	replicas := int32(1)
	client := &Client{
		Clientset: k8sfake.NewSimpleClientset(
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"}, Spec: appsv1.DeploymentSpec{Replicas: &replicas}},
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "traefik-proxy", Namespace: "kube-system", Labels: map[string]string{"app.kubernetes.io/name": "traefik"}}, Spec: appsv1.DeploymentSpec{Replicas: &replicas}},
			&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "svclb-traefik", Namespace: "kube-system"}},
		),
		DynamicClient: dynamicfake.NewSimpleDynamicClient(helmChartConfigScheme(), &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "helm.cattle.io/v1",
			"kind":       "HelmChart",
			"metadata":   map[string]any{"name": "traefik", "namespace": "kube-system"},
			"status":     map[string]any{"conditions": []any{map[string]any{"type": "Ready", "status": "True", "message": "deployed"}}},
		}}),
	}

	cases := []struct {
		name string
		want ControllerMode
	}{
		{name: "traefik", want: HelmChartMode},
		{name: "coredns", want: StaticDeploymentMode},
		{name: "servicelb", want: EmbeddedMode},
		{name: "metrics-server", want: UnknownMode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := client.DetectSystemComponent(context.Background(), "kube-system", tc.name)
			if err != nil {
				t.Fatalf("detect: %v", err)
			}
			if result.Mode != tc.want {
				t.Fatalf("mode = %q, want %q; evidence=%v", result.Mode, tc.want, result.Evidence)
			}
			if tc.name == "traefik" {
				if !result.ChartReady || result.Workload == nil || result.Workload.Name != "traefik-proxy" {
					t.Fatalf("helm workload status = %#v, want ready traefik-proxy", result)
				}
			}
		})
	}
}

func TestDetectSystemComponentDoesNotPatchWhenHelmControlCannotBeChecked(t *testing.T) {
	replicas := int32(1)
	client := &Client{Clientset: k8sfake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
	})}
	result, err := client.DetectSystemComponent(context.Background(), "kube-system", "coredns")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if result.Mode != UnknownMode {
		t.Fatalf("mode = %q, want unknown when HelmChart cannot be checked", result.Mode)
	}
}

func TestHelmChartConfigLifecycle(t *testing.T) {
	client := &Client{DynamicClient: dynamicfake.NewSimpleDynamicClient(helmChartConfigScheme())}
	ctx := context.Background()

	if err := client.ApplyHelmChartConfig(ctx, "kube-system", "coredns", "replicas: 2\nmaxUnavailable: 0\n"); err != nil {
		t.Fatalf("apply create: %v", err)
	}
	obj, err := client.GetHelmChartConfig(ctx, "kube-system", "coredns")
	if err != nil || obj == nil {
		t.Fatalf("get after create: %v %#v", err, obj)
	}
	spec, _, _ := unstructured.NestedString(obj.Object, "spec", "valuesContent")
	if spec != "replicas: 2\nmaxUnavailable: 0\n" {
		t.Fatalf("unexpected valuesContent: %q", spec)
	}

	if err := client.ApplyHelmChartConfig(ctx, "kube-system", "coredns", "replicas: 1\n"); err != nil {
		t.Fatalf("apply update: %v", err)
	}
	obj, _ = client.GetHelmChartConfig(ctx, "kube-system", "coredns")
	spec, _, _ = unstructured.NestedString(obj.Object, "spec", "valuesContent")
	if spec != "replicas: 1\n" {
		t.Fatalf("update must replace values: %q", spec)
	}

	if err := client.DeleteHelmChartConfig(ctx, "kube-system", "coredns"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	obj, err = client.GetHelmChartConfig(ctx, "kube-system", "coredns")
	if err != nil || obj != nil {
		t.Fatalf("expected nil after delete: %v %#v", err, obj)
	}
}

func TestHelmChartConfigDeleteMissingIsNoop(t *testing.T) {
	client := &Client{DynamicClient: dynamicfake.NewSimpleDynamicClient(helmChartConfigScheme())}
	if err := client.DeleteHelmChartConfig(context.Background(), "kube-system", "absent"); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestApplyCoreDNSConfigUpdatesRolloutAndNodePlacement(t *testing.T) {
	replicas := int32(1)
	client := &Client{Clientset: k8sfake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{NodeSelector: map[string]string{"kubernetes.io/os": "linux"}}},
		},
	})}

	err := client.ApplyCoreDNSConfig(context.Background(), CoreDNSConfig{
		Replicas:       2,
		MaxUnavailable: intstr.FromInt(0),
		MaxSurge:       intstr.FromInt(1),
		NodeName:       "worker-b",
	})
	if err != nil {
		t.Fatalf("apply coredns config: %v", err)
	}
	deployment, err := client.Clientset.AppsV1().Deployments("kube-system").Get(context.Background(), "coredns", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get coredns deployment: %v", err)
	}
	if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 2 {
		t.Fatalf("replicas = %#v, want 2", deployment.Spec.Replicas)
	}
	if deployment.Spec.Strategy.RollingUpdate == nil || deployment.Spec.Strategy.RollingUpdate.MaxUnavailable.IntValue() != 0 || deployment.Spec.Strategy.RollingUpdate.MaxSurge.IntValue() != 1 {
		t.Fatalf("unexpected rollout strategy: %#v", deployment.Spec.Strategy)
	}
	if deployment.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] != "worker-b" {
		t.Fatalf("hostname selector = %#v", deployment.Spec.Template.Spec.NodeSelector)
	}

	err = client.ApplyCoreDNSConfig(context.Background(), CoreDNSConfig{Replicas: 2, MaxUnavailable: intstr.FromInt(0), MaxSurge: intstr.FromInt(1)})
	if err != nil {
		t.Fatalf("remove coredns placement: %v", err)
	}
	deployment, _ = client.Clientset.AppsV1().Deployments("kube-system").Get(context.Background(), "coredns", metav1.GetOptions{})
	if _, pinned := deployment.Spec.Template.Spec.NodeSelector[corev1.LabelHostname]; pinned {
		t.Fatalf("hostname selector was not removed: %#v", deployment.Spec.Template.Spec.NodeSelector)
	}
	if deployment.Spec.Template.Spec.NodeSelector["kubernetes.io/os"] != "linux" {
		t.Fatalf("existing system selector must be preserved: %#v", deployment.Spec.Template.Spec.NodeSelector)
	}
}

func TestApplyStaticDeploymentConfigUpdatesOnlyManagedFields(t *testing.T) {
	replicas := int32(1)
	client := &Client{Clientset: k8sfake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "metrics-server", Namespace: "kube-system"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{NodeSelector: map[string]string{"kubernetes.io/os": "linux"}}},
		},
	})}

	err := client.ApplyStaticDeploymentConfig(context.Background(), "kube-system", "metrics-server", StaticDeploymentConfig{
		Replicas:       2,
		MaxUnavailable: intstr.FromInt(0),
		MaxSurge:       intstr.FromInt(1),
		NodeName:       "worker-b",
	})
	if err != nil {
		t.Fatalf("apply static deployment config: %v", err)
	}
	deployment, err := client.Clientset.AppsV1().Deployments("kube-system").Get(context.Background(), "metrics-server", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 2 {
		t.Fatalf("replicas = %#v, want 2", deployment.Spec.Replicas)
	}
	if deployment.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] != "worker-b" || deployment.Spec.Template.Spec.NodeSelector["kubernetes.io/os"] != "linux" {
		t.Fatalf("node selectors = %#v", deployment.Spec.Template.Spec.NodeSelector)
	}
}
