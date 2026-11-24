package accesscert

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
)

const (
	AgentServer    = "agent_server"
	PlatformClient = "platform_client"
)

var dnsLabelPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

type Repository interface {
	GetAccessCA() (*model.AccessCA, error)
	CreateAccessCA(*model.AccessCA) error
	ListAccessCertificates() ([]model.AccessCertificate, error)
	CreateAccessCertificate(*model.AccessCertificate) error
}

type usageRepository interface {
	MarkAccessCertificateInUse(uint, bool) error
	MarkClusterAccessCertificatesUnused(uint, string) error
}

type Service struct {
	repo Repository
	key  []byte
}

// ClientTLSMaterial contains the certificate chain the platform uses when it
// connects to a cluster-agent. The private key is decrypted only in memory
// and is never returned by an HTTP handler.
type ClientTLSMaterial struct {
	CACertificatePEM string
	CertificatePEM   string
	PrivateKeyPEM    string
}

func NewService(repo Repository, key []byte) *Service {
	return &Service{repo: repo, key: append([]byte(nil), key...)}
}

// GetCA returns the platform trust anchor without exposing its private key.
func (s *Service) GetCA() (*model.AccessCA, error) { return s.repo.GetAccessCA() }

// PlatformClientTLS returns the active platform-client identity for a
// cluster, creating it on first use. This keeps certificate provisioning
// separate from the request path while allowing a newly deployed agent to be
// used immediately by cluster-scoped pages.
func (s *Service) PlatformClientTLS(clusterID uint) (ClientTLSMaterial, error) {
	if clusterID == 0 {
		return ClientTLSMaterial{}, errors.New("集群 ID 无效")
	}
	ca, err := s.repo.GetAccessCA()
	if err != nil {
		return ClientTLSMaterial{}, err
	}
	items, err := s.repo.ListAccessCertificates()
	if err != nil {
		return ClientTLSMaterial{}, err
	}
	var item *model.AccessCertificate
	now := time.Now()
	for index := range items {
		candidate := &items[index]
		if candidate.ClusterID == clusterID && candidate.Kind == PlatformClient && candidate.RevokedAt == nil && candidate.NotAfter.After(now) {
			if item == nil || candidate.CreatedAt.After(item.CreatedAt) {
				item = candidate
			}
		}
	}
	if item == nil {
		item, err = s.Issue(clusterID, PlatformClient, "")
		if err != nil {
			return ClientTLSMaterial{}, err
		}
	}
	if usage, ok := s.repo.(usageRepository); ok && !item.InUse {
		if err := usage.MarkAccessCertificateInUse(item.ID, true); err != nil {
			return ClientTLSMaterial{}, err
		}
	}
	plainKey, err := security.Decrypt(s.key, item.EncryptedPrivateKey)
	if err != nil {
		return ClientTLSMaterial{}, err
	}
	if strings.TrimSpace(item.CertificatePEM) == "" || strings.TrimSpace(plainKey) == "" {
		return ClientTLSMaterial{}, errors.New("平台客户端证书材料不完整")
	}
	return ClientTLSMaterial{CACertificatePEM: ca.CertificatePEM, CertificatePEM: item.CertificatePEM, PrivateKeyPEM: plainKey}, nil
}

// MarkInUse changes which certificate is currently installed or consumed for
// its cluster and purpose. Activating one certificate automatically marks
// older certificates of the same kind unused.
func (s *Service) MarkInUse(id uint, inUse bool) error {
	usage, ok := s.repo.(usageRepository)
	if !ok {
		return errors.New("访问证书使用状态存储未配置")
	}
	return usage.MarkAccessCertificateInUse(id, inUse)
}

func (s *Service) MarkClusterUnused(clusterID uint, kind string) error {
	usage, ok := s.repo.(usageRepository)
	if !ok {
		return errors.New("访问证书使用状态存储未配置")
	}
	return usage.MarkClusterAccessCertificatesUnused(clusterID, kind)
}

func (s *Service) InitializeCA() (*model.AccessCA, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	subjectKeyID := make([]byte, 20)
	if _, err = rand.Read(subjectKeyID); err != nil {
		return nil, err
	}
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "Cylism Cluster Access CA"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.AddDate(10, 0, 0),
		IsCA: true, BasicConstraintsValid: true, MaxPathLenZero: true,
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		SubjectKeyId: subjectKeyID,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	encrypted, err := security.Encrypt(s.key, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})))
	if err != nil {
		return nil, err
	}
	ca := &model.AccessCA{CertificatePEM: encodeCertificate(der), EncryptedPrivateKey: encrypted, Fingerprint: fingerprint(der), NotAfter: template.NotAfter}
	if err := s.repo.CreateAccessCA(ca); err != nil {
		return nil, err
	}
	return ca, nil
}

func (s *Service) Issue(clusterID uint, kind, csrPEM string) (*model.AccessCertificate, error) {
	ca, key, err := s.signer()
	if err != nil {
		return nil, err
	}
	if kind != AgentServer && kind != PlatformClient {
		return nil, errors.New("证书类型无效")
	}
	now := time.Now().UTC()
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	identity := fmt.Sprintf("spiffe://cylism.local/cluster/%d/%s", clusterID, kind)
	uri, _ := url.Parse(identity)
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: fmt.Sprintf("cylism-%s-%d", kind, clusterID)},
		NotBefore: now.Add(-time.Minute), NotAfter: now.AddDate(1, 0, 0),
		KeyUsage: x509.KeyUsageDigitalSignature, BasicConstraintsValid: true,
		URIs: []*url.URL{uri},
	}
	var publicKey any
	var encryptedKey string
	var endpoint string
	if kind == AgentServer {
		csr, err := parseAgentCSR(csrPEM)
		if err != nil {
			return nil, err
		}
		publicKey = csr.PublicKey
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		template.DNSNames = csr.DNSNames
		template.IPAddresses = csr.IPAddresses
		if len(csr.DNSNames) == 1 {
			endpoint = csr.DNSNames[0]
		} else {
			endpoint = csr.IPAddresses[0].String()
		}
	} else {
		if strings.TrimSpace(csrPEM) != "" {
			return nil, errors.New("平台客户端证书不需要 CSR")
		}
		clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		publicKey = &clientKey.PublicKey
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
		pkcs8, err := x509.MarshalPKCS8PrivateKey(clientKey)
		if err != nil {
			return nil, err
		}
		encryptedKey, err = security.Encrypt(s.key, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})))
		if err != nil {
			return nil, err
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, publicKey, key)
	if err != nil {
		return nil, err
	}
	item := &model.AccessCertificate{
		ClusterID: clusterID, Kind: kind, Serial: serial.Text(16), Subject: template.Subject.CommonName,
		Endpoint: endpoint, CertificatePEM: encodeCertificate(der), EncryptedPrivateKey: encryptedKey,
		Fingerprint: fingerprint(der), NotAfter: template.NotAfter,
	}
	if err := s.repo.CreateAccessCertificate(item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) CRL() (string, error) {
	ca, key, err := s.signer()
	if err != nil {
		return "", err
	}
	items, err := s.repo.ListAccessCertificates()
	if err != nil {
		return "", err
	}
	revoked := make([]x509.RevocationListEntry, 0)
	for _, item := range items {
		if item.RevokedAt == nil {
			continue
		}
		serial, ok := new(big.Int).SetString(item.Serial, 16)
		if !ok {
			return "", errors.New("证书序列号无效")
		}
		revoked = append(revoked, x509.RevocationListEntry{SerialNumber: serial, RevocationTime: *item.RevokedAt})
	}
	now := time.Now().UTC()
	der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number: big.NewInt(now.UnixNano()), ThisUpdate: now, NextUpdate: now.Add(24 * time.Hour), RevokedCertificateEntries: revoked,
	}, ca, key)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: der})), nil
}

func (s *Service) signer() (*x509.Certificate, *ecdsa.PrivateKey, error) {
	stored, err := s.repo.GetAccessCA()
	if err != nil {
		return nil, nil, err
	}
	certBlock, _ := pem.Decode([]byte(stored.CertificatePEM))
	if certBlock == nil {
		return nil, nil, errors.New("访问 CA 证书损坏")
	}
	ca, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	plain, err := security.Decrypt(s.key, stored.EncryptedPrivateKey)
	if err != nil {
		return nil, nil, err
	}
	keyBlock, _ := pem.Decode([]byte(plain))
	if keyBlock == nil {
		return nil, nil, errors.New("访问 CA 私钥损坏")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, nil, errors.New("访问 CA 私钥类型无效")
	}
	if time.Now().After(ca.NotAfter) {
		return nil, nil, errors.New("访问 CA 已过期")
	}
	return ca, key, nil
}

func parseAgentCSR(raw string) (*x509.CertificateRequest, error) {
	block, rest := pem.Decode([]byte(strings.TrimSpace(raw)))
	if block == nil || block.Type != "CERTIFICATE REQUEST" || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, errors.New("请输入 PEM 格式的证书签名请求")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil || csr.CheckSignature() != nil {
		return nil, errors.New("CSR 签名无效")
	}
	if len(csr.DNSNames)+len(csr.IPAddresses) != 1 || len(csr.EmailAddresses) != 0 || len(csr.URIs) != 0 {
		return nil, errors.New("Agent CSR 必须包含且只包含一个 DNS 名称或 IP 地址")
	}
	if len(csr.DNSNames) == 1 {
		name := csr.DNSNames[0]
		if len(name) > 253 || strings.TrimSpace(name) != name {
			return nil, errors.New("Agent CSR 的 DNS 名称无效")
		}
		for _, label := range strings.Split(name, ".") {
			if !dnsLabelPattern.MatchString(label) {
				return nil, errors.New("Agent CSR 的 DNS 名称无效")
			}
		}
	}
	if len(csr.IPAddresses) == 1 && net.ParseIP(csr.IPAddresses[0].String()) == nil {
		return nil, errors.New("Agent CSR 的 IP 地址无效")
	}
	switch key := csr.PublicKey.(type) {
	case *rsa.PublicKey:
		if key.N.BitLen() < 2048 {
			return nil, errors.New("Agent CSR 的 RSA 密钥至少需要 2048 位")
		}
	case *ecdsa.PublicKey:
		if key.Params().BitSize < 256 {
			return nil, errors.New("Agent CSR 的 ECDSA 密钥至少需要 256 位")
		}
	case ed25519.PublicKey:
	default:
		return nil, errors.New("Agent CSR 的公钥类型不受支持")
	}
	return csr, nil
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, limit)
}

func encodeCertificate(der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}
