package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestListMySQLClusterTasksPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:mysql-task-pagination-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	cluster := model.MySQLCluster{Name: "test", ClusterID: 1, Status: "draft"}
	other := model.MySQLCluster{Name: "other", ClusterID: 1, Status: "draft"}
	if err := s.DB().Create(&cluster).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DB().Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 23; i++ {
		task := model.MySQLClusterTask{ClusterID: cluster.ID, Kind: "deploy", Status: "success"}
		if i == 0 {
			task.Kind, task.Status = "uninstall_operator", "running"
		}
		if err := s.DB().Create(&task).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DB().Create(&model.MySQLClusterTask{ClusterID: other.ID, Kind: "deploy", Status: "success"}).Error; err != nil {
		t.Fatal(err)
	}
	h := NewDatabaseHandler(s, bytes.Repeat([]byte("a"), 32), nil)
	router := gin.New()
	router.GET("/:id/tasks", h.ListMySQLClusterTasks)
	type taskListResponse struct {
		Code int `json:"code"`
		Data struct {
			Items                    []model.MySQLClusterTask `json:"items"`
			Total                    int64                    `json:"total"`
			OperatorUninstallPending bool                     `json:"operator_uninstall_pending"`
		} `json:"data"`
	}
	read := func(query string) taskListResponse {
		t.Helper()
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/%d/tasks%s", cluster.ID, query), nil))
		var result taskListResponse
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || response.Code != http.StatusOK || result.Code != 0 {
			t.Fatalf("task response: %s, error = %v", response.Body.String(), err)
		}
		return result
	}
	first := read("?limit=10&offset=0")
	second := read("?limit=10&offset=10")
	last := read("?limit=10&offset=20")
	if first.Data.Total != 23 || len(first.Data.Items) != 10 || len(second.Data.Items) != 10 || len(last.Data.Items) != 3 {
		t.Fatalf("unexpected pages: first=%+v second=%+v last=%+v", first.Data, second.Data, last.Data)
	}
	if first.Data.Items[0].ID <= first.Data.Items[9].ID || first.Data.Items[9].ID <= second.Data.Items[0].ID || second.Data.Items[9].ID <= last.Data.Items[0].ID {
		t.Fatal("tasks are not ordered newest first across pages")
	}
	if !second.Data.OperatorUninstallPending || !last.Data.OperatorUninstallPending {
		t.Fatal("running Operator uninstall was lost on an older page")
	}
}
