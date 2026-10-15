package cloud

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	database "github.com/cylism/cylism-manager/internal/service/database"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestUpdateClusterResourcePoolValidatesAndPersistsNodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:mysql-resource-pool-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	item := model.MySQLCluster{Name: "pool-test", ClusterID: 1, Namespace: "cylism-databases", Status: "paused", DatabaseNodes: []string{"node-a"}, PreferredPrimaryNode: "node-a"}
	if err := s.DB().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	ready := func(name, ip string) *corev1.Node {
		return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: ip}}, Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}
	}
	blocked := ready("node-c", "10.0.0.3")
	blocked.Spec.Unschedulable = true
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(ready("node-a", "10.0.0.1"), ready("node-b", "10.0.0.2"), blocked)}
	h := NewDatabaseHandler(s, bytes.Repeat([]byte("a"), 32), func(context.Context, uint) (*k8s.Client, error) { return client, nil })
	router := gin.New()
	router.PUT("/clusters/:id/resource-pool", h.UpdateClusterResourcePool)
	router.POST("/clusters/:id/preflight", h.PreflightCluster)
	router.PUT("/clusters/:id/topology", h.DeployCluster)
	request := func(body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/clusters/1/resource-pool", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, req)
		return response
	}
	if response := request(`{"database_nodes":["node-c"]}`); response.Code == http.StatusOK {
		t.Fatalf("unschedulable node accepted: %s", response.Body.String())
	}
	if response := request(`{"database_nodes":["node-b"]}`); response.Code != http.StatusOK {
		t.Fatalf("save failed: %s", response.Body.String())
	}
	if err := s.DB().First(&item, item.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(item.DatabaseNodes) != 1 || item.DatabaseNodes[0] != "node-b" || item.PreferredPrimaryNode != "" {
		t.Fatalf("saved pool = %+v", item)
	}
	for _, methodAndPath := range [][2]string{{http.MethodPost, "/clusters/1/preflight"}, {http.MethodPut, "/clusters/1/topology"}} {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(methodAndPath[0], methodAndPath[1], strings.NewReader(`{"database_nodes":["node-a"],"database_instances":1}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, req)
		if response.Code == http.StatusOK || !strings.Contains(response.Body.String(), "资源池") {
			t.Fatalf("%s %s accepted outside pool: %s", methodAndPath[0], methodAndPath[1], response.Body.String())
		}
	}
	if err := s.DB().Model(&item).Update("status", "ready").Error; err != nil {
		t.Fatal(err)
	}
	if response := request(`{"database_nodes":["node-a"]}`); response.Code == http.StatusOK {
		t.Fatalf("running pool accepted: %s", response.Body.String())
	}
}

func TestSameNodePoolRejectsChanges(t *testing.T) {
	if !sameNodePool([]string{"a", "b"}, []string{"b", "a"}) || sameNodePool([]string{"a", "b"}, []string{"a", "c"}) || sameNodePool([]string{"a", "b"}, []string{"a", "a"}) {
		t.Fatal("resource pool equality mismatch")
	}
}

func TestDatabaseCandidatesRequireResourcePoolSubset(t *testing.T) {
	pool := []string{"node-a", "node-b"}
	for _, selection := range [][]string{{"outside"}, {"node-a", "node-a"}} {
		if _, err := databaseCandidates(pool, selection); err == nil {
			t.Fatalf("invalid candidate selection accepted: %v", selection)
		}
	}
	candidates, err := databaseCandidates(pool, []string{"node-b"})
	if err != nil || len(candidates) != 1 || candidates[0] != "node-b" {
		t.Fatalf("valid subset rejected: %v, %v", candidates, err)
	}
}

func TestUpdateRunningClusterResourcePoolUpdatesBothSchedulingTemplates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:mysql-running-pool-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	item := model.MySQLCluster{Name: "running-pool", ClusterID: 1, Namespace: database.Namespace, ResourceName: "mysql-running-pool", Status: "ready", DatabaseNodes: []string{"node-a"}, DatabaseInstances: 1, StorageSize: "10Gi"}
	if err := s.DB().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	ready := corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue}
	replicas := int32(1)
	set := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName, Namespace: item.Namespace}, Spec: appsv1.StatefulSetSpec{Replicas: &replicas, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Affinity: &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchFields: []corev1.NodeSelectorRequirement{{Key: "metadata.name", Operator: corev1.NodeSelectorOpIn, Values: []string{"node-a"}}}}}}}}}}}}
	gvr := schema.GroupVersionResource{Group: "mysql.oracle.com", Version: "v2", Resource: "innodbclusters"}
	dynamic := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "InnoDBClusterList"}, database.ClusterObject(&item))
	client := &k8s.Client{
		Clientset: k8sfake.NewSimpleClientset(
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{ready}}},
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-b"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{ready}}},
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName + "-0", Namespace: item.Namespace, Labels: map[string]string{"mysql.oracle.com/cluster": item.ResourceName, "component": "mysqld"}}, Spec: corev1.PodSpec{NodeName: "node-a"}},
			set,
		),
		DynamicClient: dynamic,
	}
	h := NewDatabaseHandler(s, bytes.Repeat([]byte("a"), 32), func(context.Context, uint) (*k8s.Client, error) { return client, nil })
	router := gin.New()
	router.PUT("/clusters/:id/resource-pool", h.UpdateClusterResourcePool)
	request := func(body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/clusters/1/resource-pool", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, req)
		return response
	}
	if response := request(`{"database_nodes":["node-a","node-b"]}`); response.Code != http.StatusOK {
		t.Fatalf("expansion failed: %s", response.Body.String())
	}
	if err := s.DB().First(&item, item.ID).Error; err != nil || len(item.DatabaseNodes) != 2 {
		t.Fatalf("saved resource pool = %+v, err=%v", item, err)
	}
	cluster, err := client.DynamicClient.Resource(gvr).Namespace(item.Namespace).Get(context.Background(), item.ResourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	terms, _, _ := unstructured.NestedSlice(cluster.Object, "spec", "podSpec", "affinity", "nodeAffinity", "requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms")
	if len(terms) != 2 {
		t.Fatalf("InnoDBCluster node terms = %+v", terms)
	}
	updatedSet, err := client.Clientset.AppsV1().StatefulSets(item.Namespace).Get(context.Background(), item.ResourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if updatedSet.Spec.UpdateStrategy.RollingUpdate == nil || updatedSet.Spec.UpdateStrategy.RollingUpdate.Partition == nil || *updatedSet.Spec.UpdateStrategy.RollingUpdate.Partition != 1 || len(updatedSet.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) != 2 {
		t.Fatalf("StatefulSet update could restart existing Pods or missed nodes: %+v", updatedSet.Spec)
	}
	rollbackCandidates, err := database.UpdateDatabaseCandidates(context.Background(), client, &item, []string{"node-b"})
	if err != nil {
		t.Fatalf("select next Pod candidate: %v", err)
	}
	selectedSet, err := client.Clientset.AppsV1().StatefulSets(item.Namespace).Get(context.Background(), item.ResourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	selectedTerms := selectedSet.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(selectedTerms) != 1 || selectedTerms[0].MatchFields[0].Values[0] != "node-b" || *selectedSet.Spec.UpdateStrategy.RollingUpdate.Partition != 1 {
		t.Fatalf("next Pod placement changed existing replicas or ignored selection: %+v", selectedSet.Spec)
	}
	if err := rollbackCandidates(); err != nil {
		t.Fatalf("rollback next Pod candidates: %v", err)
	}
	if response := request(`{"database_nodes":["node-b"]}`); response.Code == http.StatusOK || !strings.Contains(response.Body.String(), "仍承载 MySQL 实例") {
		t.Fatalf("occupied node removal accepted: %s", response.Body.String())
	}
	if err := s.DB().Model(&item).Update("router_nodes", `["node-b"]`).Error; err != nil {
		t.Fatal(err)
	}
	if response := request(`{"database_nodes":["node-a"]}`); response.Code == http.StatusOK || !strings.Contains(response.Body.String(), "Router 候选节点") {
		t.Fatalf("Router node removal accepted: %s", response.Body.String())
	}
	if err := s.DB().Model(&item).Update("router_nodes", `[]`).Error; err != nil {
		t.Fatal(err)
	}
	volume := &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-node-b"}, Spec: corev1.PersistentVolumeSpec{NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: corev1.LabelHostname, Operator: corev1.NodeSelectorOpIn, Values: []string{"node-b"}}}}}}}}}
	if _, err := client.Clientset.CoreV1().PersistentVolumes().Create(context.Background(), volume, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "datadir-" + item.ResourceName + "-1", Namespace: item.Namespace}, Spec: corev1.PersistentVolumeClaimSpec{VolumeName: volume.Name}}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Create(context.Background(), claim, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if response := request(`{"database_nodes":["node-a"]}`); response.Code == http.StatusOK || !strings.Contains(response.Body.String(), "数据卷") {
		t.Fatalf("node with bound local volume removed: %s", response.Body.String())
	}
	if err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Delete(context.Background(), claim.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	failUpdate := true
	dynamic.PrependReactor("update", "innodbclusters", func(k8stesting.Action) (bool, runtime.Object, error) {
		if failUpdate {
			failUpdate = false
			return true, nil, errors.New("operator resource update failed")
		}
		return false, nil, nil
	})
	if response := request(`{"database_nodes":["node-a"]}`); response.Code == http.StatusOK {
		t.Fatalf("failed Operator update accepted: %s", response.Body.String())
	}
	if err := s.DB().First(&item, item.ID).Error; err != nil || len(item.DatabaseNodes) != 2 {
		t.Fatalf("resource pool changed after failed update: %+v, err=%v", item, err)
	}
	rolledBackSet, err := client.Clientset.AppsV1().StatefulSets(item.Namespace).Get(context.Background(), item.ResourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rolledBackSet.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) != 2 {
		t.Fatalf("StatefulSet resource pool was not restored: %+v", rolledBackSet.Spec.Template.Spec.Affinity)
	}
	if response := request(`{"database_nodes":["node-a"]}`); response.Code != http.StatusOK {
		t.Fatalf("unused node removal failed: %s", response.Body.String())
	}
}

func TestDeployClusterUsesSavedResourcePoolAndClearsPreferredPrimary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:mysql-saved-pool-deploy-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	item := model.MySQLCluster{Name: "saved-pool", ClusterID: 1, Namespace: "cylism-databases", ResourceName: "mysql-saved-pool", Status: "draft", DatabaseNodes: []string{"node-a", "node-b"}, PreferredPrimaryNode: "node-a", PrimaryConfigured: true}
	if err := s.DB().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	gvr := schema.GroupVersionResource{Group: "mysql.oracle.com", Version: "v2", Resource: "innodbclusters"}
	client := &k8s.Client{
		Clientset: k8sfake.NewSimpleClientset(
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-b"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator", Namespace: "mysql-operator"}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1}},
			&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local-path", Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}}},
		),
		DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "InnoDBClusterList"}),
	}
	h := NewDatabaseHandler(s, bytes.Repeat([]byte("a"), 32), func(context.Context, uint) (*k8s.Client, error) { return client, nil })
	router := gin.New()
	router.POST("/clusters/:id/preflight", h.PreflightCluster)
	router.PUT("/clusters/:id/topology", h.DeployCluster)
	body := `{"database_instances":2,"storage_size":"20Gi","candidate_nodes":["node-b"]}`
	for _, methodAndPath := range [][2]string{{http.MethodPost, "/clusters/1/preflight"}, {http.MethodPut, "/clusters/1/topology"}} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(methodAndPath[0], methodAndPath[1], strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s %s: %s", methodAndPath[0], methodAndPath[1], response.Body.String())
		}
	}
	if err := s.DB().First(&item, item.ID).Error; err != nil {
		t.Fatal(err)
	}
	if item.DatabaseInstances != 2 || item.StorageSize != "20Gi" || item.StorageClass != "local-path" || len(item.DatabaseNodes) != 2 || len(item.RouterNodes) != 0 || item.RouterInstances == nil || *item.RouterInstances != 0 || item.PreferredPrimaryNode != "" || item.PrimaryConfigured {
		t.Fatalf("deployment configuration = %+v", item)
	}
	created, err := client.DynamicClient.Resource(gvr).Namespace(item.Namespace).Get(context.Background(), item.ResourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	instances, _, _ := unstructured.NestedInt64(created.Object, "spec", "instances")
	if instances != 2 {
		t.Fatalf("deployed instance count = %d", instances)
	}
	terms, _, _ := unstructured.NestedSlice(created.Object, "spec", "podSpec", "affinity", "nodeAffinity", "requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms")
	if len(terms) != 1 || !strings.Contains(fmt.Sprint(terms), "node-b") {
		t.Fatalf("initial deployment ignored selected candidate: %+v", terms)
	}
}
