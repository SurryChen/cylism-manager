package cloud

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	database "github.com/cylism/cylism-manager/internal/service/database"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func lifecycleHandler(t *testing.T, status string, live bool) (*DatabaseHandler, *model.MySQLCluster) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s, err := store.New(fmt.Sprintf("file:lifecycle-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	item := &model.MySQLCluster{Name: "old", ClusterID: 1, Namespace: database.Namespace, ResourceName: "cylism-mysql-cluster-1", Status: status}
	if err := s.DB().Create(item).Error; err != nil {
		t.Fatal(err)
	}
	zero := int32(0)
	objects := []runtime.Object{&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator", Namespace: "mysql-operator"}, Spec: appsv1.DeploymentSpec{Replicas: &zero}}, &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "datadir-" + item.ResourceName + "-0", Namespace: item.Namespace}}}
	dynamicObjects := []runtime.Object{}
	if live {
		objects = append(objects, &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName, Namespace: item.Namespace}, Spec: appsv1.StatefulSetSpec{Replicas: &zero}})
		dynamicObjects = append(dynamicObjects, database.ClusterObject(item))
	}
	gvr := schema.GroupVersionResource{Group: "mysql.oracle.com", Version: "v2", Resource: "innodbclusters"}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(objects...), DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "InnoDBClusterList"}, dynamicObjects...)}
	return &DatabaseHandler{store: s, client: func(context.Context, uint) (*k8s.Client, error) { return client, nil }}, item
}
func deleteClusterRequest(h *DatabaseHandler) *httptest.ResponseRecorder {
	router := gin.New()
	router.DELETE("/:id", h.DeleteCluster)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/1", nil))
	return response
}
func TestDeleteClusterRequiresResourceRemovalAndRetainsPVC(t *testing.T) {
	h, item := lifecycleHandler(t, "stopped", true)
	if response := deleteClusterRequest(h); response.Code == http.StatusOK {
		t.Fatal("record deleted while custom resource exists")
	}
	if err := h.store.DB().Model(item).Update("status", "retired").Error; err != nil {
		t.Fatal(err)
	}
	client, _ := h.client(context.Background(), 1)
	for i := 0; i < 4; i++ {
		if _, _, err := database.RetireClusterStep(context.Background(), client, item); err != nil {
			t.Fatal(err)
		}
	}
	task := &model.MySQLClusterTask{ClusterID: item.ID, Kind: "retire", Status: "success"}
	h.store.DB().Create(task)
	h.store.DB().Create(&model.MySQLClusterTaskLog{TaskID: task.ID, Message: "retired"})
	if response := deleteClusterRequest(h); response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	var count int64
	h.store.DB().Model(&model.MySQLCluster{}).Count(&count)
	if count != 0 {
		t.Fatal("record retained")
	}
	h.store.DB().Model(&model.MySQLClusterTaskLog{}).Count(&count)
	if count != 0 {
		t.Fatal("orphan logs retained")
	}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Get(context.Background(), "datadir-"+item.ResourceName+"-0", metav1.GetOptions{}); err != nil {
		t.Fatal("PVC removed")
	}
}
func TestDeleteClusterBlocksRunningTasksAndUnreachableKubernetes(t *testing.T) {
	h, item := lifecycleHandler(t, "retired", false)
	task := &model.MySQLClusterTask{ClusterID: item.ID, Kind: "retire", Status: "running"}
	h.store.DB().Create(task)
	if response := deleteClusterRequest(h); response.Code == http.StatusOK {
		t.Fatal("deleted active task")
	}
	h.store.DB().Model(task).Update("status", "success")
	h.client = func(context.Context, uint) (*k8s.Client, error) { return nil, errors.New("unreachable") }
	if response := deleteClusterRequest(h); response.Code == http.StatusOK {
		t.Fatal("failed open on inaccessible Kubernetes")
	}
}
func TestInspectAndListAdoptManuallyStoppedCluster(t *testing.T) {
	h, item := lifecycleHandler(t, "ready", true)
	router := gin.New()
	router.GET("/:id", h.InspectCluster)
	router.GET("/", h.ListClusters)
	for _, path := range []string{"/1", "/"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"stopped"`) {
			t.Fatalf("%s: %s", path, response.Body.String())
		}
	}
	h.store.DB().First(item, item.ID)
	if item.Status != "stopped" {
		t.Fatal("status not persisted")
	}
}
func TestLifecycleConfirmationAndConcurrentTaskGuard(t *testing.T) {
	h, item := lifecycleHandler(t, "stopped", true)
	h.store.DB().Create(&model.MySQLClusterTask{ClusterID: item.ID, Kind: "stop", Status: "running"})
	router := gin.New()
	router.POST("/:id/lifecycle", h.ClusterLifecycle)
	for _, body := range []string{`{"action":"resume","confirm_resource_name":"wrong"}`, `{"action":"resume","confirm_resource_name":"cylism-mysql-cluster-1"}`} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/1/lifecycle", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)
		if response.Code == http.StatusOK {
			t.Fatal("confirmation or active task guard bypassed")
		}
	}
}
func TestLifecycleRecoveryResumesPersistedTaskWithoutBrowser(t *testing.T) {
	h, item := lifecycleHandler(t, "stopping", true)
	task := &model.MySQLClusterTask{ClusterID: item.ID, Kind: "stop", Status: "running"}
	h.store.DB().Create(task)
	h.recoverClusterLifecycles()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var current model.MySQLClusterTask
		h.store.DB().First(&current, task.ID)
		if current.Status == "success" {
			h.store.DB().First(item, item.ID)
			if item.Status != "stopped" {
				t.Fatal("task completed without stopped state")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("persisted lifecycle task did not recover")
}
func TestInspectKeepsRecordVisibleWhenKubernetesUnavailable(t *testing.T) {
	h, _ := lifecycleHandler(t, "ready", true)
	h.client = func(context.Context, uint) (*k8s.Client, error) { return nil, errors.New("network unavailable") }
	router := gin.New()
	router.GET("/:id", h.InspectCluster)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/1", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "network unavailable") {
		t.Fatal(response.Body.String())
	}
}

func TestResumeTaskWaitsForActualReadyPods(t *testing.T) {
	h, item := lifecycleHandler(t, "resuming", true)
	h.store.DB().Model(item).Update("database_instances", 1)
	task := &model.MySQLClusterTask{ClusterID: item.ID, Kind: "resume", Status: "running"}
	h.store.DB().Create(task)
	client, _ := h.client(context.Background(), 1)
	operator, _ := client.Clientset.AppsV1().Deployments("mysql-operator").Get(context.Background(), "mysql-operator", metav1.GetOptions{})
	operator.Status.ReadyReplicas = 1
	client.Clientset.AppsV1().Deployments("mysql-operator").UpdateStatus(context.Background(), operator, metav1.UpdateOptions{})
	set, _ := client.Clientset.AppsV1().StatefulSets(item.Namespace).Get(context.Background(), item.ResourceName, metav1.GetOptions{})
	set.Status.ReadyReplicas = 1
	client.Clientset.AppsV1().StatefulSets(item.Namespace).UpdateStatus(context.Background(), set, metav1.UpdateOptions{})
	if h.clusterLifecycleStep(context.Background(), item.ID) {
		t.Fatal("resume trusted stale ready counters without live Pods")
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName + "-0", Namespace: item.Namespace, Labels: map[string]string{"component": "mysqld"}}, Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}
	client.Clientset.CoreV1().Pods(item.Namespace).Create(context.Background(), pod, metav1.CreateOptions{})
	if !h.clusterLifecycleStep(context.Background(), item.ID) {
		t.Fatal("resume did not finish after actual Pod became ready")
	}
	h.store.DB().First(task, task.ID)
	if task.Status != "success" {
		t.Fatalf("task=%+v", task)
	}
}
