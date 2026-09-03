package applicationapi

import (
	"bytes"
	"encoding/json"
	"github.com/cylism/cylism-manager/internal/repository"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
)

func newJSONRequest(method, path string, body interface{}) *http.Request {
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	return req
}
func serve(r *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func newTestApplicationHandler(resources repository.ApplicationHandlerRepository, encKey []byte, dependencies KubernetesAdapter) *ApplicationHandler {
	return NewApplicationHandlerWithDependencies(resources, applicationservice.NewQueryService(resources), encKey, dependencies)
}
