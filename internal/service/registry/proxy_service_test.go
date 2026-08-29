package registry

import (
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/repository"
	"github.com/cylism/cylism-manager/internal/store"
)

func TestProxyServicePrepareDeploymentEncryptsCredentialsAndAllocatesResourceName(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	service := NewProxyService(repository.NewRegistryProxyRepository(s), []byte("01234567890123456789012345678901"))
	proxy, err := service.PrepareDeployment(ProxyInput{
		Name: "Docker Hub", Registry: "docker.io", NodeName: "node-a", EndpointHost: "100.64.0.8",
		NodePort: 30500, CacheLimitGi: 2, CleanupIntervalHours: 24,
		HTTPProxy: "http://user:secret@proxy.internal:3128", DNSServers: []string{"10.0.0.2"},
	}, nil, 7)
	if err != nil {
		t.Fatal(err)
	}
	if proxy.ResourceName == "" || !proxy.OutboundProxyConfigured || strings.Contains(proxy.EncryptedHTTPProxy, "secret") {
		t.Fatalf("expected encrypted proxy with resource name, got %#v", proxy)
	}
	stored, err := s.GetRegistryProxyByID(proxy.ID)
	if err != nil || stored.EncryptedHTTPProxy == "" || stored.DNSResolvers != `["10.0.0.2"]` {
		t.Fatalf("unexpected stored configuration: %#v, %v", stored, err)
	}
}

func TestProxyServiceRejectsDuplicateNodePort(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	service := NewProxyService(repository.NewRegistryProxyRepository(s), nil)
	first := ProxyInput{Name: "Docker Hub", Registry: "docker.io", NodeName: "node-a", EndpointHost: "10.0.0.8", NodePort: 30500, CacheLimitGi: 2, CleanupIntervalHours: 24}
	if _, err := service.PrepareDeployment(first, nil, 1); err != nil {
		t.Fatal(err)
	}
	_, err = service.PrepareDeployment(ProxyInput{Name: "Kubernetes", Registry: "registry.k8s.io", NodeName: "node-a", EndpointHost: "10.0.0.8", NodePort: 30500, CacheLimitGi: 2, CleanupIntervalHours: 24}, nil, 1)
	if err == nil || !strings.Contains(err.Error(), "NodePort 30500") {
		t.Fatalf("expected duplicate NodePort error, got %v", err)
	}
}
