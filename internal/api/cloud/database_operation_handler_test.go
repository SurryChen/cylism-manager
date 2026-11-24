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
	"github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/service/database"
	"github.com/cylism/cylism-manager/internal/service/database/operations"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestStandaloneMySQLDeploymentQueuesEmptyInstanceAndProtectsCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db, _ := s.DB().DB()
	t.Cleanup(func() { _ = db.Close() })
	key := bytes.Repeat([]byte("k"), 32)
	cluster := model.Cluster{Name: "apps", Status: "ready", Distribution: "k3s"}
	if err = s.DB().Create(&cluster).Error; err != nil {
		t.Fatal(err)
	}
	client := &k8s.Client{Clientset: fake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "ubuntudev"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local", Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}}},
	)}
	factory := func(context.Context, uint) (*k8s.Client, error) { return client, nil }
	service := operations.New(s, key, factory)
	h := NewDatabaseHandler(s, key, factory).WithOperations(service)
	r := gin.New()
	r.POST("/databases", h.Create)
	r.GET("/databases", h.List)
	r.DELETE("/databases/:id", h.Delete)
	r.PUT("/databases/:id/connection", h.UpdateConnection)
	r.GET("/operations/:taskID", h.DatabaseOperationDetail)
	r.POST("/operations/:taskID/actions", h.ControlDatabaseOperation)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	w := request(http.MethodPost, "/databases", fmt.Sprintf(`{"name":"standalone","mode":"managed","cluster_id":%d,"node_name":"ubuntudev"}`, cluster.ID))
	if w.Code != http.StatusOK {
		t.Fatalf("cannot deploy empty MySQL: %s", w.Body.String())
	}
	var item model.DatabaseInstance
	if err := s.DB().First(&item).Error; err != nil {
		t.Fatal(err)
	}
	root, err := security.Decrypt(key, item.EncryptedRoot)
	if err != nil || root == "" {
		t.Fatal("missing encrypted generated root password")
	}
	if item.Username != "" || item.DatabaseName != "" || item.CPULimit != "1" || item.MemoryLimit != "1Gi" || item.StorageClass != "local" {
		t.Fatalf("unexpected instance defaults: %+v", item)
	}
	if strings.Contains(w.Body.String(), root) || strings.Contains(w.Body.String(), item.EncryptedRoot) {
		t.Fatal("deployment API leaked root credential")
	}
	sets, err := client.Clientset.AppsV1().StatefulSets("cylism-databases").List(context.Background(), metav1.ListOptions{})
	if err != nil || len(sets.Items) != 0 {
		t.Fatal("HTTP handler performed deployment synchronously")
	}
	var task model.DatabaseOperation
	if err = s.DB().First(&task).Error; err != nil {
		t.Fatal(err)
	}
	if task.Kind != "deploy" || task.Status != "queued" {
		t.Fatal("missing persistent deployment task")
	}
	w = request(http.MethodDelete, fmt.Sprintf("/databases/%d", item.ID), "")
	if w.Code == http.StatusOK {
		t.Fatal("deleted instance during pending deploy task")
	}
	w = request(http.MethodPost, fmt.Sprintf("/operations/%d/actions", task.ID), `{"action":"pause"}`)
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	service.Reconcile(context.Background())
	w = request(http.MethodGet, fmt.Sprintf("/operations/%d", task.ID), "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"paused"`) || !strings.Contains(w.Body.String(), "提交独立 MySQL") {
		t.Fatal(w.Body.String())
	}
	if err = service.Control(task.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	service.Reconcile(context.Background())
	w = request(http.MethodPut, fmt.Sprintf("/databases/%d/connection", item.ID), `{"database_name":"app_db","username":"app_user","password":"existing-password"}`)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "existing-password") {
		t.Fatalf("cannot separately save an existing business credential: %s", w.Body.String())
	}
	// This only saves an existing account: it must not replace deployment root.
	s.DB().First(&item, item.ID)
	plain, err := security.Decrypt(key, item.EncryptedPassword)
	if err != nil || plain != "existing-password" {
		t.Fatal("business credential not encrypted")
	}
	currentRoot, _ := security.Decrypt(key, item.EncryptedRoot)
	if currentRoot != root {
		t.Fatal("business credential changed deployment root")
	}
}

func TestDeleteRechecksTaskLockBeforeRemovingWorkload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db, _ := s.DB().DB()
	t.Cleanup(func() { _ = db.Close() })
	item := model.DatabaseInstance{Name: "standalone", Mode: "managed", ClusterID: 1}
	if err := s.DB().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	client := &k8s.Client{Clientset: fake.NewSimpleClientset()}
	factory := func(context.Context, uint) (*k8s.Client, error) {
		// Simulate a task queued after the handler's initial preflight checks.
		if err := s.DB().Create(&model.DatabaseOperationLock{Resource: operations.Reference(item.ID), TaskID: 1}).Error; err != nil {
			t.Fatal(err)
		}
		return client, nil
	}
	h := NewDatabaseHandler(s, nil, factory)
	r := gin.New()
	r.DELETE("/databases/:id", h.Delete)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/databases/%d", item.ID), nil))
	if w.Code == http.StatusOK || !strings.Contains(w.Body.String(), "资源仍被数据库任务占用") {
		t.Fatalf("deletion ignored concurrent task: %s", w.Body.String())
	}
	if len(client.Clientset.(*fake.Clientset).Actions()) != 0 {
		t.Fatal("Kubernetes resources were accessed before rechecking the task lock")
	}
	var count int64
	if err := s.DB().Model(&model.DatabaseInstance{}).Where("id = ?", item.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("locked instance record was removed")
	}
	if err := s.DB().Where("resource = ?", operations.Reference(item.ID)).Delete(&model.DatabaseOperationLock{}).Error; err != nil {
		t.Fatal(err)
	}
	h.client = func(context.Context, uint) (*k8s.Client, error) { return client, nil }
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/databases/%d", item.ID), nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), database.PVCName(item.ID)) {
		t.Fatalf("unlocked deletion failed: %s", w.Body.String())
	}
}
