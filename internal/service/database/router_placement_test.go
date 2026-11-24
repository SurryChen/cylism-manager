package database

import (
	"context"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestPrepareFirstRouterPlacementRollsBackAndRejectsActiveDeployment(t *testing.T) {
	ctx := context.Background()
	zero := int32(0)
	item := &model.MySQLCluster{Namespace: Namespace, ResourceName: "mysql-router-placement", RouterNodes: []string{"node-b"}}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName + "-router", Namespace: item.Namespace},
		Spec:       appsv1.DeploymentSpec{Replicas: &zero, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"mysql.oracle.com/cluster": item.ResourceName}}}},
	}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(deployment)}
	rollback, err := PrepareFirstRouterPlacement(ctx, client, item)
	if err != nil {
		t.Fatal(err)
	}
	current, err := client.Clientset.AppsV1().Deployments(item.Namespace).Get(ctx, deployment.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if current.Spec.Template.Spec.Affinity == nil || !matchesDatabaseNodePool(current.Spec.Template.Spec.Affinity.NodeAffinity, item.RouterNodes) {
		t.Fatalf("selected Router node absent from Deployment template: %+v", current.Spec.Template.Spec)
	}
	if err := rollback(); err != nil {
		t.Fatal(err)
	}
	current, err = client.Clientset.AppsV1().Deployments(item.Namespace).Get(ctx, deployment.Name, metav1.GetOptions{})
	if err != nil || current.Spec.Template.Spec.Affinity != nil {
		t.Fatalf("Router template not restored: %+v, %v", current.Spec.Template.Spec, err)
	}
	one := int32(1)
	current.Spec.Replicas = &one
	if _, err := client.Clientset.AppsV1().Deployments(item.Namespace).Update(ctx, current, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareFirstRouterPlacement(ctx, client, item); err == nil {
		t.Fatal("active Router Deployment was modified as a first deployment")
	}
}

func TestRepairRouterPlacementRollsOutOnlyForWrongNode(t *testing.T) {
	ctx := context.Background()
	one := int32(1)
	item := &model.MySQLCluster{Namespace: Namespace, ResourceName: "mysql-router-repair", DatabaseNodes: []string{"node-a", "node-b"}, RouterNodes: []string{"node-b"}}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName + "-router", Namespace: item.Namespace},
		Spec:       appsv1.DeploymentSpec{Replicas: &one, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"mysql.oracle.com/cluster": item.ResourceName, "component": "mysqlrouter"}}}},
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: deployment.Name + "-abc", Namespace: item.Namespace, Labels: map[string]string{"mysql.oracle.com/cluster": item.ResourceName, "component": "mysqlrouter"}}, Spec: corev1.PodSpec{NodeName: "node-a"}}
	gvr := schema.GroupVersionResource{Group: "mysql.oracle.com", Version: "v2", Resource: "innodbclusters"}
	client := &k8s.Client{
		Clientset:     k8sfake.NewSimpleClientset(deployment, pod),
		DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "InnoDBClusterList"}, ClusterObject(item)),
	}
	if err := RepairRouterPlacement(ctx, client, item); err != nil {
		t.Fatal(err)
	}
	updated, err := client.Clientset.AppsV1().Deployments(item.Namespace).Get(ctx, deployment.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Spec.Template.Spec.Affinity == nil || !matchesDatabaseNodePool(updated.Spec.Template.Spec.Affinity.NodeAffinity, item.RouterNodes) || updated.Spec.Template.Annotations["cylism.io/router-placement-repaired-at"] == "" {
		t.Fatalf("Router rollout did not target saved nodes: %+v", updated.Spec.Template)
	}
	pod.Spec.NodeName = "node-b"
	if _, err := client.Clientset.CoreV1().Pods(item.Namespace).Update(ctx, pod, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := RepairRouterPlacement(ctx, client, item); err == nil {
		t.Fatal("Router already within candidates was restarted")
	}
}
