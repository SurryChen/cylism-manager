package accesscert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/store"
)

func TestIssueClusterBoundMutualTLSCertificatesAndCRL(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "access.db"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(st, []byte("12345678901234567890123456789012"))
	ca, err := service.InitializeCA()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ca.EncryptedPrivateKey, "BEGIN PRIVATE KEY") {
		t.Fatal("CA private key stored without encryption")
	}
	if _, err := service.InitializeCA(); err != store.ErrAccessCAExists {
		t.Fatalf("duplicate CA creation = %v", err)
	}
	csrKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "untrusted subject"}, IPAddresses: []net.IP{net.ParseIP("10.0.0.4")},
	}, csrKey)
	if err != nil {
		t.Fatal(err)
	}
	csr := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}))
	agent, err := service.Issue(7, AgentServer, csr)
	if err != nil {
		t.Fatal(err)
	}
	client, err := service.Issue(7, PlatformClient, "")
	if err != nil {
		t.Fatal(err)
	}
	if agent.EncryptedPrivateKey != "" || client.EncryptedPrivateKey == "" || strings.Contains(client.EncryptedPrivateKey, "BEGIN PRIVATE KEY") {
		t.Fatal("wrong private-key ownership or storage")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(ca.CertificatePEM)) {
		t.Fatal("invalid CA PEM")
	}
	for _, tc := range []struct {
		pem      string
		usage    x509.ExtKeyUsage
		identity string
	}{
		{agent.CertificatePEM, x509.ExtKeyUsageServerAuth, "spiffe://cylism.local/cluster/7/agent_server"},
		{client.CertificatePEM, x509.ExtKeyUsageClientAuth, "spiffe://cylism.local/cluster/7/platform_client"},
	} {
		block, _ := pem.Decode([]byte(tc.pem))
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := cert.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{tc.usage}}); err != nil {
			t.Fatal(err)
		}
		if len(cert.URIs) != 1 || cert.URIs[0].String() != tc.identity {
			t.Fatalf("certificate identity = %v", cert.URIs)
		}
	}
	block, _ := pem.Decode([]byte(agent.CertificatePEM))
	parsed, _ := x509.ParseCertificate(block.Bytes)
	if err := parsed.VerifyHostname("10.0.0.4"); err != nil {
		t.Fatal(err)
	}
	if err := st.RevokeAccessCertificate(agent.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	crlPEM, err := service.CRL()
	if err != nil {
		t.Fatal(err)
	}
	crlBlock, _ := pem.Decode([]byte(crlPEM))
	crl, err := x509.ParseRevocationList(crlBlock.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := crl.CheckSignatureFrom(mustParseCertificate(t, ca.CertificatePEM)); err != nil {
		t.Fatal(err)
	}
	if len(crl.RevokedCertificateEntries) != 1 || crl.RevokedCertificateEntries[0].SerialNumber.Cmp(parsed.SerialNumber) != 0 {
		t.Fatalf("CRL entries = %v", crl.RevokedCertificateEntries)
	}
}

func TestIssueRejectsCSRsWithoutOneValidEndpoint(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "access.db"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(st, []byte("12345678901234567890123456789012"))
	if _, err := service.InitializeCA(); err != nil {
		t.Fatal(err)
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	for _, names := range [][]string{nil, {"agent.example.com", "other.example.com"}, {"bad@name"}} {
		der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{DNSNames: names}, key)
		if err != nil {
			t.Fatal(err)
		}
		csr := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
		if _, err := service.Issue(1, AgentServer, csr); err == nil {
			t.Fatalf("accepted invalid CSR SAN: %v", names)
		}
	}
}

func TestIssueRejectsWeakAgentKey(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "access.db"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(st, []byte("12345678901234567890123456789012"))
	if _, err := service.InitializeCA(); err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{DNSNames: []string{"agent.example.com"}}, key)
	if err != nil {
		t.Fatal(err)
	}
	csr := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
	if _, err := service.Issue(1, AgentServer, csr); err == nil || !strings.Contains(err.Error(), "2048") {
		t.Fatalf("weak RSA CSR result = %v", err)
	}
}

func mustParseCertificate(t *testing.T, value string) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode([]byte(value))
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}
