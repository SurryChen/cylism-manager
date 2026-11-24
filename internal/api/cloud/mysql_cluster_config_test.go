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

func TestDraftMySQLConfigPersistsAndRejectsOperatorOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:mysql-config-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	item := model.MySQLCluster{Name: "config-test", ClusterID: 1, Namespace: "mysql", ResourceName: "mysql-config-test", Status: "draft"}
	if err := s.DB().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	h := NewDatabaseHandler(s, bytes.Repeat([]byte("a"), 32), nil)
	router := gin.New()
	router.PUT("/:id/config", h.UpdateMySQLClusterConfig)
	router.GET("/:id/config", h.MySQLClusterConfig)
	for _, tc := range []struct {
		body string
		want int
	}{
		{`{"mycnf":"[mysqld]\ninnodb_buffer_pool_size=128M"}`, http.StatusOK},
		{`{"mycnf":"[mysqld]\nperformance_schema=OFF"}`, http.StatusBadRequest},
		{`{"mycnf":"","cpu_limit":"1","memory_limit":"512Mi"}`, http.StatusBadRequest},
		{`{"mycnf":"","cpu_limit":"1","memory_limit":"1Gi"}`, http.StatusOK},
	} {
		request := httptest.NewRequest(http.MethodPut, "/1/config", strings.NewReader(tc.body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != tc.want {
			t.Fatalf("config update status = %d, body = %s", response.Code, response.Body.String())
		}
	}
	if err := s.DB().First(&item, item.ID).Error; err != nil {
		t.Fatal(err)
	}
	if item.MyCNF != "" || item.CPULimit != "1" || item.MemoryLimit != "1Gi" {
		t.Fatalf("persisted config = %q, cpu=%q, memory=%q", item.MyCNF, item.CPULimit, item.MemoryLimit)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/1/config", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "innodb_buffer_pool_size=256M") {
		t.Fatalf("config read status = %d, body = %s", response.Code, response.Body.String())
	}
}
