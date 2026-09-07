package logging

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type filterReaderFake struct{}

func (filterReaderFake) ListRuntimePods(context.Context, string, string) ([]corev1.Pod, error) {
	return []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "z", Namespace: "ns"}, Spec: corev1.PodSpec{NodeName: "node-b", Containers: []corev1.Container{{Name: "web"}, {Name: "api"}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"}, Spec: corev1.PodSpec{NodeName: "node-a"}},
	}, nil
}
func (filterReaderFake) ListNodesContext(context.Context) ([]corev1.Node, error) {
	return []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "node-b"}}, {ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}}, nil
}

func TestFiltersBuildsSortedPodNamespaceAndNodeOptions(t *testing.T) {
	result, err := Filters(context.Background(), "ns", filterReaderFake{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pods) != 2 || result.Pods[0].Name != "a" || result.Pods[1].Containers[0] != "api" {
		t.Fatalf("unexpected pods: %#v", result.Pods)
	}
	if len(result.Namespaces) != 1 || result.Namespaces[0] != "ns" || len(result.Nodes) != 2 || result.Nodes[0] != "node-a" {
		t.Fatalf("unexpected filter options: %#v", result)
	}
}
