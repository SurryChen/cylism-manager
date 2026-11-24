package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/gin-gonic/gin"
)

func TestRegisterFrontendRoutesKeepsAPINotFoundAsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("spa"), 0600); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	registerFrontendRoutes(r, dist)

	req := httptest.NewRequest(http.MethodGet, "/api/missing", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.Code)
	}
	var body apiShared.APIResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode API error: %v", err)
	}
	if body.Code != apiShared.CodeNotFound {
		t.Fatalf("code = %d, want %d", body.Code, apiShared.CodeNotFound)
	}
}

func TestRegisterFrontendRoutesKeepsSPAFallbackForBrowserRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("spa"), 0600); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	registerFrontendRoutes(r, dist)

	req := httptest.NewRequest(http.MethodGet, "/applications/1", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK || resp.Body.String() != "spa" {
		t.Fatalf("browser fallback = %d %q", resp.Code, resp.Body.String())
	}
}

func TestFrontendDistDirOverrideDoesNotDependOnWorkingDirectory(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dist := t.TempDir()
	t.Setenv("CYLISM_WEB_DIST_DIR", dist)
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("new frontend"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	// Restore the process working directory before other tests run.
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	r := gin.New()
	registerFrontendRoutes(r, frontendDistDir())
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/", nil))
	if resp.Code != http.StatusOK || resp.Body.String() != "new frontend" {
		t.Fatalf("frontend = %d %q", resp.Code, resp.Body.String())
	}
}
