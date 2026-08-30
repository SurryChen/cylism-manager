package infrastructure

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gin-gonic/gin"
)

func newJSONRequest(method, path string, body interface{}) *http.Request {
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, strings.NewReader(string(data)))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func serve(r *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	return response
}
