package cloud

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	database "github.com/cylism/cylism-manager/internal/service/database"
	"github.com/gin-gonic/gin"
)

func TestFirstRouterDeploymentSavesNodesAndStartsTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:mysql-first-router-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	zero := 0
	item := model.MySQLCluster{Name: "router-later", ClusterID: 1, Namespace: database.Namespace, ResourceName: "mysql-router-later", DatabaseNodes: []string{"db-a"}, DatabaseInstances: 1, RouterNodes: []string{}, RouterInstances: &zero, StorageSize: "10Gi", Status: "ready"}
	if err := s.DB().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	ready := corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue}
	zeroReplicas := int32(0)
	objects := []runtime.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "db-a"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{ready}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "router-a"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{ready}}},
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName, Namespace: item.Namespace}, Status: appsv1.StatefulSetStatus{ReadyReplicas: 1}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName + "-router", Namespace: item.Namespace}, Spec: appsv1.DeploymentSpec{Replicas: &zeroReplicas, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"mysql.oracle.com/cluster": item.ResourceName}}}}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator", Namespace: "mysql-operator"}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1}},
	}
	cr := database.ClusterObject(&item)
	gvr := schema.GroupVersionResource{Group: "mysql.oracle.com", Version: "v2", Resource: "innodbclusters"}
	client := &k8s.Client{
		Clientset:     k8sfake.NewSimpleClientset(objects...),
		DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "InnoDBClusterList"}, cr),
	}
	h := NewDatabaseHandler(s, bytes.Repeat([]byte("a"), 32), func(context.Context, uint) (*k8s.Client, error) { return client, nil })
	router := gin.New()
	router.POST("/clusters/:id/routers", h.AddMySQLRouter)
	request := httptest.NewRequest(http.MethodPost, "/clusters/1/routers", strings.NewReader(`{"router_nodes":["router-a"]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code == http.StatusOK || !strings.Contains(response.Body.String(), "不在资源池中") {
		t.Fatalf("Router node outside resource pool accepted: %s", response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/clusters/1/routers", strings.NewReader(`{"router_nodes":["db-a"]}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("first Router deployment: %s", response.Body.String())
	}
	var saved model.MySQLCluster
	if err := s.DB().First(&saved, item.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Status != "starting" || len(saved.RouterNodes) != 1 || saved.RouterNodes[0] != "db-a" || saved.RouterInstances == nil || *saved.RouterInstances != 1 {
		t.Fatalf("saved Router deployment = %+v", saved)
	}
	updated, err := client.DynamicClient.Resource(gvr).Namespace(item.Namespace).Get(context.Background(), item.ResourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	count, _, _ := unstructured.NestedInt64(updated.Object, "spec", "router", "instances")
	if count != 1 {
		t.Fatalf("CR Router count = %d", count)
	}
	selectedDeployment, err := client.Clientset.AppsV1().Deployments(item.Namespace).Get(context.Background(), item.ResourceName+"-router", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	selectedTerms := selectedDeployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(selectedTerms) != 1 || selectedTerms[0].MatchFields[0].Values[0] != "db-a" {
		t.Fatalf("existing Router Deployment ignored selected node: %+v", selectedDeployment.Spec.Template.Spec.Affinity)
	}
	var tasks []model.MySQLClusterTask
	if err := s.DB().Where("cluster_id = ?", item.ID).Find(&tasks).Error; err != nil || len(tasks) != 1 || tasks[0].Kind != "scale_router" {
		t.Fatalf("Router task = %+v, err=%v", tasks, err)
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/clusters/1/routers", strings.NewReader(`{"router_nodes":["router-a"]}`)))
	if response.Code == http.StatusOK {
		t.Fatal("concurrent Router deployment accepted")
	}
}

func TestRetireRouterWaitsForPodExitAndKeepsMySQL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:mysql-retire-router-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	one := 1
	item := model.MySQLCluster{Name: "router-retire", ClusterID: 1, Namespace: database.Namespace, ResourceName: "mysql-router-retire", DatabaseNodes: []string{"db-a"}, DatabaseInstances: 1, RouterNodes: []string{"db-a"}, RouterInstances: &one, StorageSize: "10Gi", Status: "ready"}
	if err := s.DB().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	replicas := int32(1)
	routerName := item.ResourceName + "-router"
	client := &k8s.Client{
		Clientset: k8sfake.NewSimpleClientset(
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator", Namespace: "mysql-operator"}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1}},
			&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName, Namespace: item.Namespace}, Status: appsv1.StatefulSetStatus{ReadyReplicas: 1}},
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: routerName, Namespace: item.Namespace}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"mysql.oracle.com/cluster": item.ResourceName}}}}, Status: appsv1.DeploymentStatus{Replicas: 1, ReadyReplicas: 1}},
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: routerName + "-abc", Namespace: item.Namespace, Labels: map[string]string{"mysql.oracle.com/cluster": item.ResourceName, "component": "mysqlrouter"}}},
			&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "datadir-" + item.ResourceName + "-0", Namespace: item.Namespace}},
		),
		DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{{Group: "mysql.oracle.com", Version: "v2", Resource: "innodbclusters"}: "InnoDBClusterList"}, database.ClusterObject(&item)),
	}
	h := NewDatabaseHandler(s, bytes.Repeat([]byte("a"), 32), func(context.Context, uint) (*k8s.Client, error) { return client, nil })
	router := gin.New()
	router.POST("/clusters/:id/routers/scale-down", h.RetireMySQLRouter)
	request := func() *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/clusters/1/routers/scale-down", nil))
		return response
	}
	if response := request(); response.Code != http.StatusOK {
		t.Fatalf("retire Router: %s", response.Body.String())
	}
	var saved model.MySQLCluster
	if err := s.DB().First(&saved, item.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.RouterInstances == nil || *saved.RouterInstances != 0 || saved.Status != "starting" || len(saved.RouterNodes) != 1 || saved.RouterNodes[0] != "db-a" {
		t.Fatalf("retired Router state = %+v", saved)
	}
	gvr := schema.GroupVersionResource{Group: "mysql.oracle.com", Version: "v2", Resource: "innodbclusters"}
	resource, err := client.DynamicClient.Resource(gvr).Namespace(item.Namespace).Get(context.Background(), item.ResourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	routerCount, _, _ := unstructured.NestedInt64(resource.Object, "spec", "router", "instances")
	databaseCount, _, _ := unstructured.NestedInt64(resource.Object, "spec", "instances")
	if routerCount != 0 || databaseCount != 1 {
		t.Fatalf("Operator target counts: MySQL=%d Router=%d", databaseCount, routerCount)
	}
	if response := request(); response.Code == http.StatusOK {
		t.Fatal("concurrent Router retirement accepted")
	}
	if _, err := h.syncMySQLTask(context.Background(), &saved); err != nil || saved.Status != "starting" {
		t.Fatalf("retirement finished before Router exited: status=%s err=%v", saved.Status, err)
	}
	deployment, err := client.Clientset.AppsV1().Deployments(item.Namespace).Get(context.Background(), routerName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	zero := int32(0)
	deployment.Spec.Replicas = &zero
	deployment.Status.Replicas, deployment.Status.ReadyReplicas = 0, 0
	if _, err := client.Clientset.AppsV1().Deployments(item.Namespace).Update(context.Background(), deployment, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := client.Clientset.CoreV1().Pods(item.Namespace).Delete(context.Background(), routerName+"-abc", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.syncMySQLTask(context.Background(), &saved); err != nil || saved.Status != "ready" {
		t.Fatalf("retirement did not complete: status=%s err=%v", saved.Status, err)
	}
	var task model.MySQLClusterTask
	if err := s.DB().Where("cluster_id = ?", item.ID).First(&task).Error; err != nil || task.Kind != "scale_router_down" || task.Status != "success" {
		t.Fatalf("retirement task = %+v, err=%v", task, err)
	}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Get(context.Background(), "datadir-"+item.ResourceName+"-0", metav1.GetOptions{}); err != nil {
		t.Fatalf("MySQL PVC was removed: %v", err)
	}
}

func TestAddMySQLInstanceUsesSelectedResourcePoolNodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:mysql-candidate-scale-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	zero := 0
	item := model.MySQLCluster{Name: "candidate-scale", ClusterID: 1, Namespace: database.Namespace, ResourceName: "mysql-candidate-scale", DatabaseNodes: []string{"node-a", "node-b"}, DatabaseInstances: 1, RouterInstances: &zero, StorageSize: "10Gi", Status: "ready"}
	if err := s.DB().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	ready := corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue}
	replicas := int32(1)
	affinity := &corev1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{
		{MatchFields: []corev1.NodeSelectorRequirement{{Key: "metadata.name", Operator: corev1.NodeSelectorOpIn, Values: []string{"node-a"}}}},
		{MatchFields: []corev1.NodeSelectorRequirement{{Key: "metadata.name", Operator: corev1.NodeSelectorOpIn, Values: []string{"node-b"}}}},
	}}}
	set := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName, Namespace: item.Namespace}, Spec: appsv1.StatefulSetSpec{Replicas: &replicas, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Affinity: &corev1.Affinity{NodeAffinity: affinity}}}}, Status: appsv1.StatefulSetStatus{ReadyReplicas: 1}}
	gvr := schema.GroupVersionResource{Group: "mysql.oracle.com", Version: "v2", Resource: "innodbclusters"}
	client := &k8s.Client{
		Clientset: k8sfake.NewSimpleClientset(
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{ready}}},
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-b"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{ready}}},
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator", Namespace: "mysql-operator"}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1}},
			set,
		),
		DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "InnoDBClusterList"}, database.ClusterObject(&item)),
	}
	h := NewDatabaseHandler(s, bytes.Repeat([]byte("a"), 32), func(context.Context, uint) (*k8s.Client, error) { return client, nil })
	router := gin.New()
	router.POST("/clusters/:id/instances", h.AddMySQLInstance)
	request := func(body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/clusters/1/instances", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, req)
		return response
	}
	if response := request(`{"candidate_nodes":["outside"]}`); response.Code == http.StatusOK {
		t.Fatalf("outside-pool node accepted: %s", response.Body.String())
	}
	if response := request(`{"candidate_nodes":["node-b"]}`); response.Code != http.StatusOK {
		t.Fatalf("scale with selected node failed: %s", response.Body.String())
	}
	selected, err := client.Clientset.AppsV1().StatefulSets(item.Namespace).Get(context.Background(), item.ResourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	terms := selected.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(terms) != 1 || terms[0].MatchFields[0].Values[0] != "node-b" || selected.Spec.UpdateStrategy.RollingUpdate == nil || *selected.Spec.UpdateStrategy.RollingUpdate.Partition != 1 {
		t.Fatalf("new Pod placement or existing Pod partition incorrect: %+v", selected.Spec)
	}
	cluster, err := client.DynamicClient.Resource(gvr).Namespace(item.Namespace).Get(context.Background(), item.ResourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	count, _, _ := unstructured.NestedInt64(cluster.Object, "spec", "instances")
	if count != 2 {
		t.Fatalf("Operator target count = %d", count)
	}
	clusterTerms, _, _ := unstructured.NestedSlice(cluster.Object, "spec", "podSpec", "affinity", "nodeAffinity", "requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms")
	if len(clusterTerms) != 1 || !strings.Contains(fmt.Sprint(clusterTerms), "node-b") {
		t.Fatalf("Operator placement did not retain selected node: %+v", clusterTerms)
	}
}
