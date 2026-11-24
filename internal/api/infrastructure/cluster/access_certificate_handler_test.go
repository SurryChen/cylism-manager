package cluster

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestAccessCertificateAPIKeepsPrivateKeysServerSide(t *testing.T) {
	gin.SetMode(gin.TestMode)
	st, err := store.New(filepath.Join(t.TempDir(), "certs.db"))
	if err != nil {
		t.Fatal(err)
	}
	cluster := &model.Cluster{Name: "test", Distribution: "k3s", Status: "draft"}
	if err := st.CreateCluster(cluster); err != nil {
		t.Fatal(err)
	}
	h := NewAccessCertificateHandler(st, []byte("12345678901234567890123456789012"))
	router := gin.New()
	router.GET("/access-certificates/ca", h.CA)
	router.POST("/access-certificates/ca", h.InitializeCA)
	router.GET("/access-certificates", h.List)
	router.POST("/access-certificates", h.Issue)
	router.GET("/access-certificates/:id", h.Certificate)
	router.POST("/access-certificates/:id/revoke", h.Revoke)
	router.DELETE("/access-certificates/:id", h.Delete)
	router.GET("/access-certificates/crl", h.CRL)

	response := serve(router, newJSONRequest(http.MethodPost, "/access-certificates", map[string]any{"cluster_id": cluster.ID, "kind": "platform_client"}))
	if response.Code != http.StatusConflict {
		t.Fatalf("issue without CA = %d %s", response.Code, response.Body.String())
	}
	response = serve(router, newJSONRequest(http.MethodPost, "/access-certificates/ca", nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "encrypted_private_key") || strings.Contains(response.Body.String(), "PRIVATE KEY") {
		t.Fatalf("CA response = %d %s", response.Code, response.Body.String())
	}
	response = serve(router, newJSONRequest(http.MethodPost, "/access-certificates", map[string]any{"cluster_id": cluster.ID, "kind": "platform_client"}))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "private_key") {
		t.Fatalf("issue response = %d %s", response.Code, response.Body.String())
	}
	var result struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	response = serve(router, newJSONRequest(http.MethodGet, "/access-certificates", nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "private_key") {
		t.Fatalf("list response = %d %s", response.Code, response.Body.String())
	}
	response = serve(router, newJSONRequest(http.MethodGet, "/access-certificates/1", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "BEGIN CERTIFICATE") || strings.Contains(response.Body.String(), "PRIVATE KEY") {
		t.Fatalf("certificate response = %d %s", response.Code, response.Body.String())
	}
	response = serve(router, newJSONRequest(http.MethodPost, "/access-certificates/1/revoke", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("revoke response = %d %s", response.Code, response.Body.String())
	}
	response = serve(router, newJSONRequest(http.MethodGet, "/access-certificates/crl", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "BEGIN X509 CRL") {
		t.Fatalf("CRL response = %d %s", response.Code, response.Body.String())
	}
}

func TestAccessCertificateDeleteOnlyAllowsUnusedCertificates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	st, err := store.New(filepath.Join(t.TempDir(), "cert-delete.db"))
	if err != nil {
		t.Fatal(err)
	}
	cluster := &model.Cluster{Name: "delete-test", Distribution: "k3s", Status: "draft"}
	if err := st.CreateCluster(cluster); err != nil {
		t.Fatal(err)
	}
	h := NewAccessCertificateHandler(st, []byte("12345678901234567890123456789012"))
	if _, err := h.Service().InitializeCA(); err != nil {
		t.Fatal(err)
	}
	cert, err := h.Service().Issue(cluster.ID, "platform_client", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Service().MarkInUse(cert.ID, true); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.DELETE("/access-certificates/:id", h.Delete)
	response := serve(router, newJSONRequest(http.MethodDelete, "/access-certificates/"+fmt.Sprint(cert.ID), nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("delete in-use certificate = %d %s", response.Code, response.Body.String())
	}
	if err := h.Service().MarkInUse(cert.ID, false); err != nil {
		t.Fatal(err)
	}
	response = serve(router, newJSONRequest(http.MethodDelete, "/access-certificates/"+fmt.Sprint(cert.ID), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("delete unused certificate = %d %s", response.Code, response.Body.String())
	}
}

func TestAccessCertificateIssueRequiresExistingCluster(t *testing.T) {
	gin.SetMode(gin.TestMode)
	st, err := store.New(filepath.Join(t.TempDir(), "certs.db"))
	if err != nil {
		t.Fatal(err)
	}
	h := NewAccessCertificateHandler(st, []byte("12345678901234567890123456789012"))
	router := gin.New()
	router.POST("/access-certificates", h.Issue)
	response := serve(router, newJSONRequest(http.MethodPost, "/access-certificates", map[string]any{"cluster_id": 999, "kind": "platform_client"}))
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing cluster = %d %s", response.Code, response.Body.String())
	}
}
