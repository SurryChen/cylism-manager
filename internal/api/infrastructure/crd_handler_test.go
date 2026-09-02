package infrastructure

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCRDHandler_CheckCRDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewCRDHandler()
	r.GET("/api/system/crds", h.CheckCRDs)

	req := httptest.NewRequest(http.MethodGet, "/api/system/crds", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
