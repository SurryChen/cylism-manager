package clusteragent

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"strings"
)

// GenerateCSR creates an ECDSA P-256 key and a CSR containing exactly one DNS
// name or IP address. The private key stays on the agent host.
func GenerateCSR(hostname, keyPath, csrPath string) (string, error) {
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return "", fmt.Errorf("hostname is required")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", err
	}
	template := &x509.CertificateRequest{Subject: pkix.Name{CommonName: hostname}}
	if ip := net.ParseIP(hostname); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{hostname}
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, template, key)
	if err != nil {
		return "", err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
	if err := writePrivate(keyPath, keyPEM); err != nil {
		return "", err
	}
	if err := os.WriteFile(csrPath, csrPEM, 0600); err != nil {
		return "", err
	}
	return string(csrPEM), nil
}

func writePrivate(path string, data []byte) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("key path is required")
	}
	if err := os.MkdirAll(filepathDir(path), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return nil
}

func filepathDir(path string) string {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		if i == 0 {
			return "/"
		}
		return path[:i]
	}
	return "."
}
