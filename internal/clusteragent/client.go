package clusteragent

import (
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	accesscert "github.com/cylism/cylism-manager/internal/service/accesscert"
	"k8s.io/client-go/rest"
)

// NewKubernetesClient creates a Kubernetes client whose API endpoint is the
// HTTPS reverse proxy exposed by cluster-agent. The agent validates the
// platform client certificate and then forwards the request to its local
// Kubernetes API, so the platform never needs direct network access to 6443.
func NewKubernetesClient(endpoint string, material accesscert.ClientTLSMaterial) (*k8s.Client, error) {
	endpoint, err := normalizeEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM([]byte(material.CACertificatePEM)) {
		return nil, fmt.Errorf("Agent CA 证书无效")
	}
	config := &rest.Config{
		Host:    endpoint,
		Timeout: 15 * time.Second,
		TLSClientConfig: rest.TLSClientConfig{
			CAData:   []byte(material.CACertificatePEM),
			CertData: []byte(material.CertificatePEM),
			KeyData:  []byte(material.PrivateKeyPEM),
		},
	}
	client, err := k8s.NewClientFromRestConfig(config)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func normalizeEndpoint(raw string) (string, error) {
	host := strings.TrimSpace(raw)
	if host == "" {
		return "", fmt.Errorf("未配置控制平面地址")
	}
	if !strings.Contains(host, "://") {
		host = "https://" + host
	}
	parsed, err := url.Parse(host)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return "", fmt.Errorf("控制平面地址无效")
	}
	if parsed.Port() == "" {
		parsed.Host = net.JoinHostPort(parsed.Hostname(), "9443")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}
