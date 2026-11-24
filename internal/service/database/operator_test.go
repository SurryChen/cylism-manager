package database

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestClusterObjectUsesSeparateNodeGroupsAndStorage(t *testing.T) {
	item := &model.MySQLCluster{ResourceName: "cylism-mysql-cluster-7", Namespace: Namespace, DatabaseNodes: []string{"db-a", "db-b", "db-c"}, RouterNodes: []string{"db-a", "db-b"}, StorageSize: "20Gi"}
	object := ClusterObject(item)
	instances, found, err := unstructured.NestedInt64(object.Object, "spec", "instances")
	if err != nil || !found || instances != 3 {
		t.Fatalf("database replica count = %d, found=%v, err=%v", instances, found, err)
	}
	routers, _, _ := unstructured.NestedInt64(object.Object, "spec", "router", "instances")
	if routers != 2 {
		t.Fatalf("router replica count = %d", routers)
	}
	claim, _, _ := unstructured.NestedMap(object.Object, "spec", "datadirVolumeClaimTemplate")
	if _, exists := claim["storageClassName"]; exists {
		t.Fatal("empty storage class must use cluster default")
	}
	for _, path := range [][]string{{"spec", "podSpec", "affinity", "nodeAffinity", "requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms"}, {"spec", "router", "podSpec", "affinity", "nodeAffinity", "requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms"}} {
		terms, found, err := unstructured.NestedSlice(object.Object, path...)
		expected := 3
		if path[1] == "router" {
			expected = 2
		}
		if err != nil || !found || len(terms) != expected {
			t.Fatalf("missing placement %v: %v", path, err)
		}
		for _, term := range terms {
			fields := term.(map[string]any)["matchFields"].([]any)
			values := fields[0].(map[string]any)["values"].([]any)
			if len(values) != 1 {
				t.Fatalf("node name field selector must have one value: %v", values)
			}
		}
	}
	anti, _, _ := unstructured.NestedSlice(object.Object, "spec", "podSpec", "affinity", "podAntiAffinity", "preferredDuringSchedulingIgnoredDuringExecution")
	labels := anti[0].(map[string]any)["podAffinityTerm"].(map[string]any)["labelSelector"].(map[string]any)["matchLabels"].(map[string]any)
	if labels["component"] != "mysqld" {
		t.Fatalf("database anti-affinity selector = %v", labels)
	}
	tolerations, _, _ := unstructured.NestedSlice(object.Object, "spec", "podSpec", "tolerations")
	if len(tolerations) != 1 || tolerations[0].(map[string]any)["key"] != databaseNoScheduleTaintKey || tolerations[0].(map[string]any)["effect"] != "NoSchedule" || tolerations[0].(map[string]any)["value"] != "true" {
		t.Fatalf("database scheduling exception is incorrect: %v", tolerations)
	}
	if _, found, _ := unstructured.NestedSlice(object.Object, "spec", "router", "podSpec", "tolerations"); found {
		t.Fatal("Router must not receive database-only tolerations")
	}
}

func TestClusterObjectAllowsThreeInstancesOnTwoNodes(t *testing.T) {
	item := &model.MySQLCluster{ResourceName: "cylism-mysql-cluster-8", Namespace: Namespace, DatabaseNodes: []string{"db-a", "db-b"}, DatabaseInstances: 3, RouterNodes: []string{"db-a", "db-b"}, StorageSize: "10Gi"}
	object := ClusterObject(item)
	instances, _, err := unstructured.NestedInt64(object.Object, "spec", "instances")
	if err != nil || instances != 3 {
		t.Fatalf("database replica count = %d, err=%v", instances, err)
	}
	if _, found, _ := unstructured.NestedSlice(object.Object, "spec", "podSpec", "affinity", "podAntiAffinity", "requiredDuringSchedulingIgnoredDuringExecution"); found {
		t.Fatal("required anti-affinity prevents two instances sharing a node")
	}
	if preferred, found, err := unstructured.NestedSlice(object.Object, "spec", "podSpec", "affinity", "podAntiAffinity", "preferredDuringSchedulingIgnoredDuringExecution"); err != nil || !found || len(preferred) != 1 {
		t.Fatalf("database pods should prefer separate nodes: %v, %v", preferred, err)
	}
	if err := ValidateDatabaseInstances(3, item.DatabaseNodes); err != nil {
		t.Fatalf("two-node placement rejected: %v", err)
	}
	if err := ValidateDatabaseInstances(1, item.DatabaseNodes); err != nil {
		t.Fatalf("single-member InnoDB Cluster rejected: %v", err)
	}
	if err := ValidateDatabaseInstances(0, item.DatabaseNodes); err == nil {
		t.Fatal("zero-member InnoDB Cluster must be rejected")
	}
}

func TestClusterObjectAllowsNoInitialRouter(t *testing.T) {
	zero := 0
	item := &model.MySQLCluster{ResourceName: "mysql-one", Namespace: Namespace, DatabaseNodes: []string{"db-a"}, DatabaseInstances: 1, RouterNodes: []string{}, RouterInstances: &zero, StorageSize: "10Gi"}
	object := ClusterObject(item)
	instances, _, _ := unstructured.NestedInt64(object.Object, "spec", "instances")
	routers, _, _ := unstructured.NestedInt64(object.Object, "spec", "router", "instances")
	if instances != 1 || routers != 0 {
		t.Fatalf("initial counts = database %d, router %d", instances, routers)
	}
	if _, found, _ := unstructured.NestedMap(object.Object, "spec", "router", "podSpec"); found {
		t.Fatal("Router placement should be set when Router is deployed")
	}
	item.RouterNodes = []string{"db-a"}
	item.RouterInstances = nil
	if count := RouterInstanceCount(item); count != 1 {
		t.Fatalf("legacy router count = %d", count)
	}
}

func TestClusterObjectIncludesMySQLResourceLimits(t *testing.T) {
	item := &model.MySQLCluster{ResourceName: "mysql-small", Namespace: Namespace, DatabaseNodes: []string{"db-a"}, StorageSize: "10Gi", CPULimit: "1", MemoryLimit: "1Gi"}
	object := ClusterObject(item)
	containers, found, err := unstructured.NestedSlice(object.Object, "spec", "podSpec", "containers")
	if err != nil || !found || len(containers) != 1 {
		t.Fatalf("pod containers = %v, found=%v, err=%v", containers, found, err)
	}
	resources := containers[0].(map[string]any)["resources"].(map[string]any)
	for _, kind := range []string{"requests", "limits"} {
		values := resources[kind].(map[string]any)
		if values["cpu"] != "1" || values["memory"] != "1Gi" {
			t.Fatalf("%s = %v", kind, values)
		}
	}
	config, _, _ := unstructured.NestedString(object.Object, "spec", "mycnf")
	if !strings.Contains(config, "innodb_buffer_pool_size=256M") {
		t.Fatalf("my.cnf = %q", config)
	}
}

func TestScaleClusterCreatesRouterPlacementOnFirstRouter(t *testing.T) {
	zero := 0
	item := &model.MySQLCluster{ResourceName: "mysql-router-later", Namespace: Namespace, DatabaseNodes: []string{"db-a"}, DatabaseInstances: 1, RouterInstances: &zero, StorageSize: "10Gi"}
	client := &k8s.Client{DynamicClient: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), ClusterObject(item))}
	item.RouterNodes = []string{"db-b"}
	if err := ScaleCluster(context.Background(), client, item, "router", 0); err != nil {
		t.Fatal(err)
	}
	updated, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(Namespace).Get(context.Background(), item.ResourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	count, _, _ := unstructured.NestedInt64(updated.Object, "spec", "router", "instances")
	terms, found, err := unstructured.NestedSlice(updated.Object, "spec", "router", "podSpec", "affinity", "nodeAffinity", "requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms")
	if count != 1 || err != nil || !found || len(terms) != 1 {
		t.Fatalf("Router count=%d placement=%v found=%v err=%v", count, terms, found, err)
	}
}

func TestScaleClusterPreservesPlacementAndStorage(t *testing.T) {
	zero := 0
	item := &model.MySQLCluster{ResourceName: "mysql-scale", Namespace: Namespace, DatabaseNodes: []string{"db-a", "db-b"}, DatabaseInstances: 1, RouterNodes: []string{"db-a"}, RouterInstances: &zero, StorageClass: "local-path", StorageSize: "10Gi"}
	original := ClusterObject(item)
	client := &k8s.Client{DynamicClient: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), original)}
	for _, component := range []string{"database", "router"} {
		if err := ScaleCluster(context.Background(), client, item, component, map[string]int{"database": 1, "router": 0}[component]); err != nil {
			t.Fatalf("scale %s: %v", component, err)
		}
	}
	updated, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(Namespace).Get(context.Background(), item.ResourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	instances, _, _ := unstructured.NestedInt64(updated.Object, "spec", "instances")
	routers, _, _ := unstructured.NestedInt64(updated.Object, "spec", "router", "instances")
	if instances != 2 || routers != 1 {
		t.Fatalf("scaled counts = database %d, router %d", instances, routers)
	}
	for _, field := range []string{"podSpec", "datadirVolumeClaimTemplate"} {
		before, _, _ := unstructured.NestedMap(original.Object, "spec", field)
		after, _, _ := unstructured.NestedMap(updated.Object, "spec", field)
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("%s changed during scaling", field)
		}
	}
	beforeRouter, _, _ := unstructured.NestedMap(original.Object, "spec", "router", "podSpec")
	afterRouter, _, _ := unstructured.NestedMap(updated.Object, "spec", "router", "podSpec")
	if !reflect.DeepEqual(beforeRouter, afterRouter) {
		t.Fatal("router placement changed during scaling")
	}
	if err := ScaleCluster(context.Background(), client, item, "database", 1); err == nil {
		t.Fatal("stale database count accepted")
	}
}

func TestScaleClusterRejectsLegacyStrictAntiAffinity(t *testing.T) {
	item := &model.MySQLCluster{ResourceName: "mysql-strict", Namespace: Namespace, DatabaseNodes: []string{"db-a"}, DatabaseInstances: 1, RouterNodes: []string{"db-a"}, StorageSize: "10Gi"}
	object := ClusterObject(item)
	affinity, _, _ := unstructured.NestedMap(object.Object, "spec", "podSpec", "affinity", "podAntiAffinity")
	affinity["requiredDuringSchedulingIgnoredDuringExecution"] = affinity["preferredDuringSchedulingIgnoredDuringExecution"]
	delete(affinity, "preferredDuringSchedulingIgnoredDuringExecution")
	if err := unstructured.SetNestedMap(object.Object, affinity, "spec", "podSpec", "affinity", "podAntiAffinity"); err != nil {
		t.Fatal(err)
	}
	client := &k8s.Client{DynamicClient: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), object)}
	if err := ScaleCluster(context.Background(), client, item, "database", 1); err == nil {
		t.Fatal("legacy strict anti-affinity cluster should not be scaled past its candidate nodes")
	}
}

func TestValidateClusterNodesRejectsUnschedulableNodes(t *testing.T) {
	ready := corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue}
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "db-a"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{ready}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "db-b"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{ready}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "db-c"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{ready}}},
	}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(&nodes[0], &nodes[1], &nodes[2])}
	if err := ValidateClusterNodes(context.Background(), client, []string{"db-a", "db-b", "db-c"}, []string{"db-a", "db-b"}); err != nil {
		t.Fatalf("valid topology rejected: %v", err)
	}
	nodes[2].Spec.Taints = []corev1.Taint{{Key: "dedicated", Effect: corev1.TaintEffectNoSchedule}}
	if _, err := client.Clientset.CoreV1().Nodes().Update(context.Background(), &nodes[2], metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateClusterNodes(context.Background(), client, []string{"db-a", "db-b", "db-c"}, []string{"db-a", "db-b"}); err == nil {
		t.Fatal("tainted node must be rejected")
	}
}
