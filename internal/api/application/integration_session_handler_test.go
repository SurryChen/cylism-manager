package applicationapi

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/application"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

func TestIntegrationHandoffOnlyAllowsProtectedConsoleAndLoopbackDebugURL(t *testing.T) {
	r, s := setupApplicationRouter()
	app := createApplicationForReleaseRuntimeTest(t, s)
	protected := &model.ApplicationEndpoint{ApplicationID: app.ID, Exposure: application.ExposurePublic, Domain: "console.example.com", Path: "/", TLSEnabled: true, ServicePort: 80, AccessMode: model.ApplicationEndpointAccessProtectedConsole}
	if err := s.CreateApplicationEndpoint(protected); err != nil {
		t.Fatalf("create protected endpoint: %v", err)
	}
	public := &model.ApplicationEndpoint{ApplicationID: app.ID, Exposure: application.ExposurePublic, Domain: "api.example.com", Path: "/", TLSEnabled: true, ServicePort: 80, AccessMode: model.ApplicationEndpointAccessPublic}
	if err := s.CreateApplicationEndpoint(public); err != nil {
		t.Fatalf("create public endpoint: %v", err)
	}

	local := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/integration-handoffs", gin.H{"endpoint_id": protected.ID, "redirect_url": "http://localhost:5178/console?source=test"}))
	if local.Code != http.StatusOK || !strings.Contains(local.Body.String(), "http://localhost:5178/console?handoff_code=") || !strings.Contains(local.Body.String(), "source=test") {
		t.Fatalf("unexpected local handoff response: %d %s", local.Code, local.Body.String())
	}

	nonLoopback := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/integration-handoffs", gin.H{"endpoint_id": protected.ID, "redirect_url": "https://example.com"}))
	if nonLoopback.Code != http.StatusBadRequest || !strings.Contains(nonLoopback.Body.String(), "本地联调地址无效") {
		t.Fatalf("unexpected non-loopback response: %d %s", nonLoopback.Code, nonLoopback.Body.String())
	}

	publicResult := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/integration-handoffs", gin.H{"endpoint_id": public.ID}))
	if publicResult.Code != http.StatusBadRequest || !strings.Contains(publicResult.Body.String(), "不是受保护控制台") {
		t.Fatalf("unexpected public endpoint response: %d %s", publicResult.Code, publicResult.Body.String())
	}
}

func TestIntegrationSessionURLHelpers(t *testing.T) {
	redirect, err := parseLoopbackRedirectURL("http://[::1]:8080/console")
	if err != nil || redirect.Hostname() != "::1" {
		t.Fatalf("expected IPv6 loopback redirect, got %v, %v", redirect, err)
	}
	if _, err := parseLoopbackRedirectURL("https://example.com"); err == nil {
		t.Fatal("expected non-loopback redirect to be rejected")
	}
	withCode, err := appendHandoffCode("https://console.example.com/path?source=test", "opaque")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(withCode)
	if err != nil || u.Query().Get("handoff_code") != "opaque" || u.Query().Get("source") != "test" {
		t.Fatalf("handoff code was not appended safely: %s", withCode)
	}
	if opaqueHash("value") == opaqueHash("other") || opaqueHash("") == "" {
		t.Fatal("opaque hash should be deterministic and non-empty")
	}
}
