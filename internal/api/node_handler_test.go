package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func setupNodeRouter() (*gin.Engine, *store.Store) {
	gin.SetMode(gin.TestMode)
	s, _ := store.New(":memory:")
	r := gin.New()
	h := NewNodeHandler(s)
	nodes := r.Group("/api/nodes")
	{
		nodes.GET("", h.ListNode)
		nodes.POST("/:id/add", h.AddNode)
		nodes.POST("/:id/drain", h.DrainNode)
		nodes.DELETE("/:id", h.RemoveNode)
	}
	return r, s
}

func TestNodeHandler_ListNode(t *testing.T) {
	r, _ := setupNodeRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestNodeHandler_AddNode(t *testing.T) {
	r, s := setupNodeRouter()
	s.DB().Exec("INSERT INTO servers (name, host, ssh_auth_type, ssh_user, ssh_port) VALUES ('test', '1.1.1.1', 'password', 'root', 22)")

	req := httptest.NewRequest(http.MethodPost, "/api/nodes/1/add", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestNodeHandler_AddNodeNotFound(t *testing.T) {
	r, _ := setupNodeRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/nodes/999/add", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestNodeHandler_DrainNode(t *testing.T) {
	r, _ := setupNodeRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/nodes/web-01/drain", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestNodeHandler_RemoveNode(t *testing.T) {
	r, _ := setupNodeRouter()
	req := httptest.NewRequest(http.MethodDelete, "/api/nodes/web-01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
