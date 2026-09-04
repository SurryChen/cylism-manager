package cluster

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerLifecycleAdapterUsesUnifiedResponse(t *testing.T) {
	r, st := newClusterTestRouter(t)
	request := httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewBufferString(`{"name":"worker","host":"10.0.0.11"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"code":0`)) {
		t.Fatalf("create response = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/servers", nil))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"name":"worker"`)) {
		t.Fatalf("list response = %d %s", response.Code, response.Body.String())
	}

	serverID := uint(1)
	server, err := st.GetServer(serverID)
	if err != nil || server.Name != "worker" {
		t.Fatalf("stored server = %#v, err=%v", server, err)
	}
	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/servers/1/unbind", nil))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("已解除集群绑定")) {
		t.Fatalf("unbind response = %d %s", response.Code, response.Body.String())
	}
}
