package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func setupTailscaleRouter() (*gin.Engine, *store.Store) {
	gin.SetMode(gin.TestMode)
	s, _ := store.New(":memory:")
	r := gin.New()
	h := NewTailscaleHandler(s, make([]byte, 32))
	tg := r.Group("/api/tailscale")
	{
		tg.POST("/init", h.Init)
		tg.GET("/status", h.Status)
		tg.GET("/install-script", h.InstallScript)
	}
	return r, s
}

func TestTailscale_Status(t *testing.T) {
	r, _ := setupTailscaleRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/tailscale/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Initialized bool   `json:"initialized"`
			IP          string `json:"ip"`
			Online      bool   `json:"online"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 0 {
		t.Errorf("expected code 0, got %d", resp.Code)
	}
}

func TestTailscale_InitBadRequest(t *testing.T) {
	r, _ := setupTailscaleRouter()
	body := `{"auth_key": "bad-key"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tailscale/init", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTailscale_InitNoAuthKey(t *testing.T) {
	r, _ := setupTailscaleRouter()
	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/tailscale/init", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTailscale_InstallScriptNoKey(t *testing.T) {
	r, _ := setupTailscaleRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/tailscale/install-script", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
