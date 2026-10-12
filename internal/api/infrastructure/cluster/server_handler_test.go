package cluster

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	clusterservice "github.com/cylism/cylism-manager/internal/service/cluster"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
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

func TestServerNameDefaultsToHostAndFollowsHostChanges(t *testing.T) {
	r, st := newClusterTestRouter(t)
	r.PUT("/api/servers/:id", NewServerHandler(make([]byte, 32), clusterservice.NewService(st, nil)).Update)
	request := httptest.NewRequest(http.MethodPost, "/api/servers", strings.NewReader(`{"host":"10.0.0.11"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"10.0.0.11"`) {
		t.Fatalf("create response = %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPut, "/api/servers/1", strings.NewReader(`{"host":"10.0.0.12"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	server, err := st.GetServer(1)
	if err != nil || response.Code != http.StatusOK || server.Name != "10.0.0.12" || server.Host != "10.0.0.12" {
		t.Fatalf("updated server = %#v, response = %d %s, err = %v", server, response.Code, response.Body.String(), err)
	}

	server.Name = "自定义名称"
	if err := st.UpdateServer(server); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPut, "/api/servers/1", strings.NewReader(`{"host":"10.0.0.13"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	server, err = st.GetServer(1)
	if err != nil || response.Code != http.StatusOK || server.Name != "自定义名称" || server.Host != "10.0.0.13" {
		t.Fatalf("custom server = %#v, response = %d %s, err = %v", server, response.Code, response.Body.String(), err)
	}

	request = httptest.NewRequest(http.MethodPut, "/api/servers/1", strings.NewReader(`{"name":"  "}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	server, err = st.GetServer(1)
	if err != nil || response.Code != http.StatusOK || server.Name != "10.0.0.13" {
		t.Fatalf("cleared server name = %#v, response = %d %s, err = %v", server, response.Code, response.Body.String(), err)
	}
}

func TestDuplicateServerHostReturnsActionableMessage(t *testing.T) {
	r, st := newClusterTestRouter(t)
	r.PUT("/api/servers/:id", NewServerHandler(make([]byte, 32), clusterservice.NewService(st, nil)).Update)
	if err := st.CreateServer(&model.Server{Name: "existing", Host: "10.0.0.11"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateServer(&model.Server{Name: "another", Host: "10.0.0.12"}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/servers", strings.NewReader(`{"host":"10.0.0.11","hostname":"worker-a"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "该主机已添加，请编辑已有服务器") {
		t.Fatalf("duplicate response = %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPut, "/api/servers/2", strings.NewReader(`{"host":"10.0.0.11","hostname":"worker-b"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "该主机已添加，请编辑已有服务器") {
		t.Fatalf("duplicate update response = %d %s", response.Code, response.Body.String())
	}
}

func TestServerProxyLifecycle(t *testing.T) {
	r, st := newClusterTestRouter(t)
	handler := NewServerHandler(make([]byte, 32), clusterservice.NewService(st, nil))
	r.PUT("/api/servers/:id", handler.Update)
	r.DELETE("/api/servers/:id", handler.Delete)
	send := func(method, path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		return response
	}
	proxy := &model.Server{Name: "gateway", Host: "203.0.113.1"}
	if err := st.CreateServer(proxy); err != nil {
		t.Fatal(err)
	}
	response := send(http.MethodPost, "/api/servers", fmt.Sprintf(`{"name":"internal","host":"100.100.1.2","proxy_server_id":%d}`, proxy.ID))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"proxy_server_name":"gateway"`) {
		t.Fatalf("create proxy server = %d %s", response.Code, response.Body.String())
	}
	response = send(http.MethodGet, "/api/servers", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"proxy_server_name":"gateway"`) {
		t.Fatalf("list proxy server = %d %s", response.Code, response.Body.String())
	}
	response = send(http.MethodPut, "/api/servers/2", `{"name":"renamed"}`)
	stored, err := st.GetServer(2)
	if response.Code != http.StatusOK || err != nil || stored.ProxyServerID == nil || *stored.ProxyServerID != proxy.ID {
		t.Fatalf("partial update lost proxy: response=%d %s, stored=%#v, err=%v", response.Code, response.Body.String(), stored, err)
	}
	response = send(http.MethodDelete, "/api/servers/1", "")
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "代理节点") {
		t.Fatalf("delete active proxy = %d %s", response.Code, response.Body.String())
	}
	response = send(http.MethodPut, "/api/servers/1", `{"proxy_server_id":2}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("nested proxy = %d %s", response.Code, response.Body.String())
	}
	response = send(http.MethodPut, "/api/servers/2", `{"proxy_server_id":null}`)
	stored, err = st.GetServer(2)
	if response.Code != http.StatusOK || err != nil || stored.ProxyServerID != nil {
		t.Fatalf("clear proxy = %d %s, stored=%#v, err=%v", response.Code, response.Body.String(), stored, err)
	}
}

type recordingServerInspector struct {
	server        *model.Server
	hostname      string
	hostnameErr   error
	hostnameCalls int
}

func (i *recordingServerInspector) Probe(_ context.Context, server *model.Server) (bool, string) {
	copy := *server
	i.server = &copy
	return true, ""
}

func (i *recordingServerInspector) Precheck(context.Context, *model.Server) []clusterservice.Precheck {
	return nil
}

func (i *recordingServerInspector) Hostname(_ context.Context, server *model.Server) (string, error) {
	copy := *server
	i.server = &copy
	i.hostnameCalls++
	return i.hostname, i.hostnameErr
}

func TestServerHostnameIsResolvedOnlyOnRequestAndSavedWithDetails(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	inspector := &recordingServerInspector{hostname: "cloudzone.cylism.com"}
	handler := NewServerHandler(key, clusterservice.NewService(st, nil).WithServerImporter(inspector))
	router := gin.New()
	router.POST("/servers", handler.Create)
	router.POST("/servers/hostname", handler.ResolveHostname)
	router.POST("/servers/:id/hostname", handler.ResolveHostname)
	router.PUT("/servers/:id", handler.Update)
	router.GET("/servers", handler.List)
	send := func(method, path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}

	response := send(http.MethodPost, "/servers", `{"host":"10.0.0.9","ssh_user":"root","ssh_password":"old-secret"}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"hostname":""`) || inspector.hostnameCalls != 0 {
		t.Fatalf("create response = %d %s", response.Code, response.Body.String())
	}

	response = send(http.MethodPost, "/servers/hostname", `{"host":"10.0.0.10","ssh_user":"admin","ssh_password":"new-secret"}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"hostname":"cloudzone.cylism.com"`) || inspector.hostnameCalls != 1 || inspector.server.ID != 0 || inspector.server.Host != "10.0.0.10" {
		t.Fatalf("draft lookup response = %d %s, server = %#v", response.Code, response.Body.String(), inspector.server)
	}
	password, err := security.Decrypt(key, inspector.server.SSHPassword)
	if err != nil || password != "new-secret" {
		t.Fatalf("draft password = %q, err = %v", password, err)
	}

	response = send(http.MethodPost, "/servers/1/hostname", `{"host":"10.0.0.9","ssh_user":"root"}`)
	if response.Code != http.StatusOK || inspector.hostnameCalls != 2 || inspector.server.ID != 1 {
		t.Fatalf("edit lookup response = %d %s, server = %#v", response.Code, response.Body.String(), inspector.server)
	}
	password, err = security.Decrypt(key, inspector.server.SSHPassword)
	if err != nil || password != "old-secret" {
		t.Fatalf("stored password = %q, err = %v", password, err)
	}

	stored, err := st.GetServer(1)
	if err != nil || stored.Hostname != "" || stored.Host != "10.0.0.9" {
		t.Fatalf("lookup changed stored server = %#v, err = %v", stored, err)
	}

	response = send(http.MethodPut, "/servers/1", `{"hostname":"cloudzone.cylism.com","ssh_user":"admin"}`)
	stored, err = st.GetServer(1)
	if response.Code != http.StatusOK || err != nil || stored.Hostname != "cloudzone.cylism.com" || stored.SSHUser != "admin" || inspector.hostnameCalls != 2 {
		t.Fatalf("save response = %d %s, server = %#v, calls = %d, err = %v", response.Code, response.Body.String(), stored, inspector.hostnameCalls, err)
	}

	response = send(http.MethodGet, "/servers", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"hostname":"cloudzone.cylism.com"`) || inspector.hostnameCalls != 2 {
		t.Fatalf("list response = %d %s, hostname calls = %d", response.Code, response.Body.String(), inspector.hostnameCalls)
	}

	inspector.hostnameErr = errors.New("SSH 认证失败")
	response = send(http.MethodPost, "/servers/1/hostname", `{"host":"10.0.0.9","ssh_user":"admin"}`)
	stored, err = st.GetServer(1)
	if err != nil || response.Code != http.StatusBadRequest || stored.Hostname != "cloudzone.cylism.com" || !strings.Contains(response.Body.String(), "SSH 认证失败") {
		t.Fatalf("failed lookup changed server = %#v, response = %d %s, err = %v", stored, response.Code, response.Body.String(), err)
	}
}

func TestProbeServerUsesDraftConnectionWithoutSavingIt(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	storedPassword, err := security.Encrypt(key, "old-password")
	if err != nil {
		t.Fatal(err)
	}
	server := &model.Server{Name: "worker", Host: "10.0.0.1", SSHPort: 22, SSHUser: "root", SSHAuthType: "password", SSHPassword: storedPassword}
	if err := st.CreateServer(server); err != nil {
		t.Fatal(err)
	}
	inspector := &recordingServerInspector{}
	handler := NewServerHandler(key, clusterservice.NewService(st, nil).WithServerInspector(inspector))
	router := gin.New()
	router.POST("/servers/:id/probe", handler.Probe)
	path := fmt.Sprintf("/servers/%d/probe", server.ID)

	for _, test := range []struct {
		name, password, expectedPassword string
	}{
		{name: "new password", password: "new-password", expectedPassword: "new-password"},
		{name: "keep stored password", password: "", expectedPassword: "old-password"},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"host":"10.0.0.9","ssh_port":2222,"ssh_user":"admin","ssh_auth_type":"password","ssh_password":%q}`, test.password)
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("probe status = %d, body = %s", response.Code, response.Body.String())
			}
			if inspector.server == nil || inspector.server.Host != "10.0.0.9" || inspector.server.SSHPort != 2222 || inspector.server.SSHUser != "admin" {
				t.Fatalf("draft connection was not probed: %#v", inspector.server)
			}
			password, err := security.Decrypt(key, inspector.server.SSHPassword)
			if err != nil || password != test.expectedPassword {
				t.Fatalf("probed password = %q, err = %v", password, err)
			}
			stored, err := st.GetServer(server.ID)
			if err != nil || stored.Host != "10.0.0.1" || stored.SSHPassword != storedPassword {
				t.Fatalf("probe changed stored server: %#v, err = %v", stored, err)
			}
		})
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
	if response.Code != http.StatusOK || inspector.server.Host != "10.0.0.1" {
		t.Fatalf("stored connection probe = %d, server = %#v", response.Code, inspector.server)
	}
}
