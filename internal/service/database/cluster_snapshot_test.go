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

func TestSnapshotUsesObservedServiceAndInstanceAddresses(t *testing.T) {
	ctx := context.Background()
	routerInstances := 1
	item := &model.MySQLCluster{Namespace: "cylism-databases", ResourceName: "cylism-mysql-cluster-1", DatabaseInstances: 1, RouterInstances: &routerInstances}
	client := &k8s.Client{
		Clientset: k8sfake.NewSimpleClientset(
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator", Namespace: "mysql-operator"}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1}},
			&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName, Namespace: item.Namespace}, Status: appsv1.StatefulSetStatus{ReadyReplicas: 1}},
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName + "-router", Namespace: item.Namespace}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1}},
			&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName, Namespace: item.Namespace}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 6446}, {Port: 6447}}}},
			&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName + "-instances", Namespace: item.Namespace}, Spec: corev1.ServiceSpec{ClusterIP: corev1.ClusterIPNone}},
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName + "-0", Namespace: item.Namespace, Labels: map[string]string{"mysql.oracle.com/cluster": item.ResourceName, "component": "mysqld"}}, Status: corev1.PodStatus{PodIP: "10.42.4.5"}},
		),
		DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{innoDBClusterGVR: "InnoDBClusterList"}, ClusterObject(item)),
	}
	snapshot, err := Snapshot(ctx, client, item)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.RouterReadWrite != "cylism-mysql-cluster-1.cylism-databases.svc.cluster.local:6446" || snapshot.RouterReadOnly != "cylism-mysql-cluster-1.cylism-databases.svc.cluster.local:6447" {
		t.Fatalf("router addresses: %+v", snapshot)
	}
	if !snapshot.ConnectionChecked {
		t.Fatal("connection details were not marked as checked")
	}
	if len(snapshot.Pods) != 1 || snapshot.Pods[0].Address != "cylism-mysql-cluster-1-0.cylism-mysql-cluster-1-instances.cylism-databases.svc.cluster.local:3306" {
		t.Fatalf("instance address: %+v", snapshot.Pods)
	}
	if err := client.Clientset.CoreV1().Services(item.Namespace).Delete(ctx, item.ResourceName+"-instances", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = Snapshot(ctx, client, item)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Pods[0].Address != "10.42.4.5:3306" {
		t.Fatalf("fallback pod address: %q", snapshot.Pods[0].Address)
	}
}
