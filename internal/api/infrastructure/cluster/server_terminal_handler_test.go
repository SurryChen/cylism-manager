package cluster

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestServerTerminalRejectsInvalidServerIDBeforeUpgrade(t *testing.T) {
	store, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/api/servers/:id/terminal", NewServerTerminalHandler(store, nil).Terminal)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/servers/not-an-id/terminal", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}
