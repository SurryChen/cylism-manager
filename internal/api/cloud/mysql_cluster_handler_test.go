package cloud

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestCreateMySQLClusterOnlyCreatesDraft(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:mysql-cluster-handler-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	cluster := model.Cluster{Name: "target", Distribution: "k3s", Status: "ready"}
	if err := s.DB().Create(&cluster).Error; err != nil {
		t.Fatal(err)
	}
	h := NewDatabaseHandler(s, bytes.Repeat([]byte("a"), 32), nil)
	router := gin.New()
	router.POST("/clusters", h.CreateCluster)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/clusters", strings.NewReader(`{"name":"production","cluster_id":1}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("create response: %s", response.Body.String())
	}
	var item model.MySQLCluster
	if err := s.DB().First(&item).Error; err != nil {
		t.Fatal(err)
	}
	if item.Status != "draft" || item.ClusterID != cluster.ID || item.ResourceName == "" || item.EncryptedRoot != "" {
		t.Fatalf("unexpected draft: %+v", item)
	}
	if len(item.DatabaseNodes) != 0 || len(item.RouterNodes) != 0 {
		t.Fatal("draft must not have deployed members")
	}
}

func TestStartMySQLTaskSavesConfigurationAndPreventsDuplicate(t *testing.T) {
	s, err := store.New("file:mysql-task-start-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	item := model.MySQLCluster{Name: "test", ClusterID: 1, Namespace: "mysql", ResourceName: "mysql-test", Status: "draft", DatabaseNodes: []string{}, RouterNodes: []string{}}
	if err := s.DB().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	h := NewDatabaseHandler(s, bytes.Repeat([]byte("a"), 32), nil)
	item.DatabaseNodes = []string{"node-a", "node-b", "node-c"}
	item.DatabaseInstances = 3
	item.RouterNodes = []string{"node-a", "node-b"}
	item.StorageSize = "20Gi"
	item.PreferredPrimaryNode = "node-b"
	item.EncryptedRoot = "encrypted"
	task, err := h.startMySQLTask(&item)
	if err != nil {
		t.Fatal(err)
	}
	if task.ID == 0 || item.Status != "starting" {
		t.Fatalf("task was not started: %+v, %+v", task, item)
	}
	var saved model.MySQLCluster
	if err := s.DB().First(&saved, item.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Status != "starting" || saved.StorageSize != "20Gi" || saved.EncryptedRoot != "encrypted" || len(saved.DatabaseNodes) != 3 || saved.DatabaseInstances != 3 || len(saved.RouterNodes) != 2 || saved.PreferredPrimaryNode != "node-b" || saved.PrimaryConfigured {
		t.Fatalf("deployment configuration was not saved with task: %+v", saved)
	}
	if _, err := h.startMySQLTask(&item); err == nil {
		t.Fatal("duplicate deployment should be rejected")
	}
	var count int64
	if err := s.DB().Model(&model.MySQLClusterTask{}).Where("cluster_id = ?", item.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("expected one task, count=%d err=%v", count, err)
	}
}
