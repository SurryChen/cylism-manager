package database

import (
	"context"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestValidateMyCNFProtectsOperatorSettings(t *testing.T) {
	valid, err := ValidateMyCNF("[mysqld]\ninnodb_buffer_pool_size=128M\nperformance_schema_digests_size=1000")
	if err != nil || valid != "[mysqld]\ninnodb_buffer_pool_size=128M\nperformance_schema_digests_size=1000\n" {
		t.Fatalf("valid config = %q, %v", valid, err)
	}
	for _, input := range []string{"performance_schema=OFF", "[client]\nport=3306", "[mysqld]\ngroup_replication_start_on_boot=OFF", "[mysqld]\n!includedir /tmp", "[mysqld]\nmax_connections=20\nmax_connections=40"} {
		if _, err := ValidateMyCNF(input); err == nil {
			t.Errorf("accepted invalid config %q", input)
		}
	}
}

func TestMySQLResourceTuningForOneGiB(t *testing.T) {
	cpu, memory, err := ValidateMySQLResources("1", "1Gi")
	if err != nil || cpu != "1" || memory != "1Gi" {
		t.Fatalf("resource limits = %q, %q, %v", cpu, memory, err)
	}
	for _, pair := range [][2]string{{"", "1Gi"}, {"1", ""}, {"1", "512Mi"}, {"0", "1Gi"}, {"17", "1Gi"}} {
		if _, _, err := ValidateMySQLResources(pair[0], pair[1]); err == nil {
			t.Fatalf("accepted invalid limits %q / %q", pair[0], pair[1])
		}
	}
	config := EffectiveMyCNF("[mysqld]\ninnodb_buffer_pool_size=512M\nperformance_schema_max_sql_text_length=1024\n", memory)
	for _, option := range []string{"innodb_buffer_pool_size=256M", "max_connections=50", "sort_buffer_size=512K", "join_buffer_size=512K", "read_buffer_size=512K", "read_rnd_buffer_size=512K", "performance_schema_max_sql_text_length=1024"} {
		if !strings.Contains(config, option) {
			t.Fatalf("missing %s in %q", option, config)
		}
	}
	if strings.Contains(config, "innodb_buffer_pool_size=512M") {
		t.Fatalf("custom buffer pool overrode the memory profile: %q", config)
	}
	if _, err := ValidateMyCNF(config); err != nil {
		t.Fatalf("generated my.cnf is invalid: %v", err)
	}
}

func TestUpdateMySQLSettingsPreservesOtherContainers(t *testing.T) {
	ctx := context.Background()
	item := &model.MySQLCluster{ResourceName: "mysql-limits", Namespace: "mysql", DatabaseNodes: []string{"node-a"}, StorageSize: "10Gi"}
	object := ClusterObject(item)
	if err := unstructured.SetNestedSlice(object.Object, []any{
		map[string]any{"name": "sidecar", "resources": map[string]any{"limits": map[string]any{"memory": "128Mi"}}},
		map[string]any{"name": "mysql", "imagePullPolicy": "IfNotPresent"},
	}, "spec", "podSpec", "containers"); err != nil {
		t.Fatal(err)
	}
	client := &k8s.Client{DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{innoDBClusterGVR: "InnoDBClusterList"}, object)}
	if err := UpdateMySQLSettings(ctx, client, item, "", "1", "1Gi"); err != nil {
		t.Fatal(err)
	}
	updated, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	containers, _, _ := unstructured.NestedSlice(updated.Object, "spec", "podSpec", "containers")
	if len(containers) != 2 || containers[0].(map[string]any)["name"] != "sidecar" || containers[1].(map[string]any)["name"] != "mysql" {
		t.Fatalf("pod container overrides = %v", containers)
	}
	limits := containers[1].(map[string]any)["resources"].(map[string]any)["limits"].(map[string]any)
	if limits["cpu"] != "1" || limits["memory"] != "1Gi" {
		t.Fatalf("mysql limits = %v", limits)
	}
	if containers[1].(map[string]any)["imagePullPolicy"] != "IfNotPresent" {
		t.Fatal("existing MySQL container override was lost")
	}
	config, _, _ := unstructured.NestedString(updated.Object, "spec", "mycnf")
	if !strings.Contains(config, "innodb_buffer_pool_size=256M") {
		t.Fatalf("resource profile was not applied: %q", config)
	}
}

func TestMyCNFUpdateAndRestartPreserveCluster(t *testing.T) {
	ctx := context.Background()
	item := &model.MySQLCluster{ResourceName: "mysql-test", Namespace: "mysql", DatabaseNodes: []string{"node-a"}, DatabaseInstances: 1, StorageSize: "10Gi", Status: "ready"}
	object := ClusterObject(item)
	client := &k8s.Client{
		DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{innoDBClusterGVR: "InnoDBClusterList"}, object),
		Clientset: k8sfake.NewSimpleClientset(&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName, Namespace: item.Namespace}, Spec: appsv1.StatefulSetSpec{
			Replicas: int32ptr(1), Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "mysql"}, {Name: "sidecar"}}}},
		}}),
	}
	config := "[mysqld]\ninnodb_buffer_pool_size=128M\n"
	if err := UpdateMyCNF(ctx, client, item, config); err != nil {
		t.Fatal(err)
	}
	item.ConfigRestartRequired = true
	actual, pending, err := MyCNFState(ctx, client, item)
	if err != nil || actual != config || !pending {
		t.Fatalf("config state = %q, pending=%v, err=%v", actual, pending, err)
	}
	if err := RestartMySQLForConfig(ctx, client, item, config); err != nil {
		t.Fatal(err)
	}
	item.ConfigRestartRequired = false
	_, pending, err = MyCNFState(ctx, client, item)
	if err != nil || pending {
		t.Fatalf("restart still pending=%v, err=%v", pending, err)
	}
	if err := UpdateMyCNF(ctx, client, item, ""); err != nil {
		t.Fatal(err)
	}
	item.ConfigRestartRequired = true
	_, pending, err = MyCNFState(ctx, client, item)
	if err != nil || !pending {
		t.Fatalf("clearing config must require restart: pending=%v, err=%v", pending, err)
	}
	if _, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{}); err != nil {
		t.Fatalf("cluster resource was removed: %v", err)
	}
}

func TestRestartMySQLAppliesResourceLimitsToStatefulSet(t *testing.T) {
	ctx := context.Background()
	item := &model.MySQLCluster{ResourceName: "mysql-limits", Namespace: "mysql", DatabaseNodes: []string{"node-a"}, StorageSize: "10Gi", CPULimit: "1", MemoryLimit: "1Gi"}
	object := ClusterObject(item)
	original := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName, Namespace: item.Namespace}, Spec: appsv1.StatefulSetSpec{
		Replicas: int32ptr(1), Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "mysql"}, {Name: "sidecar"}}}},
	}}
	client := &k8s.Client{
		DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{innoDBClusterGVR: "InnoDBClusterList"}, object),
		Clientset:     k8sfake.NewSimpleClientset(original),
	}
	value := EffectiveMyCNF("", item.MemoryLimit)
	if err := RestartMySQLForConfig(ctx, client, item, value); err != nil {
		t.Fatal(err)
	}
	updated, err := client.Clientset.AppsV1().StatefulSets(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	container := updated.Spec.Template.Spec.Containers[0]
	if container.Resources.Limits.Cpu().String() != "1" || container.Resources.Limits.Memory().String() != "1Gi" {
		t.Fatalf("MySQL limits = %v", container.Resources.Limits)
	}
	if updated.Spec.Template.Spec.Containers[1].Resources.Limits != nil {
		t.Fatal("sidecar resources changed")
	}
	if updated.Spec.Template.Annotations[mysqlResourceManagedAnnotation] != "true" || updated.Spec.Template.Annotations[MyCNFRestartAnnotation] != MyCNFRevision(value) {
		t.Fatalf("restart annotations = %v", updated.Spec.Template.Annotations)
	}
	item.CPULimit, item.MemoryLimit = "", ""
	if err := RestartMySQLForConfig(ctx, client, item, ""); err != nil {
		t.Fatal(err)
	}
	updated, err = client.Clientset.AppsV1().StatefulSets(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Spec.Template.Spec.Containers[0].Resources.Limits) != 0 || len(updated.Spec.Template.Spec.Containers[0].Resources.Requests) != 0 {
		t.Fatalf("MySQL resources were not cleared: %v", updated.Spec.Template.Spec.Containers[0].Resources)
	}
}

func int32ptr(value int32) *int32 { return &value }
