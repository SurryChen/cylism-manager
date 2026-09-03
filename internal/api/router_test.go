package api_test

import (
	api "github.com/cylism/cylism-manager/internal/api"
	"github.com/cylism/cylism-manager/internal/bootstrap"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRegisterRoutesSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	container, err := bootstrap.NewContainer(bootstrap.Config{DBPath: "file:router-snapshot?mode=memory&cache=shared", EncryptionKey: make([]byte, 32), JWTSecret: []byte("router-snapshot-secret"), AccessTokenTTL: time.Hour, RefreshTokenTTL: 24 * time.Hour})
	if err != nil {
		t.Fatalf("create container: %v", err)
	}
	api.RegisterRoutes(r, container.BuildRouteDependencies())

	actual := routePaths(r)
	const snapshotPath = "testdata/routes.golden"
	if os.Getenv("UPDATE_ROUTE_SNAPSHOT") == "1" {
		if err := os.WriteFile(snapshotPath, []byte(strings.Join(actual, "\n")+"\n"), 0o644); err != nil {
			t.Fatalf("write route snapshot: %v", err)
		}
		return
	}
	wantSnapshot, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read route snapshot: %v", err)
	}
	expected := strings.FieldsFunc(string(wantSnapshot), func(r rune) bool { return r == '\n' || r == '\r' })
	if missing, unexpected := routeDiff(expected, actual); len(missing) > 0 || len(unexpected) > 0 {
		t.Fatalf("route snapshot changed:\nmissing: %s\nunexpected: %s", strings.Join(missing, ", "), strings.Join(unexpected, ", "))
	}
}

// routePaths returns a sorted, human-reviewable snapshot of the complete
// registered method/path set. UPDATE_ROUTE_SNAPSHOT=1 intentionally refreshes
// its golden file when a route change has been reviewed.
func routePaths(r *gin.Engine) []string {
	paths := make([]string, 0, len(r.Routes()))
	for _, route := range r.Routes() {
		paths = append(paths, route.Method+" "+route.Path)
	}
	sort.Strings(paths)
	return paths
}

func routeDiff(want, actual []string) (missing, unexpected []string) {
	wantSet := make(map[string]struct{}, len(want))
	for _, route := range want {
		wantSet[route] = struct{}{}
	}
	actualSet := make(map[string]struct{}, len(actual))
	for _, route := range actual {
		actualSet[route] = struct{}{}
	}
	for _, route := range want {
		if _, ok := actualSet[route]; !ok {
			missing = append(missing, route)
		}
	}
	for _, route := range actual {
		if _, ok := wantSet[route]; !ok {
			unexpected = append(unexpected, route)
		}
	}
	return missing, unexpected
}

func TestRegisterRoutesDoesNotFallbackUnknownAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	container, err := bootstrap.NewContainer(bootstrap.Config{DBPath: "file:router-fallback?mode=memory&cache=shared", EncryptionKey: make([]byte, 32), JWTSecret: []byte("router-fallback-secret")})
	if err != nil {
		t.Fatalf("create container: %v", err)
	}
	api.RegisterRoutes(r, container.BuildRouteDependencies())
	req := httptest.NewRequest(http.MethodGet, "/api/route-that-does-not-exist", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.Code)
	}
}
