package network

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNetworkHandlerKeepsComposedRouteCallbacks(t *testing.T) {
	callback := func(c *gin.Context) { c.Status(http.StatusNoContent) }
	h := NewNetworkHandler(nil, NetworkHandler{RouteList: callback})
	if h.Service != nil || h.RouteList == nil {
		t.Fatal("network handler did not retain composed dependencies")
	}
	router := gin.New()
	router.GET("/routes", h.RouteList)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/routes", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("callback status = %d", response.Code)
	}
}
