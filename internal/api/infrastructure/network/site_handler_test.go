package network

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestSiteHandlerCreatesAndListsSites(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	handler := NewSiteHandler(st)
	router.POST("/api/sites", handler.Create)
	router.GET("/api/sites", handler.List)

	response := serve(router, newJSONRequest(http.MethodPost, "/api/sites", gin.H{
		"server_id": 1,
		"domain":    "example.com",
	}))
	if response.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serve(router, newJSONRequest(http.MethodGet, "/api/sites", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "example.com") {
		t.Fatalf("list status = %d, body = %s", response.Code, response.Body.String())
	}
}
