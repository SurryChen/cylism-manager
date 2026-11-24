package cluster

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	clusterservice "github.com/cylism/cylism-manager/internal/service/cluster"
)

func TestSingBoxURLValidation(t *testing.T) {
	for _, test := range []struct {
		url   string
		valid bool
	}{
		{"https://example.com/config.json?token=abc", true},
		{"http://example.com/config.json", false},
		{"https://user:pass@example.com/config.json", false},
		{"https://example.com/config.json\nwhoami", false},
		{"file:///etc/passwd", false},
	} {
		if got := validSingBoxURL(test.url); got != test.valid {
			t.Errorf("validSingBoxURL(%q) = %t, want %t", test.url, got, test.valid)
		}
	}
}

func TestSingBoxImportScriptQuotesURLAndRollsBack(t *testing.T) {
	configURL := "https://example.com/a'b?token=secret"
	script := singBoxImportScript(configURL)
	if !strings.Contains(script, "url='https://example.com/a'\\''b?token=secret'") {
		t.Fatalf("URL is not shell quoted: %s", script)
	}
	for _, fragment := range []string{"sing-box check -c", "--proto-redir '=https'", "cp -p \"$backup\" /etc/sing-box/config.json", "echo RESTART_FAILED"} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("script missing %q", fragment)
		}
	}
}

func TestSingBoxImportRejectsInvalidURLBeforeSSH(t *testing.T) {
	router, st := newClusterTestRouter(t)
	if err := st.CreateServer(&model.Server{Name: "node", Host: "192.0.2.10"}); err != nil {
		t.Fatal(err)
	}
	router.POST("/api/servers/:id/sing-box/import", NewServerHandler(make([]byte, 32), clusterservice.NewService(st, nil)).SingBoxImport)
	request := httptest.NewRequest(http.MethodPost, "/api/servers/1/sing-box/import", strings.NewReader(`{"url":"http://example.com/config.json"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "HTTPS") {
		t.Fatalf("invalid URL response = %d %s", response.Code, response.Body.String())
	}
}

func TestSingBoxInstallRejectsSelfAsDownloadEgress(t *testing.T) {
	router, st := newClusterTestRouter(t)
	if err := st.CreateServer(&model.Server{Name: "node", Host: "192.0.2.10"}); err != nil {
		t.Fatal(err)
	}
	router.POST("/api/servers/:id/sing-box/install", NewServerHandler(make([]byte, 32), clusterservice.NewService(st, nil)).SingBoxInstall)
	request := httptest.NewRequest(http.MethodPost, "/api/servers/1/sing-box/install", strings.NewReader(`{"egress_server_id":1}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "其他服务器") {
		t.Fatalf("self egress response = %d %s", response.Code, response.Body.String())
	}
}
