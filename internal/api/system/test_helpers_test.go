package system

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/gin-gonic/gin"
)

// k8sClient is test-local state used by legacy system handler fixtures. The
// production package no longer keeps a mutable Kubernetes client global.
var k8sClient *k8sclient.Client

func newJSONRequest(method, path string, body interface{}) *http.Request {
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func serve(r *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	return response
}
