package crypto

import (
	"crypto/x509"
	"os"
	"testing"
)

func TestGenerateCA(t *testing.T) {
	certPEM, keyPEM, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	if len(certPEM) == 0 {
		t.Error("certPEM is empty")
	}
	if len(keyPEM) == 0 {
		t.Error("keyPEM is empty")
	}

	cert, err := parseCert(certPEM)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	if !cert.IsCA {
		t.Error("expected IsCA=true")
	}
	if cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Error("expected KeyUsageCertSign")
	}
}

func TestIssueCert(t *testing.T) {
	caCertPEM, caKeyPEM, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}

	certPEM, keyPEM, err := IssueCert(caCertPEM, caKeyPEM, "agent-1")
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		t.Error("cert or key is empty")
	}

	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(caCertPEM)

	issuedCert, err := parseCert(certPEM)
	if err != nil {
		t.Fatalf("parse issued cert: %v", err)
	}

	opts := x509.VerifyOptions{Roots: roots}
	if _, err := issuedCert.Verify(opts); err != nil {
		t.Fatalf("issued cert not verified by CA: %v", err)
	}

	if issuedCert.Subject.CommonName != "agent-1" {
		t.Errorf("expected CN 'agent-1', got '%s'", issuedCert.Subject.CommonName)
	}
}

func TestIssueCert_InvalidCAKey(t *testing.T) {
	caCertPEM, _, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}

	_, wrongKeyPEM, _ := GenerateCA()
	_, _, err = IssueCert(caCertPEM, wrongKeyPEM, "test")
	if err == nil {
		t.Error("expected error for mismatched CA key")
	}
}

func TestSaveAndLoadTLSFiles(t *testing.T) {
	caCertPEM, caKeyPEM, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(dir+"/ca-cert.pem", caCertPEM, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/ca-key.pem", caKeyPEM, 0600); err != nil {
		t.Fatal(err)
	}

	loadedCert, _ := os.ReadFile(dir + "/ca-cert.pem")
	if string(loadedCert) != string(caCertPEM) {
		t.Error("cert roundtrip mismatch")
	}
}
