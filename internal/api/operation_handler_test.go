package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	var resp struct {
		Operations []model.OperationLog `json:"operations"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if len(resp.Operations) != 2 {
		t.Errorf("expected 2 operations, got %d", len(resp.Operations))
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

	var resp struct {
		Operations []model.OperationLog `json:"operations"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Operations) != 0 {
		t.Errorf("expected 0 operations, got %d", len(resp.Operations))
	}
}

func TestOperationHandler_MissingParams(t *testing.T) {
	r, s := setupTestRouter()

	handler := NewOperationHandler(s)
	r.GET("/api/operations", handler.ListOperations)

	req := httptest.NewRequest(http.MethodGet, "/api/operations", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
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
