package system

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func setupTestRouter() (*gin.Engine, *store.Store) {
	gin.SetMode(gin.TestMode)
	s, _ := store.New(":memory:")
	r := gin.New()
	return r, s
}

func TestOperationHandler_ListOperations(t *testing.T) {
	r, s := setupTestRouter()

	// Seed data
	s.CreateOperationLog(&model.OperationLog{
		ResourceType: "server",
		ResourceID:   1,
		Step:         "正在连接 SSH",
		Status:       "success",
		Detail:       "连接成功",
	})
	s.CreateOperationLog(&model.OperationLog{
		ResourceType: "server",
		ResourceID:   1,
		Step:         "正在上传 Agent",
		Status:       "running",
	})

	handler := NewOperationHandler(s)
	r.GET("/api/operations", handler.ListOperations)

	req := httptest.NewRequest(http.MethodGet, "/api/operations?resource_type=server&resource_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var apiResp struct {
		Code int `json:"code"`
		Data struct {
			Operations []model.OperationLog `json:"operations"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &apiResp); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if len(apiResp.Data.Operations) != 2 {
		t.Errorf("expected 2 operations, got %d", len(apiResp.Data.Operations))
	}
}

func TestOperationHandler_ListOperationsEmpty(t *testing.T) {
	r, s := setupTestRouter()

	handler := NewOperationHandler(s)
	r.GET("/api/operations", handler.ListOperations)

	req := httptest.NewRequest(http.MethodGet, "/api/operations?resource_type=server&resource_id=999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var apiResp struct {
		Code int `json:"code"`
		Data struct {
			Operations []model.OperationLog `json:"operations"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &apiResp)
	if len(apiResp.Data.Operations) != 0 {
		t.Errorf("expected 0 operations, got %d", len(apiResp.Data.Operations))
	}
}

func TestOperationHandler_ListsGlobalOperationsWithFilters(t *testing.T) {
	r, s := setupTestRouter()
	if err := s.CreateOperationLog(&model.OperationLog{ResourceType: "application", ResourceID: 1, Step: "发布应用", Status: "success", Detail: "console v1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateOperationLog(&model.OperationLog{ResourceType: "application", ResourceID: 2, Step: "发布应用", Status: "failed", Detail: "worker v2"}); err != nil {
		t.Fatal(err)
	}

	handler := NewOperationHandler(s)
	r.GET("/api/operations", handler.ListOperations)

	req := httptest.NewRequest(http.MethodGet, "/api/operations?resource_type=application&status=failed&keyword=worker&limit=20", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"total":1`) || !strings.Contains(w.Body.String(), `"resource_id":2`) {
		t.Fatalf("unexpected global operations response: %d %s", w.Code, w.Body.String())
	}
}

func TestOperationHandler_InvalidResourceID(t *testing.T) {
	r, s := setupTestRouter()

	handler := NewOperationHandler(s)
	r.GET("/api/operations", handler.ListOperations)

	req := httptest.NewRequest(http.MethodGet, "/api/operations?resource_type=server&resource_id=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}
