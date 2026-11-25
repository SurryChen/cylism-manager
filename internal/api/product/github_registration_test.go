package productapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/service/githubapp"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

type registrationGitHubClient struct {
	githubClient
	convert func(context.Context, string) (githubapp.AppRegistration, error)
}

func (client registrationGitHubClient) ConvertManifest(ctx context.Context, code string) (githubapp.AppRegistration, error) {
	return client.convert(ctx, code)
}

func TestGitHubManifestRegistration(t *testing.T) {
	repo, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	encKey := make([]byte, 32)
	h := NewHandler(repo).WithGitHub(repo, encKey, "https://manager.example.com/")
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	app := githubapp.AppRegistration{App: githubapp.App{ID: 42, Slug: "test-app"}, PrivateKey: keyPEM}
	app.Owner.Login, app.Owner.Type = "project-org", "Organization"
	conversions := 0
	h.github.client = registrationGitHubClient{
		convert: func(_ context.Context, code string) (githubapp.AppRegistration, error) {
			conversions++
			if code != "sample-registration-code" {
				t.Fatalf("unexpected conversion code %q", code)
			}
			return app, nil
		},
	}
	router := gin.New()
	router.POST("/start", func(c *gin.Context) {
		c.Set("user_id", uint(7))
		h.StartGitHubRegistration(c)
	})
	router.GET(GitHubManifestCallbackPath, h.GitHubManifestCallback)
	router.GET(GitHubInstallationReturnPath, GitHubInstallationReturn)
	startRequest := httptest.NewRequest(http.MethodPost, "/start", strings.NewReader(`{"organization":"Project-Org"}`))
	startRequest.Header.Set("Origin", "https://manager.example.com")
	start := httptest.NewRecorder()
	router.ServeHTTP(start, startRequest)
	if start.Code != http.StatusOK {
		t.Fatalf("start: %d %s", start.Code, start.Body.String())
	}
	var payload struct {
		Data struct {
			Action   string
			Manifest struct {
				RedirectURL string            `json:"redirect_url"`
				SetupURL    string            `json:"setup_url"`
				Public      bool              `json:"public"`
				Permissions map[string]string `json:"default_permissions"`
			}
		}
	}
	if err := json.Unmarshal(start.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	action, err := url.Parse(payload.Data.Action)
	if err != nil || action.Scheme != "https" || action.Host != "github.com" || action.Path != "/organizations/Project-Org/settings/apps/new" {
		t.Fatalf("unexpected GitHub form action: %s", payload.Data.Action)
	}
	manifest := payload.Data.Manifest
	if manifest.RedirectURL != "https://manager.example.com"+GitHubManifestCallbackPath || manifest.SetupURL != "https://manager.example.com"+GitHubInstallationReturnPath ||
		manifest.Public || len(manifest.Permissions) != 3 || manifest.Permissions["contents"] != "write" || manifest.Permissions["actions"] != "write" || manifest.Permissions["pull_requests"] != "write" {
		t.Fatalf("unexpected GitHub manifest: %+v", manifest)
	}
	cookies := start.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != githubRegistrationCookie || cookies[0].Path != "/" || cookies[0].Domain != "" ||
		!cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected registration cookie: %v", cookies)
	}
	state := action.Query().Get("state")
	if len(state) != 43 || state == cookies[0].Value {
		t.Fatal("callback state and browser verifier must be separate")
	}
	callback := func(state string, cookie *http.Cookie) *httptest.ResponseRecorder {
		query := url.Values{"state": {state}, "code": {"sample-registration-code"}}
		req := httptest.NewRequest(http.MethodGet, GitHubManifestCallbackPath+"?"+query.Encode(), nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	for _, denied := range []*httptest.ResponseRecorder{
		callback(state, nil), callback(strings.Repeat("x", len(state)), cookies[0]),
	} {
		if denied.Code != http.StatusSeeOther || denied.Header().Get("Location") != githubSettingsPath+"&github_setup=expired" {
			t.Fatalf("unverified callback: %d %s", denied.Code, denied.Header().Get("Location"))
		}
	}
	if conversions != 0 {
		t.Fatal("called GitHub for an unverified callback")
	}
	result := callback(state, cookies[0])
	if result.Code != http.StatusSeeOther || result.Header().Get("Location") != "https://github.com/apps/test-app/installations/new" ||
		result.Header().Get("Referrer-Policy") != "no-referrer" || strings.Contains(result.Body.String(), keyPEM) {
		t.Fatalf("unexpected callback: %d %s", result.Code, result.Header().Get("Location"))
	}
	connection, err := repo.GitHubConnection(7)
	if err != nil || connection.AppID != 42 || connection.Slug != "test-app" || connection.EncryptedKey == keyPEM {
		t.Fatalf("registration was not saved for its initiator: %v", err)
	}
	if plaintext, err := security.Decrypt(encKey, connection.EncryptedKey); err != nil || plaintext != keyPEM {
		t.Fatal("platform cannot read the encrypted GitHub credential")
	}
	if _, err := repo.GitHubConnection(8); !errors.Is(err, model.ErrGitHubNotConfigured) {
		t.Fatalf("registration attached to another account: %v", err)
	}
	if repeat := callback(state, cookies[0]); repeat.Header().Get("Location") != githubSettingsPath+"&github_setup=expired" || conversions != 1 {
		t.Fatalf("callback replay was accepted: %s", repeat.Header().Get("Location"))
	}
	finished := httptest.NewRecorder()
	router.ServeHTTP(finished, httptest.NewRequest(http.MethodGet, GitHubInstallationReturnPath+"?installation_id=601&next=https://example.org", nil))
	if finished.Code != http.StatusSeeOther || finished.Header().Get("Location") != githubSettingsPath {
		t.Fatalf("installation return did not go back to settings: %d %s", finished.Code, finished.Header().Get("Location"))
	}
}
