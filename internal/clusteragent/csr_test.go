package clusteragent

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateCSRKeepsPrivateKeyLocal(t *testing.T) {
	dir := t.TempDir()
	csrPEM, err := GenerateCSR("control.example.internal", filepath.Join(dir, "agent.key"), filepath.Join(dir, "agent.csr"))
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode([]byte(csrPEM))
	req, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := req.CheckSignature(); err != nil {
		t.Fatal(err)
	}
	if len(req.DNSNames) != 1 || req.DNSNames[0] != "control.example.internal" {
		t.Fatalf("SAN = %#v", req.DNSNames)
	}
	info, err := os.Stat(filepath.Join(dir, "agent.key"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("key mode = %o", info.Mode().Perm())
	}
}

func TestGenerateCSRSupportsIP(t *testing.T) {
	dir := t.TempDir()
	csrPEM, err := GenerateCSR("10.0.0.12", filepath.Join(dir, "agent.key"), filepath.Join(dir, "agent.csr"))
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode([]byte(csrPEM))
	req, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.IPAddresses) != 1 || req.IPAddresses[0].String() != "10.0.0.12" {
		t.Fatalf("SAN = %#v", req.IPAddresses)
	}
	if _, ok := req.PublicKey.(*ecdsa.PublicKey); !ok {
		t.Fatalf("key type = %T", req.PublicKey)
	}
}
