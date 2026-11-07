package database

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func lifecycleFixture(t *testing.T) (*k8s.Client, *model.MySQLCluster) {
	t.Helper()
	zero, one := int32(0), 1
	item := &model.MySQLCluster{ID: 1, Namespace: Namespace, ResourceName: "cylism-mysql-cluster-1", DatabaseInstances: 2, RouterInstances: &one}
	operator := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator", Namespace: "mysql-operator"}, Spec: appsv1.DeploymentSpec{Replicas: &zero}}
	set := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName, Namespace: item.Namespace}, Spec: appsv1.StatefulSetSpec{Replicas: &zero}, Status: appsv1.StatefulSetStatus{ReadyReplicas: 2}}
	router := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName + "-router", Namespace: item.Namespace}, Spec: appsv1.DeploymentSpec{Replicas: &zero}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1}}
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "datadir-" + item.ResourceName + "-0", Namespace: item.Namespace, OwnerReferences: []metav1.OwnerReference{{Kind: "InnoDBCluster", Name: item.ResourceName, UID: types.UID("cluster")}}}}
	independent := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "cylism-mysql-2", Namespace: item.Namespace}, Spec: appsv1.StatefulSetSpec{Replicas: func() *int32 { x := int32(1); return &x }()}}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(operator, set, router, claim, independent), DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{innoDBClusterGVR: "InnoDBClusterList", mysqlOperatorHelmGVR: "HelmChartList"}, ClusterObject(item))}
	return client, item
}
func TestStoppedClusterSnapshotReadableWithSuspendedOperator(t *testing.T) {
	client, item := lifecycleFixture(t)
	snapshot, err := Snapshot(context.Background(), client, item)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Status != "stopped" || snapshot.OnlineInstances != 0 || snapshot.RouterReady != 0 || len(snapshot.Claims) != 1 {
		t.Fatalf("stale state or missing data: %+v", snapshot)
	}
	state, err := OperatorInstallationStatus(context.Background(), client)
	if err != nil || state.State != "suspended" || state.CanCancel {
		t.Fatalf("operator: %+v %v", state, err)
	}
}
func TestStopRefusesToSuspendSharedOperator(t *testing.T) {
	client, item := lifecycleFixture(t)
	other := *item
	other.Namespace = "other"
	other.ResourceName = "another"
	_, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(other.Namespace).Create(context.Background(), ClusterObject(&other), metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	client.Clientset.(*k8sfake.Clientset).ClearActions()
	if _, _, err := StopClusterStep(context.Background(), client, item); err == nil {
		t.Fatal("shared controller suspension allowed")
	}
	for _, action := range client.Clientset.(*k8sfake.Clientset).Actions() {
		if action.GetVerb() == "update" {
			t.Fatalf("mutated resource on rejected stop: %+v", action)
		}
	}
}
func TestResumePreservesIdentityAndRestoresCounts(t *testing.T) {
	client, item := lifecycleFixture(t)
	if err := ResumeCluster(context.Background(), client, item); err != nil {
		t.Fatal(err)
	}
	set, _ := client.Clientset.AppsV1().StatefulSets(item.Namespace).Get(context.Background(), item.ResourceName, metav1.GetOptions{})
	router, _ := client.Clientset.AppsV1().Deployments(item.Namespace).Get(context.Background(), item.ResourceName+"-router", metav1.GetOptions{})
	operator, _ := operatorDeployment(context.Background(), client)
	if *set.Spec.Replicas != 2 || *router.Spec.Replicas != 1 || *operator.Spec.Replicas != 1 {
		t.Fatal("original counts were not restored")
	}
	for _, action := range client.DynamicClient.(*dynamicfake.FakeDynamicClient).Actions() {
		if action.GetVerb() == "create" || action.GetVerb() == "update" {
			t.Fatal("resume must not recreate custom resource")
		}
	}
}
func TestRetireAndDeleteCheckPreserveClaimsAndIndependentMySQL(t *testing.T) {
	client, item := lifecycleFixture(t)
	ctx := context.Background()
	if err := RemoveClusterRecordCheck(ctx, client, item); err == nil {
		t.Fatal("allowed orphaned live cluster")
	}
	for i := 0; i < 4; i++ {
		_, _, err := RetireClusterStep(ctx, client, item)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveClusterRecordCheck(ctx, client, item); err != nil {
		t.Fatal(err)
	}
	claim, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Get(ctx, "datadir-"+item.ResourceName+"-0", metav1.GetOptions{})
	if err != nil || len(claim.OwnerReferences) != 0 {
		t.Fatalf("retained claim not orphaned safely: %+v %v", claim, err)
	}
	set, err := client.Clientset.AppsV1().StatefulSets(item.Namespace).Get(ctx, "cylism-mysql-2", metav1.GetOptions{})
	if err != nil || *set.Spec.Replicas != 1 {
		t.Fatal("independent MySQL touched")
	}
	if _, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatal("custom resource not removed")
	}
}
func TestStopOnlyReleasesKnownFinalizersAfterNormalContainerExit(t *testing.T) {
	for _, exitCode := range []int32{0, 1} {
		client, item := lifecycleFixture(t)
		ctx := context.Background()
		now := metav1.Now()
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName + "-0", Namespace: item.Namespace, DeletionTimestamp: &now, Finalizers: []string{"mysql.oracle.com/membership", "kopf.zalando.org/KopfFinalizerMarker", "example.com/retain"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "mysql"}}}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "mysql", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: exitCode}}}}}}
		if _, err := client.Clientset.CoreV1().Pods(item.Namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, _, err := StopClusterStep(ctx, client, item); err != nil {
			t.Fatal(err)
		}
		current, _ := client.Clientset.CoreV1().Pods(item.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		want := 3
		if exitCode == 0 {
			want = 1
		}
		if len(current.Finalizers) != want || current.Finalizers[len(current.Finalizers)-1] != "example.com/retain" {
			t.Fatalf("exit=%d finalizers=%v", exitCode, current.Finalizers)
		}
	}
}
func TestOfflineRetireRefusesUnknownFinalizers(t *testing.T) {
	client, item := lifecycleFixture(t)
	ctx := context.Background()
	resource := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace)
	current, _ := resource.Get(ctx, item.ResourceName, metav1.GetOptions{})
	now := metav1.Now()
	current.SetDeletionTimestamp(&now)
	current.SetFinalizers([]string{"example.com/protect"})
	if _, err := resource.Update(ctx, current, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := finalizeStoppedCluster(ctx, client, item); err == nil {
		t.Fatal("unknown finalizer stripped")
	}
	current, _ = resource.Get(ctx, item.ResourceName, metav1.GetOptions{})
	if len(current.GetFinalizers()) != 1 {
		t.Fatal("unknown finalizer changed")
	}
}

func TestStoppedDetectionWaitsForControllerToActuallyExit(t *testing.T) {
	client, item := lifecycleFixture(t)
	ctx := context.Background()
	operator := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator-abc-123", Namespace: "mysql-operator"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "operator"}}}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "operator", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}}}}
	client.Clientset.CoreV1().Pods(operator.Namespace).Create(ctx, operator, metav1.CreateOptions{})
	stopped, err := ClusterStopped(ctx, client, item)
	if err != nil || stopped {
		t.Fatalf("controller still running, stopped=%v err=%v", stopped, err)
	}
}

func TestOfflineRetireAcceptsOracleAndKopfClusterFinalizers(t *testing.T) {
	for _, finalizers := range [][]string{
		{"mysql.oracle.com/cluster"},
		{"kopf.zalando.org/KopfFinalizerMarker"},
		{"mysql.oracle.com/cluster", "kopf.zalando.org/KopfFinalizerMarker"},
	} {
		t.Run(strings.Join(finalizers, "+"), func(t *testing.T) {
			client, item := lifecycleFixture(t)
			ctx := context.Background()
			resource := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace)
			current, err := resource.Get(ctx, item.ResourceName, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			now := metav1.Now()
			current.SetDeletionTimestamp(&now)
			current.SetFinalizers(finalizers)
			if _, err := resource.Update(ctx, current, metav1.UpdateOptions{}); err != nil {
				t.Fatal(err)
			}
			if err := finalizeStoppedCluster(ctx, client, item); err != nil {
				t.Fatal(err)
			}
			current, err = resource.Get(ctx, item.ResourceName, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(current.GetFinalizers()) != 0 {
				t.Fatalf("known cluster finalizers retained: %v", current.GetFinalizers())
			}
			if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Get(ctx, "datadir-"+item.ResourceName+"-0", metav1.GetOptions{}); err != nil {
				t.Fatal("finalization deleted a data volume")
			}
		})
	}
}

func TestOfflineRetireAllowsWorkloadsAlreadyRemoved(t *testing.T) {
	client, item := lifecycleFixture(t)
	ctx := context.Background()
	resource := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace)
	current, _ := resource.Get(ctx, item.ResourceName, metav1.GetOptions{})
	now := metav1.Now()
	current.SetDeletionTimestamp(&now)
	current.SetFinalizers([]string{"mysql.oracle.com/cluster"})
	if _, err := resource.Update(ctx, current, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := client.Clientset.AppsV1().StatefulSets(item.Namespace).Delete(ctx, item.ResourceName, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := client.Clientset.AppsV1().Deployments(item.Namespace).Delete(ctx, item.ResourceName+"-router", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := finalizeStoppedCluster(ctx, client, item); err != nil {
		t.Fatalf("partially completed cleanup cannot resume: %v", err)
	}
}

func TestOfflineRetirePreservesFinalizersWhenUnsafe(t *testing.T) {
	cases := []string{"not-deleting", "unmanaged", "unknown-finalizer", "database-pod", "operator-pod", "operator-running", "active-statefulset", "labelled-deployment", "other-cluster"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			client, item := lifecycleFixture(t)
			ctx := context.Background()
			resource := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace)
			current, _ := resource.Get(ctx, item.ResourceName, metav1.GetOptions{})
			now := metav1.Now()
			current.SetDeletionTimestamp(&now)
			finalizers := []string{"mysql.oracle.com/cluster", "kopf.zalando.org/KopfFinalizerMarker"}
			one := int32(1)
			switch name {
			case "not-deleting":
				current.SetDeletionTimestamp(nil)
			case "unmanaged":
				current.SetLabels(nil)
			case "unknown-finalizer":
				finalizers = append(finalizers, "example.com/protect")
			case "database-pod":
				_, err := client.Clientset.CoreV1().Pods(item.Namespace).Create(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName + "-0", Namespace: item.Namespace}}, metav1.CreateOptions{})
				if err != nil {
					t.Fatal(err)
				}
			case "operator-pod":
				_, err := client.Clientset.CoreV1().Pods("mysql-operator").Create(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator-abc-123", Namespace: "mysql-operator"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "operator"}}}}, metav1.CreateOptions{})
				if err != nil {
					t.Fatal(err)
				}
			case "operator-running":
				operator, err := operatorDeployment(ctx, client)
				if err != nil {
					t.Fatal(err)
				}
				operator.Spec.Replicas = &one
				if _, err := client.Clientset.AppsV1().Deployments(operator.Namespace).Update(ctx, operator, metav1.UpdateOptions{}); err != nil {
					t.Fatal(err)
				}
			case "active-statefulset":
				_, err := client.Clientset.AppsV1().StatefulSets(item.Namespace).Create(ctx, &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName + "-read-replica", Namespace: item.Namespace}, Spec: appsv1.StatefulSetSpec{Replicas: &one}}, metav1.CreateOptions{})
				if err != nil {
					t.Fatal(err)
				}
			case "labelled-deployment":
				_, err := client.Clientset.AppsV1().Deployments(item.Namespace).Create(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "custom-router", Namespace: item.Namespace, Labels: map[string]string{"mysql.oracle.com/cluster": item.ResourceName}}, Spec: appsv1.DeploymentSpec{Replicas: &one}}, metav1.CreateOptions{})
				if err != nil {
					t.Fatal(err)
				}
			case "other-cluster":
				other := *item
				other.ResourceName = "other-cluster"
				if _, err := resource.Create(ctx, ClusterObject(&other), metav1.CreateOptions{}); err != nil {
					t.Fatal(err)
				}
			}
			current.SetFinalizers(finalizers)
			if _, err := resource.Update(ctx, current, metav1.UpdateOptions{}); err != nil {
				t.Fatal(err)
			}
			if err := finalizeStoppedCluster(ctx, client, item); err == nil {
				t.Fatal("unsafe finalizer removal allowed")
			}
			current, err := resource.Get(ctx, item.ResourceName, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(current.GetFinalizers(), finalizers) {
				t.Fatalf("finalizers changed on rejected cleanup: %v", current.GetFinalizers())
			}
		})
	}
}
