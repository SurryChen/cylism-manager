package cluster

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gin-gonic/gin"
)

func newJSONRequest(method, path string, body any) *http.Request {
	var payload strings.Builder
	if body != nil {
		_ = json.NewEncoder(&payload).Encode(body)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(payload.String()))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func serve(router *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}
