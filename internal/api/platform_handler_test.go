package api

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
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestPlatformWebhookAcceptsSignedDigestAndRejectsReplay(t *testing.T) {
	original := K8s
	defer func() { K8s = original }()
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "cylism-manager", Namespace: "default"}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "platform", Image: "registry.example.com/cylism-manager@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}}}})}
	secretKey := []byte("01234567890123456789012345678901")
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := crypto.Encrypt(secretKey, "webhook-secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSystemConfig(platformWebhookSecretConfigKey, encrypted); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSystemConfig(platformImagePrefixConfigKey, "registry.example.com/cylism-manager"); err != nil {
		t.Fatal(err)
	}
	handler := NewPlatformHandler(s, secretKey)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/platform/deployments", handler.Webhook)

	body := `{"image":"registry.example.com/cylism-manager@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","commit_sha":"abc"}`
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
	handler := NewPlatformHandler(nil, []byte("01234567890123456789012345678901"))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/platform/deployments", handler.Webhook)
	request := httptest.NewRequest(http.MethodPost, "/api/platform/deployments", strings.NewReader(`{"image":"registry.example.com/cylism-manager@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`))
	request.Header.Set("X-Cylism-Timestamp", "1")
	request.Header.Set("X-Cylism-Nonce", "1234567890abcdef")
	request.Header.Set("X-Cylism-Signature", "invalid")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized signature, got %d: %s", response.Code, response.Body.String())
	}
}

func TestPlatformManualUpdateAcceptsAuthenticatedDigest(t *testing.T) {
	original := K8s
	defer func() { K8s = original }()
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "cylism-manager", Namespace: "default"}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "platform", Image: "registry.example.com/cylism-manager@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}}}})}
	secretKey := []byte("01234567890123456789012345678901")
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSystemConfig(platformImagePrefixConfigKey, "registry.example.com/cylism-manager"); err != nil {
		t.Fatal(err)
	}
	handler := NewPlatformHandler(s, secretKey)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/platform/releases", handler.ManualUpdate)

	request := httptest.NewRequest(http.MethodPost, "/api/platform/releases", strings.NewReader(`{"image":"registry.example.com/cylism-manager@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"source":"manual"`) {
		t.Fatalf("expected accepted manual release, got %d: %s", response.Code, response.Body.String())
	}
}

func TestPlatformManualUpdateRejectsTag(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSystemConfig(platformImagePrefixConfigKey, "registry.example.com/cylism-manager"); err != nil {
		t.Fatal(err)
	}
	handler := NewPlatformHandler(s, []byte("01234567890123456789012345678901"))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/platform/releases", handler.ManualUpdate)
	request := httptest.NewRequest(http.MethodPost, "/api/platform/releases", strings.NewReader(`{"image":"registry.example.com/cylism-manager:latest"}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected tag rejection, got %d: %s", response.Code, response.Body.String())
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
