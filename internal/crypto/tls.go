package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"
)

// GenerateCA 生成自签名 CA 证书（ECDSA P256，10 年有效期）
func GenerateCA() (certPEM, keyPEM []byte, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "Cylism Manager CA",
			Organization: []string{"Cylism"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(3650 * 24 * time.Hour), // 10 years
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("create ca cert: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	privDER, _ := x509.MarshalECPrivateKey(priv)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER})

	return certPEM, keyPEM, nil
}

// IssueCert 由 CA 签发客户端证书（ECDSA P256，10 年有效期）
func IssueCert(caCertPEM, caKeyPEM []byte, cn string) (certPEM, keyPEM []byte, err error) {
	caCert, err := parseCert(caCertPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("parse ca cert: %w", err)
	}

	caKey, err := parseECKey(caKeyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("parse ca key: %w", err)
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName:   cn,
			Organization: []string{"Cylism Agent"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(3650 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &priv.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create cert: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	privDER, _ := x509.MarshalECPrivateKey(priv)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER})

	return certPEM, keyPEM, nil
}

// LoadOrGenerateCA 加载已有 CA 或生成新 CA
func LoadOrGenerateCA(certPath, keyPath string) (certPEM, keyPEM []byte, err error) {
	certPEM, err = os.ReadFile(certPath)
	if err == nil {
		keyPEM, err = os.ReadFile(keyPath)
		if err == nil {
			return certPEM, keyPEM, nil
		}
	}

	// Generate new CA
	certPEM, keyPEM, err = GenerateCA()
	if err != nil {
		return nil, nil, err
	}

	os.MkdirAll("data/tls", 0700)
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return nil, nil, fmt.Errorf("write ca cert: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return nil, nil, fmt.Errorf("write ca key: %w", err)
	}

	return certPEM, keyPEM, nil
}

func parseCert(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("failed to decode PEM certificate")
	}
	return x509.ParseCertificate(block.Bytes)
}

func parseECKey(keyPEM []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil || block.Type != "EC PRIVATE KEY" {
		return nil, fmt.Errorf("failed to decode PEM key")
	}
	return x509.ParseECPrivateKey(block.Bytes)
}
