package network

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNginxHandlerImportReportsPendingImplementation(t *testing.T) {
	router := gin.New()
	router.GET("/nginx/import", NewNginxHandler().Import)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/nginx/import", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "pending") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}
