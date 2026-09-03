package agent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/repository"
	"github.com/gin-gonic/gin"
)

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

func itoa(id uint) string { return strconv.FormatUint(uint64(id), 10) }

func newTestAgentHandler(store repository.AgentReadRepository, client *k8s.Client, authenticator AgentAuthenticator) *AgentHandler {
	return NewAgentHandlerWithKubernetesAdapter(store, NewKubernetesAdapter(client), authenticator)
}

func newTestAgentOperationHandler(store repository.AgentOperationManagementRepository, client *k8s.Client) *AgentOperationHandler {
	return NewAgentOperationHandlerWithKubernetesAdapter(store, NewKubernetesAdapter(client))
}
