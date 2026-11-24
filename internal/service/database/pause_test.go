package database

import (
	"context"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestPauseUnstartedClusterRemovesOnlyItsCustomResource(t *testing.T) {
	ctx := context.Background()
	item := &model.MySQLCluster{Namespace: Namespace, ResourceName: "cylism-mysql-cluster-1"}
	resource := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "mysql.oracle.com/v2", "kind": "InnoDBCluster",
		"metadata": map[string]any{"name": item.ResourceName, "namespace": item.Namespace, "labels": map[string]any{k8s.ManagedByLabel: k8s.ManagedByValue}},
	}}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(), DynamicClient: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), resource)}
	if err := PauseCluster(ctx, client, item); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{}); err == nil {
		t.Fatal("cluster custom resource was not removed")
	}
}

func TestCleanupPausedClusterWaitsForWorkloadAndPodBeforeDeletingClaim(t *testing.T) {
	ctx := context.Background()
	item := &model.MySQLCluster{Namespace: Namespace, ResourceName: "cylism-mysql-cluster-1"}
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "data-" + item.ResourceName + "-0", Namespace: item.Namespace}}
	set := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName, Namespace: item.Namespace}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName + "-0", Namespace: item.Namespace}}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(claim, set, pod), DynamicClient: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())}
	for step := 0; step < 2; step++ {
		if _, err := CleanupPausedCluster(ctx, client, item, false); err != nil {
			t.Fatal(err)
		}
		if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Get(ctx, claim.Name, metav1.GetOptions{}); err != nil {
			t.Fatalf("claim removed before workload and Pod exited: %v", err)
		}
	}
	if _, err := CleanupPausedCluster(ctx, client, item, false); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Get(ctx, claim.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("workload cleanup removed claim: %v", err)
	}
	if _, err := CleanupPausedCluster(ctx, client, item, true); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Get(ctx, claim.Name, metav1.GetOptions{}); err == nil {
		t.Fatal("claim should be removed after workloads and Pods exit")
	}
}

func TestPauseClusterWithWorkloadPreservesExistingData(t *testing.T) {
	ctx := context.Background()
	item := &model.MySQLCluster{Namespace: Namespace, ResourceName: "cylism-mysql-cluster-1"}
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "data-" + item.ResourceName + "-0", Namespace: item.Namespace, Labels: map[string]string{"mysql.oracle.com/cluster": item.ResourceName}}}
	cluster := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "mysql.oracle.com/v2", "kind": "InnoDBCluster", "metadata": map[string]any{"name": item.ResourceName, "namespace": item.Namespace, "labels": map[string]any{k8s.ManagedByLabel: k8s.ManagedByValue}}}}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(claim), DynamicClient: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), cluster)}
	if err := PauseCluster(ctx, client, item); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Get(ctx, claim.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("pause removed existing data volume: %v", err)
	}
	state, err := InspectClusterCleanup(ctx, client, item)
	if err != nil || len(state.Claims) != 1 {
		t.Fatalf("retained claim not visible for cleanup: state=%+v err=%v", state, err)
	}
	if _, err := CleanupPausedCluster(ctx, client, item, false); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Get(ctx, claim.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("workload cleanup removed claim: %v", err)
	}
	if _, err := CleanupPausedCluster(ctx, client, item, true); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Get(ctx, claim.Name, metav1.GetOptions{}); err == nil {
		t.Fatal("explicit cleanup did not remove the claim")
	}
}

func TestRetryClusterShutdownRequiresStalledManagedDeletionAndThrottlesRestarts(t *testing.T) {
	ctx := context.Background()
	item := &model.MySQLCluster{Namespace: Namespace, ResourceName: "cylism-mysql-cluster-1"}
	cluster := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "mysql.oracle.com/v2", "kind": "InnoDBCluster", "metadata": map[string]any{"name": item.ResourceName, "namespace": item.Namespace, "labels": map[string]any{k8s.ManagedByLabel: k8s.ManagedByValue}}}}
	operator := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator", Namespace: "mysql-operator"}}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(operator), DynamicClient: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), cluster)}
	if err := RetryClusterShutdown(ctx, client, item); err == nil {
		t.Fatal("retry accepted a cluster without a deletion request")
	}
	resource := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace)
	cluster, err := resource.Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	started := metav1.NewTime(time.Now().Add(-4 * time.Minute))
	cluster.SetDeletionTimestamp(&started)
	cluster.SetFinalizers([]string{"mysql.oracle.com/cluster"})
	if _, err := resource.Update(ctx, cluster, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	state, err := InspectClusterCleanup(ctx, client, item)
	if err != nil || !state.RetryAvailable || state.DeletionStartedAt == nil || len(state.Finalizers) != 1 {
		t.Fatalf("stalled deletion not exposed: state=%+v err=%v", state, err)
	}
	if err := RetryClusterShutdown(ctx, client, item); err != nil {
		t.Fatal(err)
	}
	operator, err = client.Clientset.AppsV1().Deployments("mysql-operator").Get(ctx, "mysql-operator", metav1.GetOptions{})
	if err != nil || operator.Spec.Template.Annotations[operatorRestartAnnotation] == "" {
		t.Fatalf("operator restart not requested: deployment=%+v err=%v", operator, err)
	}
	if err := RetryClusterShutdown(ctx, client, item); err == nil {
		t.Fatal("retry accepted a second operator restart within the cooldown")
	}
	state, err = InspectClusterCleanup(ctx, client, item)
	if err != nil || state.RetryAvailable {
		t.Fatalf("cooldown not reflected in cleanup state: state=%+v err=%v", state, err)
	}
}
