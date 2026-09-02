package infrastructure

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	networkservice "github.com/cylism/cylism-manager/internal/service/network"
	"github.com/gin-gonic/gin"
)

func setupCertRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &CertHandler{network: networkservice.NewService(nil)}
	cert := r.Group("/api/certs")
	{
		cert.GET("/status", h.Status)
		cert.POST("/install", h.Install)
		cert.GET("", h.ListCerts)
		cert.POST("", h.CreateCert)
		cert.DELETE("/:namespace/:name", h.DeleteCert)
	}
	return r
}

func TestCertHandler_StatusReportsUnavailableWithoutKubernetesClient(t *testing.T) {
	r := setupCertRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/certs/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "unavailable") {
		t.Fatalf("expected unavailable status, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCertHandler_ListCerts(t *testing.T) {
	r := setupCertRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/certs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestCertHandler_DeleteCert(t *testing.T) {
	r := setupCertRouter()
	req := httptest.NewRequest(http.MethodDelete, "/api/certs/default/test-cert", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
