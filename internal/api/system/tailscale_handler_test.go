package system

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	crypto "github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

type fakeTailscaleRuntime struct {
	installed bool
	outputs   map[string][]byte
	calls     []string
}

func (f *fakeTailscaleRuntime) Installed(context.Context) bool { return f.installed }
func (f *fakeTailscaleRuntime) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	return f.outputs[name+" "+strings.Join(args, " ")], nil
}
func (f *fakeTailscaleRuntime) Install(context.Context) ([]byte, error) {
	f.installed = true
	f.calls = append(f.calls, "install")
	return nil, nil
}
func (f *fakeTailscaleRuntime) ReadFile(context.Context, string) ([]byte, error) {
	return []byte("token"), nil
}

func TestTailscaleStatusUsesRuntimeAdapter(t *testing.T) {
	runtime := &fakeTailscaleRuntime{installed: true, outputs: map[string][]byte{"tailscale ip -4": []byte("100.64.0.10\n"), "tailscale status": []byte("peer online\n")}}
	h := NewTailscaleHandler(nil, make([]byte, 32)).WithRuntime(runtime)
	recorder := httptest.NewRecorder()
	c := gin.CreateTestContextOnly(recorder, gin.New())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(context.Background())
	h.Status(c)
	if !strings.Contains(recorder.Body.String(), "100.64.0.10") {
		t.Fatalf("unexpected status response: %s", recorder.Body.String())
	}
}

func TestTailscaleStatusStripsWarningsFromIPOutput(t *testing.T) {
	runtime := &fakeTailscaleRuntime{installed: true, outputs: map[string][]byte{
		"tailscale ip -4":  []byte("Warning: client version mismatch\n100.64.0.10\n"),
		"tailscale status": []byte("peer online\n"),
	}}
	h := NewTailscaleHandler(nil, make([]byte, 32)).WithRuntime(runtime)
	recorder := httptest.NewRecorder()
	c := gin.CreateTestContextOnly(recorder, gin.New())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(context.Background())
	h.Status(c)
	if !strings.Contains(recorder.Body.String(), `"ip":"100.64.0.10"`) {
		t.Fatalf("unexpected sanitized status response: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "Warning: client version mismatch") {
		t.Fatalf("warning leaked into status response: %s", recorder.Body.String())
	}
}

type systemConfigFake struct{ values map[string]string }

func (f *systemConfigFake) GetSystemConfig(key string) (string, error) {
	value, ok := f.values[key]
	if !ok {
		return "", os.ErrNotExist
	}
	return value, nil
}
func (f *systemConfigFake) SetSystemConfig(key, value string) error {
	if f.values == nil {
		f.values = map[string]string{}
	}
	f.values[key] = value
	return nil
}

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

func TestTailscaleHandlerReadsConfigThroughRepository(t *testing.T) {
	key := make([]byte, 32)
	encrypted, err := crypto.Encrypt(key, "tskey-auth-example-token")
	if err != nil {
		t.Fatal(err)
	}
	configs := &systemConfigFake{values: map[string]string{"tailscale_auth_key": encrypted}}
	router := gin.New()
	router.GET("/install-script", NewTailscaleHandler(configs, key).InstallScript)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/install-script", nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "example-token") {
		t.Fatalf("unexpected config-backed response: %d %s", response.Code, response.Body.String())
	}
}
