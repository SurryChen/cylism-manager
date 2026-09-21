package kubernetes

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type stubPlatformIdentityReader struct {
	version string
	err     error
}

func (r stubPlatformIdentityReader) ServerVersionContext(context.Context) (string, error) {
	return r.version, r.err
}

func TestClusterPlatformHandlerClassifiesClusterAndCapabilities(t *testing.T) {
	tests := []struct {
		name              string
		reader            PlatformIdentityReader
		wantDistribution  string
		wantK3sCapability bool
		wantReason        string
	}{
		{name: "k3s", reader: stubPlatformIdentityReader{version: "v1.31.2+k3s1"}, wantDistribution: "k3s", wantK3sCapability: true},
		{name: "kubernetes", reader: stubPlatformIdentityReader{version: "v1.31.2"}, wantDistribution: "kubernetes"},
		{name: "unavailable", reader: stubPlatformIdentityReader{err: errors.New("dial tcp 10.0.0.1:6443: i/o timeout")}, wantDistribution: "unknown", wantReason: "unavailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/api/k8s/platform", NewClusterPlatformHandler(tt.reader).Get)
			response := serve(r, httptest.NewRequest(http.MethodGet, "/api/k8s/platform", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("Get status = %d: %s", response.Code, response.Body.String())
			}
			body := response.Body.String()
			for _, expected := range []string{
				`"distribution":"` + tt.wantDistribution + `"`,
				`"k3s_node_join":` + map[bool]string{true: "true", false: "false"}[tt.wantK3sCapability],
			} {
				if !strings.Contains(body, expected) {
					t.Fatalf("response missing %s: %s", expected, body)
				}
			}
			if tt.wantReason != "" && !strings.Contains(body, `"reason":"`+tt.wantReason+`"`) {
				t.Fatalf("response missing reason %q: %s", tt.wantReason, body)
			}
			if strings.Contains(body, "10.0.0.1") {
				t.Fatalf("response leaked discovery error: %s", body)
			}
			if strings.Contains(body, "k3s_vpn_diagnostics") {
				t.Fatalf("response exposes removed VPN diagnostics capability: %s", body)
			}
		})
	}
}
