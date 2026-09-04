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
