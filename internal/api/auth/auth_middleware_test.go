package authapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	coreauth "github.com/cylism/cylism-manager/internal/auth"
	"github.com/gin-gonic/gin"
)

func TestJWTAuthMiddlewareAcceptsHeaderAndQueryToken(t *testing.T) {
	secret := []byte("secret")
	token, err := coreauth.GenerateAccessToken(secret, 7, "alice", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, header, path string }{
		{name: "header", header: "Bearer " + token, path: "/"},
		{name: "query", path: "/?token=" + token},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(JWTAuthMiddleware(secret))
			r.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			resp := httptest.NewRecorder()
			r.ServeHTTP(resp, req)
			if resp.Code != http.StatusNoContent {
				t.Fatalf("status = %d, body=%s", resp.Code, resp.Body.String())
			}
		})
	}
}

func TestJWTAuthMiddlewareRejectsMissingAndInvalidToken(t *testing.T) {
	for _, path := range []string{"/", "/?token=invalid"} {
		r := gin.New()
		r.Use(JWTAuthMiddleware([]byte("secret")))
		r.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))
		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("path %s status = %d", path, resp.Code)
		}
	}
}

func TestDelegationAuthMiddlewareRequiresDelegationToken(t *testing.T) {
	secret := []byte("secret")
	delegation, err := coreauth.GenerateDelegationToken(secret, coreauth.DelegationClaims{UserID: 7, Username: "alice", ProjectID: 9, Capability: "reader", Actions: []string{"application:read"}}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(DelegationAuthMiddleware(secret))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	valid := httptest.NewRequest(http.MethodGet, "/", nil)
	valid.Header.Set("Authorization", "Bearer "+delegation)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, valid)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("delegation status = %d, body=%s", resp.Code, resp.Body.String())
	}
	ordinary, err := coreauth.GenerateAccessToken(secret, 7, "alice", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	invalid := httptest.NewRequest(http.MethodGet, "/?token="+ordinary, nil)
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, invalid)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("ordinary JWT status = %d", resp.Code)
	}
}
