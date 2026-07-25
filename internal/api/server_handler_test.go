package api

import (
	"testing"

	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func setupServerRouter() (*gin.Engine, *store.Store) {
	gin.SetMode(gin.TestMode)
	s, _ := store.New(":memory:")
	r := gin.New()
	encKey := make([]byte, 32)
	h := NewServerHandler(s, encKey)
	servers := r.Group("/api/servers")
	{
		servers.POST("", h.Create)
		servers.GET("", h.List)
		servers.GET("/:id", h.Get)
		servers.PUT("/:id", h.Update)
		servers.DELETE("/:id", h.Delete)
		servers.POST("/:id/probe", h.Probe)
		servers.POST("/:id/precheck", h.Precheck)
	}
	return r, s
}

func TestServerHandler_ProbeNotFound(t *testing.T) {
	r, _ := setupServerRouter()
	req := newJSONRequest("POST", "/api/servers/999/probe", nil)
	w := serve(r, req)
	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestServerHandler_PrecheckNotFound(t *testing.T) {
	r, _ := setupServerRouter()
	req := newJSONRequest("POST", "/api/servers/999/precheck", nil)
	w := serve(r, req)
	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
