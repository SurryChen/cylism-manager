package delivery

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/crypto"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	platformservice "github.com/cylism/cylism-manager/internal/service/platform"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestPlatformWebhookAcceptsSignedTagAndRejectsReplay(t *testing.T) {
	client := platformTestClient()
	secretKey := []byte("01234567890123456789012345678901")
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := crypto.Encrypt(secretKey, "webhook-secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSystemConfig(platformservice.WebhookSecretConfigKey, encrypted); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSystemConfig(platformservice.ImagePrefixConfigKey, "registry.example.com/cylism-manager"); err != nil {
		t.Fatal(err)
	}
	handler := NewPlatformHandler(s, secretKey, client)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/platform/deployments", handler.Webhook)

	body := `{"image":"registry.example.com/cylism-manager:latest","commit_sha":"abc"}`
	timestamp := time.Now().Unix()
	nonce := "1234567890abcdef"
	request := signedPlatformWebhookRequest(body, timestamp, nonce, "webhook-secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"status":"accepted"`) {
		t.Fatalf("expected accepted release, got %d: %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, signedPlatformWebhookRequest(body, timestamp, nonce, "webhook-secret"))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected replay rejection, got %d: %s", response.Code, response.Body.String())
	}
}

func TestPlatformWebhookRejectsInvalidSignature(t *testing.T) {
	handler := NewPlatformHandler(nil, []byte("01234567890123456789012345678901"), nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/platform/deployments", handler.Webhook)
	request := httptest.NewRequest(http.MethodPost, "/api/platform/deployments", strings.NewReader(`{"image":"registry.example.com/cylism-manager:latest"}`))
	request.Header.Set("X-Cylism-Timestamp", "1")
	request.Header.Set("X-Cylism-Nonce", "1234567890abcdef")
	request.Header.Set("X-Cylism-Signature", "invalid")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized signature, got %d: %s", response.Code, response.Body.String())
	}
}

func TestPlatformManualUpdateAcceptsAuthenticatedTag(t *testing.T) {
	client := platformTestClient()
	secretKey := []byte("01234567890123456789012345678901")
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSystemConfig(platformservice.ImagePrefixConfigKey, "registry.example.com/cylism-manager"); err != nil {
		t.Fatal(err)
	}
	handler := NewPlatformHandler(s, secretKey, client)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/platform/releases", handler.ManualUpdate)

	request := httptest.NewRequest(http.MethodPost, "/api/platform/releases", strings.NewReader(`{"image":"registry.example.com/cylism-manager:latest"}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"source":"manual"`) {
		t.Fatalf("expected accepted manual release, got %d: %s", response.Code, response.Body.String())
	}
}

func TestPlatformManualUpdateRejectsUntaggedImage(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSystemConfig(platformservice.ImagePrefixConfigKey, "registry.example.com/cylism-manager"); err != nil {
		t.Fatal(err)
	}
	handler := NewPlatformHandler(s, []byte("01234567890123456789012345678901"), nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/platform/releases", handler.ManualUpdate)
	request := httptest.NewRequest(http.MethodPost, "/api/platform/releases", strings.NewReader(`{"image":"registry.example.com/cylism-manager"}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected tag rejection, got %d: %s", response.Code, response.Body.String())
	}
}

func TestPlatformImagePrefixesAllowMultipleRegistries(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSystemConfig(platformservice.ImagePrefixConfigKey, "registry.example.com/cylism-manager\noci-registry.example.com/cylism-manager"); err != nil {
		t.Fatal(err)
	}
	h := NewPlatformHandler(s, []byte("01234567890123456789012345678901"), nil)
	for _, image := range []string{
		"registry.example.com/cylism-manager:1.0.0",
		"oci-registry.example.com/cylism-manager:2.0.0",
	} {
		if err := h.release.ValidateImage(image); err != nil {
			t.Fatalf("expected image %q to be accepted: %v", image, err)
		}
	}
	if err := h.release.ValidateImage("other.example.com/cylism-manager:1.0.0"); err == nil {
		t.Fatal("expected image outside configured prefixes to be rejected")
	}
	prefixes, err := platformservice.NormalizeImagePrefixes("registry.example.com/cylism-manager, registry.example.com/cylism-manager\noci-registry.example.com/cylism-manager/")
	if err != nil || len(prefixes) != 2 || prefixes[1] != "oci-registry.example.com/cylism-manager" {
		t.Fatalf("unexpected normalized prefixes: %#v, %v", prefixes, err)
	}
}

func TestPlatformEndpointStatusDefaultsToNotConfigured(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewPlatformHandler(s, []byte("01234567890123456789012345678901"), nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/platform/endpoint", handler.EndpointStatus)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/platform/endpoint", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"state":"not_configured"`) {
		t.Fatalf("expected not configured endpoint status, got %d: %s", response.Code, response.Body.String())
	}
}

func platformTestClient() *k8sclient.Client {
	return &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "cylism-manager", Namespace: "default"}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "platform", Image: "registry.example.com/cylism-manager:latest"}}}}}})}
}

func TestValidPlatformHostname(t *testing.T) {
	for _, hostname := range []string{"console.example.com", "console-1.example.co.uk"} {
		if !validPlatformHostname(hostname) {
			t.Fatalf("expected valid hostname %q", hostname)
		}
	}
	for _, hostname := range []string{"", "localhost", "*.example.com", "console_example.com", "-console.example.com", "console-.example.com"} {
		if validPlatformHostname(hostname) {
			t.Fatalf("expected invalid hostname %q", hostname)
		}
	}
}

func TestCertificateCoversHostname(t *testing.T) {
	for _, test := range []struct {
		certificate string
		hostname    string
		covered     bool
	}{
		{"console.example.com", "console.example.com", true},
		{"*.example.com", "console.example.com", true},
		{"*.example.com", "api.console.example.com", false},
		{"*.example.com", "example.com", false},
		{"console.example.com", "api.example.com", false},
	} {
		if actual := certificateCoversHostname(test.certificate, test.hostname); actual != test.covered {
			t.Fatalf("certificateCoversHostname(%q, %q) = %t, want %t", test.certificate, test.hostname, actual, test.covered)
		}
	}
}

func signedPlatformWebhookRequest(body string, timestamp int64, nonce, secret string) *http.Request {
	payload := strings.Join([]string{strconv.FormatInt(timestamp, 10), nonce, body}, ".")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	request := httptest.NewRequest(http.MethodPost, "/api/platform/deployments", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Cylism-Timestamp", strconv.FormatInt(timestamp, 10))
	request.Header.Set("X-Cylism-Nonce", nonce)
	request.Header.Set("X-Cylism-Signature", hex.EncodeToString(mac.Sum(nil)))
	return request
}
