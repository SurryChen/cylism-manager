package network

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crypto "github.com/cylism/cylism-manager/internal/security"
	networkservice "github.com/cylism/cylism-manager/internal/service/network"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func setupCertRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &CertHandler{network: networkservice.NewService(nil)}
	cert := r.Group("/api/certs")
	{
		cert.GET("/status", h.Status)
		cert.POST("/install", h.Install)
		cert.GET("", h.ListCerts)
		cert.POST("", h.CreateCert)
		cert.DELETE("/:namespace/:name", h.DeleteCert)
	}
	return r
}

func TestCertHandler_StatusReportsUnavailableWithoutKubernetesClient(t *testing.T) {
	r := setupCertRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/certs/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "unavailable") {
		t.Fatalf("expected unavailable status, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCertHandler_ListCerts(t *testing.T) {
	r := setupCertRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/certs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestCertHandler_DeleteCert(t *testing.T) {
	r := setupCertRouter()
	req := httptest.NewRequest(http.MethodDelete, "/api/certs/default/test-cert", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestDNSCredentialRequestEncryptsSecretAndKeepsItWhenBlank(t *testing.T) {
	st, err := store.New(t.TempDir() + "/cylism.db")
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("01234567890123456789012345678901")
	h := NewCertHandlerWithComposedDependencies(st, key, nil, nil)
	secret, identifier := "aliyun-secret", "LTAI"
	credential, values, err := h.dnsCredentialFromRequest(dnsCredentialRequest{Name: "aliyun", Namespace: "cert-manager", Provider: "alidns", Values: map[string]*string{"access_key_id": &identifier, "access_key_secret": &secret}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if values["access_key_secret"] != secret || credential.EncryptedValues == secret {
		t.Fatalf("secret was not protected: %#v", credential)
	}
	credential.SecretName = "cylism-dns-alidns-1"
	if err := st.CreateDNSCredential(credential); err != nil {
		t.Fatal(err)
	}
	newIdentifier := "LTAI-2"
	updated, values, err := h.dnsCredentialFromRequest(dnsCredentialRequest{Name: "aliyun", Namespace: "cert-manager", Provider: "alidns", Values: map[string]*string{"access_key_id": &newIdentifier}}, credential)
	if err != nil {
		t.Fatal(err)
	}
	if values["access_key_secret"] != secret {
		t.Fatalf("blank update should retain the secret, got %q", values["access_key_secret"])
	}
	decrypted, err := crypto.Decrypt(key, updated.EncryptedValues)
	if err != nil || decrypted == secret {
		t.Fatalf("unexpected encrypted credential: %q, %v", decrypted, err)
	}
}

func TestDNSCredentialRequestRejectsUnknownProviderField(t *testing.T) {
	st, err := store.New(t.TempDir() + "/cylism.db")
	if err != nil {
		t.Fatal(err)
	}
	h := NewCertHandlerWithComposedDependencies(st, []byte("01234567890123456789012345678901"), nil, nil)
	value := "unexpected"
	_, _, err = h.dnsCredentialFromRequest(dnsCredentialRequest{Name: "aliyun", Namespace: "cert-manager", Provider: "alidns", Values: map[string]*string{"unknown": &value}}, nil)
	if err == nil {
		t.Fatal("expected unknown provider field to be rejected")
	}
}
