package k8s

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func helmChartConfigScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "helm.cattle.io", Version: "v1", Kind: "HelmChartConfig"}, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "helm.cattle.io", Version: "v1", Kind: "HelmChartConfigList"}, &unstructured.UnstructuredList{})
	return scheme
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
