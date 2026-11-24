package clusteragent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"sigs.k8s.io/yaml"
)

type Config struct {
	ListenAddress string
	KubeAPI       string
	KubeConfig    string
	ServerCert    string
	ServerKey     string
	ClientCA      string
	ClusterID     uint
}

func NewHandler(cfg Config) (http.Handler, error) {
	target, err := url.Parse(cfg.KubeAPI)
	if err != nil || target.Scheme == "" || target.Host == "" {
		return nil, fmt.Errorf("invalid kube api URL")
	}
	if strings.TrimSpace(cfg.ClientCA) == "" {
		return nil, fmt.Errorf("client CA is required")
	}
	caPEM, err := os.ReadFile(cfg.ClientCA)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("invalid client CA")
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport, err = kubeTransport(cfg)
	if err != nil {
		return nil, err
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		http.Error(w, "kubernetes api unavailable", http.StatusBadGateway)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ready\n")
	})
	mux.Handle("/", verifyClient(proxy, roots, cfg.ClusterID))
	return mux, nil
}

type kubeConfigFile struct {
	CurrentContext string `json:"current-context"`
	Clusters       []struct {
		Name    string `json:"name"`
		Cluster struct {
			Server                   string `json:"server"`
			CertificateAuthorityData string `json:"certificate-authority-data"`
		} `json:"cluster"`
	} `json:"clusters"`
	Contexts []struct {
		Name    string `json:"name"`
		Context struct {
			Cluster string `json:"cluster"`
			User    string `json:"user"`
		} `json:"context"`
	} `json:"contexts"`
	Users []struct {
		Name string `json:"name"`
		User struct {
			ClientCertificateData string `json:"client-certificate-data"`
			ClientKeyData         string `json:"client-key-data"`
		} `json:"user"`
	} `json:"users"`
}

func kubeTransport(cfg Config) (http.RoundTripper, error) {
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	if strings.TrimSpace(cfg.KubeConfig) == "" {
		return transport, nil
	}
	raw, err := os.ReadFile(cfg.KubeConfig)
	if err != nil {
		return transport, nil
	} // local API may use a system trust store
	var file kubeConfigFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("invalid kubeconfig: %w", err)
	}
	var clusterName, userName string
	for _, item := range file.Contexts {
		if item.Name == file.CurrentContext || (file.CurrentContext == "" && clusterName == "") {
			clusterName, userName = item.Context.Cluster, item.Context.User
		}
	}
	for _, item := range file.Clusters {
		if item.Name != clusterName && clusterName != "" {
			continue
		}
		if item.Cluster.CertificateAuthorityData != "" {
			ca, err := base64.StdEncoding.DecodeString(item.Cluster.CertificateAuthorityData)
			if err != nil {
				return nil, err
			}
			roots := x509.NewCertPool()
			if !roots.AppendCertsFromPEM(ca) {
				return nil, fmt.Errorf("invalid kubeconfig CA")
			}
			transport.TLSClientConfig.RootCAs = roots
		}
		break
	}
	for _, item := range file.Users {
		if item.Name != userName && userName != "" {
			continue
		}
		if item.User.ClientCertificateData == "" || item.User.ClientKeyData == "" {
			break
		}
		certPEM, err := base64.StdEncoding.DecodeString(item.User.ClientCertificateData)
		if err != nil {
			return nil, err
		}
		keyPEM, err := base64.StdEncoding.DecodeString(item.User.ClientKeyData)
		if err != nil {
			return nil, err
		}
		pair, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig.Certificates = []tls.Certificate{pair}
		break
	}
	return transport, nil
}

func Serve(ctx context.Context, cfg Config) error {
	handler, err := NewHandler(cfg)
	if err != nil {
		return err
	}
	cert, err := tls.LoadX509KeyPair(cfg.ServerCert, cfg.ServerKey)
	if err != nil {
		return err
	}
	caPEM, err := os.ReadFile(cfg.ClientCA)
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return fmt.Errorf("invalid client CA")
	}
	server := &http.Server{Addr: cfg.ListenAddress, Handler: handler, ReadHeaderTimeout: 10 * time.Second, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}, ClientCAs: pool, ClientAuth: tls.VerifyClientCertIfGiven}}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	err = server.ListenAndServeTLS("", "")
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func verifyClient(next http.Handler, roots *x509.CertPool, clusterID uint) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "client certificate required", http.StatusUnauthorized)
			return
		}
		cert := r.TLS.PeerCertificates[0]
		opts := x509.VerifyOptions{Roots: roots, Intermediates: x509.NewCertPool(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
		for _, c := range r.TLS.PeerCertificates[1:] {
			opts.Intermediates.AddCert(c)
		}
		if _, err := cert.Verify(opts); err != nil {
			http.Error(w, "client certificate rejected", http.StatusForbidden)
			return
		}
		expected := fmt.Sprintf("spiffe://cylism.local/cluster/%d/platform_client", clusterID)
		valid := false
		for _, uri := range cert.URIs {
			if uri.String() == expected {
				valid = true
				break
			}
		}
		if !valid {
			http.Error(w, "client identity rejected", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ParseCertificate is a small helper used by tests and diagnostics.
func ParseCertificate(raw []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("certificate PEM is invalid")
	}
	return x509.ParseCertificate(block.Bytes)
}
