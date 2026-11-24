package database

import (
	"context"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestDatabaseSchedulingExceptionPreservesApplicationRestriction(t *testing.T) {
	ctx := context.Background()
	ready := corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}
	nodes := []*corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "a"}, Status: ready},
		{ObjectMeta: metav1.ObjectMeta{Name: "b"}, Status: ready},
		{ObjectMeta: metav1.ObjectMeta{Name: "ubuntudev"}, Status: ready, Spec: corev1.NodeSpec{Taints: []corev1.Taint{{Key: databaseNoScheduleTaintKey, Value: "true", Effect: corev1.TaintEffectNoSchedule}}}},
	}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(nodes[0], nodes[1], nodes[2])}
	if err := ValidateClusterNodes(ctx, client, []string{"a", "b", "ubuntudev"}, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateClusterNodes(ctx, client, []string{"a", "b", "ubuntudev"}, []string{"a", "ubuntudev"}); err == nil {
		t.Fatal("Router must not inherit the database scheduling exception")
	}
	options, err := DeploymentNodes(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	for _, option := range options {
		if option.Name == "ubuntudev" && (!option.DatabaseAllowed || option.RouterAllowed || !option.DefaultNoSchedule) {
			t.Fatalf("incorrect scheduling options: %+v", option)
		}
	}
	for _, taint := range []corev1.Taint{
		{Key: "example.com/maintenance", Value: "true", Effect: corev1.TaintEffectNoSchedule},
		{Key: databaseNoScheduleTaintKey, Value: "true", Effect: corev1.TaintEffectNoExecute},
		{Key: databaseNoScheduleTaintKey, Value: "other", Effect: corev1.TaintEffectNoSchedule},
	} {
		nodes[2].Spec.Taints = []corev1.Taint{taint}
		_, _ = client.Clientset.CoreV1().Nodes().Update(ctx, nodes[2], metav1.UpdateOptions{})
		if err := ValidateClusterNodes(ctx, client, []string{"a", "b", "ubuntudev"}, []string{"a", "b"}); err == nil {
			t.Fatalf("unrelated taint was tolerated: %+v", taint)
		}
	}
	nodes[2].Spec.Taints = nil
	nodes[2].Spec.Unschedulable = true
	_, _ = client.Clientset.CoreV1().Nodes().Update(ctx, nodes[2], metav1.UpdateOptions{})
	if err := ValidateClusterNodes(ctx, client, []string{"a", "b", "ubuntudev"}, []string{"a", "b"}); err == nil {
		t.Fatal("cordoned nodes must remain blocked")
	}
}

func TestResolveStorageClassRequiresRealClassAndUnambiguousDefault(t *testing.T) {
	ctx := context.Background()
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local-path", Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}}, Provisioner: "rancher.io/local-path"})}
	if name, err := ResolveStorageClass(ctx, client, ""); name != "local-path" || err != nil {
		t.Fatalf("default = %q, %v", name, err)
	}
	if _, err := ResolveStorageClass(ctx, client, "typo"); err == nil {
		t.Fatal("nonexistent class accepted")
	}
	_, _ = client.Clientset.StorageV1().StorageClasses().Create(ctx, &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "other", Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}}}, metav1.CreateOptions{})
	if _, err := ResolveStorageClass(ctx, client, ""); err == nil {
		t.Fatal("ambiguous default accepted")
	}
	if name, err := ResolveStorageClass(ctx, client, "local-path"); err != nil || name != "local-path" {
		t.Fatalf("explicit choice rejected: %q, %v", name, err)
	}
	client.Clientset = k8sfake.NewSimpleClientset()
	if _, err := ResolveStorageClass(ctx, client, ""); err == nil {
		t.Fatal("missing default accepted")
	}
}

func TestPrimaryNodeMustBelongToDatabaseSelection(t *testing.T) {
	if err := ValidatePrimaryNode("router-only", []string{"a", "b", "c"}); err == nil {
		t.Fatal("primary outside database placement accepted")
	}
	if err := ValidatePrimaryNode("b", []string{"a", "b", "c"}); err != nil {
		t.Fatal(err)
	}
}
