package cloud

import (
	"bytes"
	"context"
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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestPauseClusterDeploymentUpdatesTaskAndKeepsConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:mysql-deployment-pause-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	item := model.MySQLCluster{Name: "test", ClusterID: 2, Namespace: "mysql", ResourceName: "mysql-test", Status: "starting", DatabaseNodes: []string{"a", "b", "c"}, StorageSize: "10Gi"}
	if err := s.DB().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	task := model.MySQLClusterTask{ClusterID: item.ID, Kind: "deploy", Status: "running"}
	if err := s.DB().Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(), DynamicClient: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())}
	h := NewDatabaseHandler(s, bytes.Repeat([]byte("a"), 32), func(context.Context, uint) (*k8s.Client, error) { return client, nil })
	router := gin.New()
	router.POST("/:id/pause", h.PauseClusterDeployment)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/1/pause", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("pause response: %s", response.Body.String())
	}
	if err := s.DB().First(&item, item.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DB().First(&task, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if item.Status != "paused" || task.Status != "paused" || item.StorageSize != "10Gi" || len(item.DatabaseNodes) != 3 {
		t.Fatalf("deployment configuration or task not preserved: item=%+v task=%+v", item, task)
	}
	var logs []model.MySQLClusterTaskLog
	if err := s.DB().Where("task_id = ?", task.ID).Find(&logs).Error; err != nil || len(logs) != 1 {
		t.Fatalf("pause log missing: logs=%+v err=%v", logs, err)
	}
}

func TestPauseScaleWithdrawsOnlyNewTarget(t *testing.T) {
	for _, kind := range []string{"scale_database", "scale_router"} {
		t.Run(kind, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			s, err := store.New("file:pause-" + kind + "?mode=memory&cache=shared")
			if err != nil {
				t.Fatal(err)
			}
			routerCount, dbCount := 0, 1
			if kind == "scale_database" {
				dbCount = 2
			} else {
				routerCount = 1
			}
			item := model.MySQLCluster{Name: "pause-" + kind, ClusterID: 2, Namespace: "mysql", ResourceName: "mysql-pause-" + kind, Status: "starting", DatabaseNodes: []string{"node-a"}, DatabaseInstances: dbCount, RouterNodes: []string{"node-a"}, RouterInstances: &routerCount, StorageSize: "10Gi"}
			if err := s.DB().Create(&item).Error; err != nil {
				t.Fatal(err)
			}
			task := model.MySQLClusterTask{ClusterID: item.ID, Kind: kind, Status: "running"}
			if err := s.DB().Create(&task).Error; err != nil {
				t.Fatal(err)
			}
			gvr := schema.GroupVersionResource{Group: "mysql.oracle.com", Version: "v2", Resource: "innodbclusters"}
			client := &k8s.Client{
				Clientset: k8sfake.NewSimpleClientset(
					&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName, Namespace: item.Namespace}, Status: appsv1.StatefulSetStatus{ReadyReplicas: 1}},
					&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "datadir-" + item.ResourceName + "-0", Namespace: item.Namespace}},
				),
				DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "InnoDBClusterList"}, database.ClusterObject(&item)),
			}
			h := NewDatabaseHandler(s, bytes.Repeat([]byte("a"), 32), func(context.Context, uint) (*k8s.Client, error) { return client, nil })
			router := gin.New()
			router.POST("/:id/pause", h.PauseClusterDeployment)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/1/pause", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("pause response: %s", response.Body.String())
			}
			if err := s.DB().First(&item, item.ID).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.DB().First(&task, task.ID).Error; err != nil {
				t.Fatal(err)
			}
			if item.Status != "starting" || item.DatabaseInstances != 1 || database.RouterInstanceCount(&item) != 0 || task.Status != "paused" {
				t.Fatalf("unexpected state after withdrawal: item=%+v task=%+v", item, task)
			}
			resource, err := client.DynamicClient.Resource(gvr).Namespace(item.Namespace).Get(context.Background(), item.ResourceName, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("cluster resource removed: %v", err)
			}
			instances, _, _ := unstructured.NestedInt64(resource.Object, "spec", "instances")
			routers, _, _ := unstructured.NestedInt64(resource.Object, "spec", "router", "instances")
			if instances != 1 || routers != 0 {
				t.Fatalf("operator targets = MySQL %d, Router %d", instances, routers)
			}
			if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Get(context.Background(), "datadir-"+item.ResourceName+"-0", metav1.GetOptions{}); err != nil {
				t.Fatalf("existing PVC removed: %v", err)
			}
		})
	}
}

func TestPauseScaleRejectsReadyInstanceAndCompletedTask(t *testing.T) {
	for _, tc := range []struct {
		name       string
		kind       string
		status     string
		cluster    string
		readyCount int32
	}{
		{name: "ready database", kind: "scale_database", status: "running", cluster: "starting", readyCount: 2},
		{name: "ready router without label", kind: "scale_router", status: "running", cluster: "starting", readyCount: 1},
		{name: "completed database scale", kind: "scale_database", status: "success", cluster: "ready", readyCount: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			s, err := store.New("file:pause-reject-" + strings.ReplaceAll(tc.name, " ", "-") + "?mode=memory&cache=shared")
			if err != nil {
				t.Fatal(err)
			}
			routerCount := 0
			if tc.kind == "scale_router" {
				routerCount = 1
			}
			item := model.MySQLCluster{Name: tc.name, ClusterID: 2, Namespace: "mysql", ResourceName: "mysql-reject", Status: tc.cluster, DatabaseNodes: []string{"node-a"}, DatabaseInstances: 2, RouterInstances: &routerCount, StorageSize: "10Gi"}
			if err := s.DB().Create(&item).Error; err != nil {
				t.Fatal(err)
			}
			task := model.MySQLClusterTask{ClusterID: item.ID, Kind: tc.kind, Status: tc.status}
			if err := s.DB().Create(&task).Error; err != nil {
				t.Fatal(err)
			}
			gvr := schema.GroupVersionResource{Group: "mysql.oracle.com", Version: "v2", Resource: "innodbclusters"}
			client := &k8s.Client{
				Clientset: k8sfake.NewSimpleClientset(
					&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName, Namespace: item.Namespace}, Status: appsv1.StatefulSetStatus{ReadyReplicas: tc.readyCount}},
					&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName + "-router", Namespace: item.Namespace}, Status: appsv1.DeploymentStatus{ReadyReplicas: tc.readyCount}},
				),
				DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "InnoDBClusterList"}, database.ClusterObject(&item)),
			}
			h := NewDatabaseHandler(s, bytes.Repeat([]byte("a"), 32), func(context.Context, uint) (*k8s.Client, error) { return client, nil })
			router := gin.New()
			router.POST("/:id/pause", h.PauseClusterDeployment)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/1/pause", nil))
			if response.Code == http.StatusOK {
				t.Fatalf("unexpected withdrawal: %s", response.Body.String())
			}
			resource, err := client.DynamicClient.Resource(gvr).Namespace(item.Namespace).Get(context.Background(), item.ResourceName, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("cluster resource removed: %v", err)
			}
			instances, _, _ := unstructured.NestedInt64(resource.Object, "spec", "instances")
			routers, _, _ := unstructured.NestedInt64(resource.Object, "spec", "router", "instances")
			if instances != 2 || routers != int64(routerCount) {
				t.Fatalf("operator targets changed: MySQL %d, Router %d", instances, routers)
			}
		})
	}
}

func TestRetryShutdownTaskTracksResourceDeletionAndTimeout(t *testing.T) {
	s, err := store.New("file:mysql-shutdown-retry-progress-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	h := NewDatabaseHandler(s, bytes.Repeat([]byte("a"), 32), nil)
	task := model.MySQLClusterTask{ClusterID: 1, Kind: "retry_shutdown", Status: "running", CreatedAt: time.Now().Add(-6 * time.Minute)}
	if err := s.DB().Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.syncRetryShutdownTask(1, &database.ClusterCleanupState{ClusterResource: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.DB().First(&task, task.ID).Error; err != nil || task.Status != "failed" {
		t.Fatalf("stalled shutdown not marked failed: task=%+v err=%v", task, err)
	}
	var count int64
	if err := s.DB().Model(&model.MySQLClusterTaskLog{}).Where("task_id = ?", task.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("unexpected retry logs after timeout: count=%d err=%v", count, err)
	}
	task = model.MySQLClusterTask{ClusterID: 1, Kind: "retry_shutdown", Status: "running"}
	if err := s.DB().Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.syncRetryShutdownTask(1, &database.ClusterCleanupState{}); err != nil {
		t.Fatal(err)
	}
	if err := s.DB().First(&task, task.ID).Error; err != nil || task.Status != "success" {
		t.Fatalf("completed shutdown not marked successful: task=%+v err=%v", task, err)
	}
}

func TestRetryClusterShutdownCreatesTrackedTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:mysql-shutdown-retry-api-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	item := model.MySQLCluster{Name: "retry", ClusterID: 2, Namespace: "mysql", ResourceName: "mysql-retry", Status: "paused"}
	if err := s.DB().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	cluster := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "mysql.oracle.com/v2", "kind": "InnoDBCluster", "metadata": map[string]any{"name": item.ResourceName, "namespace": item.Namespace, "labels": map[string]any{k8s.ManagedByLabel: k8s.ManagedByValue}}}}
	started := metav1.NewTime(time.Now().Add(-4 * time.Minute))
	cluster.SetDeletionTimestamp(&started)
	cluster.SetFinalizers([]string{"mysql.oracle.com/cluster"})
	operator := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator", Namespace: "mysql-operator"}}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(operator), DynamicClient: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), cluster)}
	h := NewDatabaseHandler(s, bytes.Repeat([]byte("a"), 32), func(context.Context, uint) (*k8s.Client, error) { return client, nil })
	router := gin.New()
	router.POST("/:id/retry", h.RetryClusterShutdown)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/1/retry", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("retry response: %s", response.Body.String())
	}
	var task model.MySQLClusterTask
	if err := s.DB().Where("cluster_id = ? AND kind = ?", item.ID, "retry_shutdown").First(&task).Error; err != nil || task.Status != "running" {
		t.Fatalf("retry task not recorded: task=%+v err=%v", task, err)
	}
	var count int64
	if err := s.DB().Model(&model.MySQLClusterTaskLog{}).Where("task_id = ?", task.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("retry log not recorded: count=%d err=%v", count, err)
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/1/retry", nil))
	if response.Code == http.StatusOK {
		t.Fatal("second retry bypassed the restart cooldown")
	}
}

func TestCleanupPausedClusterRequiresConfirmationAndExplicitPVCDeletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:mysql-cleanup-confirm-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	item := model.MySQLCluster{Name: "cleanup", ClusterID: 2, Namespace: "mysql", ResourceName: "mysql-cleanup", Status: "paused"}
	if err := s.DB().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "datadir-mysql-cleanup-0", Namespace: item.Namespace}}
	other := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "backup-mysql-cleanup-0", Namespace: item.Namespace}}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(claim, other), DynamicClient: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())}
	h := NewDatabaseHandler(s, bytes.Repeat([]byte("a"), 32), func(context.Context, uint) (*k8s.Client, error) { return client, nil })
	router := gin.New()
	router.POST("/:id/cleanup", h.CleanupPausedCluster)
	request := func(body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/1/cleanup", strings.NewReader(body)))
		return response
	}
	if response := request(`{"confirm_resource_name":"other","delete_claims":true}`); response.Code == http.StatusOK {
		t.Fatal("cleanup accepted an unrelated resource name")
	}
	if response := request(`{"confirm_resource_name":"mysql-cleanup"}`); response.Code != http.StatusOK {
		t.Fatalf("workload cleanup response: %s", response.Body.String())
	}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Get(context.Background(), claim.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("workload cleanup removed database data: %v", err)
	}
	if response := request(`{"confirm_resource_name":"mysql-cleanup","delete_claims":true}`); response.Code != http.StatusOK {
		t.Fatalf("data cleanup response: %s", response.Body.String())
	}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Get(context.Background(), claim.Name, metav1.GetOptions{}); err == nil {
		t.Fatal("confirmed cleanup retained the database claim")
	}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Get(context.Background(), other.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("cleanup touched an unrelated claim: %v", err)
	}
}
