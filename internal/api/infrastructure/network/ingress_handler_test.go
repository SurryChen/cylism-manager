package network

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupIngressRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewIngressHandler(nil)
	ing := r.Group("/api/routes")
	{
		ing.GET("", h.ListRoutes)
		ing.POST("", h.CreateRoute)
		ing.DELETE("/:namespace/:name", h.DeleteRoute)
	}
	return r
}

func TestIngressHandler_ListRoutes(t *testing.T) {
	r := setupIngressRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/routes", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestIngressHandler_DeleteRoute(t *testing.T) {
	r := setupIngressRouter()
	req := httptest.NewRequest(http.MethodDelete, "/api/routes/default/test-route", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
