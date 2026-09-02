package api

import (
	"crypto/sha256"
	"encoding/hex"
	authapi "github.com/cylism/cylism-manager/internal/api/auth"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestRegisterRoutesSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := store.New("file:router-snapshot?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	r := gin.New()
	RegisterRoutes(r, db, make([]byte, 32), &authapi.AuthConfig{
		JWTSecret:       []byte("router-snapshot-secret"),
		AccessTokenTTL:  time.Hour,
		RefreshTokenTTL: 24 * time.Hour,
	}, nil, KubernetesDependencies{})

	want := []string{
		"GET /health",
		"POST /api/auth/login",
		"POST /api/auth/temporary-login",
		"GET /api/auth/temporary-tokens",
		"POST /api/auth/temporary-tokens",
		"DELETE /api/auth/temporary-tokens/:id",
		"GET /api/monitoring/query",
		"GET /api/monitoring/alerts/overview",
		"POST /api/monitoring/logs/query",
		"GET /api/system-components",
		"GET /api/applications",
		"GET /api/k8s/persistent-volume-claims",
		"GET /api/tailscale/status",
		"GET /api/audit-logs",
	}
	registered := make(map[string]struct{}, len(r.Routes()))
	for _, route := range r.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}
	for _, route := range want {
		if _, ok := registered[route]; !ok {
			t.Errorf("missing route %s", route)
		}
	}
	if got := routeSnapshotDigest(r); got.count != 312 || got.digest != "7d4d473b9d334363b67939f8d44f45ae243d7ebe1aecaf5e3bc6ce1444e7c76a" {
		t.Fatalf("full route snapshot changed: count=%d digest=%s", got.count, got.digest)
	}
}

type routeSnapshot struct {
	count  int
	digest string
}

// routeSnapshotDigest covers the complete registered method/path set rather
// than only a handful of representative routes. Sorting makes the assertion
// independent of Gin's registration order while the digest keeps the golden
// snapshot compact and reviewable.
func routeSnapshotDigest(r *gin.Engine) routeSnapshot {
	paths := make([]string, 0, len(r.Routes()))
	for _, route := range r.Routes() {
		paths = append(paths, route.Method+" "+route.Path)
	}
	sort.Strings(paths)
	digest := sha256.Sum256([]byte(strings.Join(paths, "\n")))
	return routeSnapshot{count: len(paths), digest: hex.EncodeToString(digest[:])}
}

func TestRegisterRoutesDoesNotFallbackUnknownAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := store.New("file:router-fallback?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	r := gin.New()
	RegisterRoutes(r, db, make([]byte, 32), &authapi.AuthConfig{JWTSecret: []byte("router-fallback-secret")}, nil, KubernetesDependencies{})
	req := httptest.NewRequest(http.MethodGet, "/api/route-that-does-not-exist", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.Code)
	}
}
