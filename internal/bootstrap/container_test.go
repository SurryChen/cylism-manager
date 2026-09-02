package bootstrap

import (
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
)

func TestNewContainerInitializesStoreAndAuth(t *testing.T) {
	container, err := NewContainer(Config{
		DBPath:          ":memory:",
		EncryptionKey:   []byte("01234567890123456789012345678901"),
		JWTSecret:       []byte("secret"),
		AccessTokenTTL:  time.Minute,
		RefreshTokenTTL: time.Hour,
		PlatformURL:     "https://example.test",
	})
	if err != nil {
		t.Fatalf("NewContainer: %v", err)
	}
	if container.Store == nil || container.Auth == nil {
		t.Fatal("expected store and auth dependencies")
	}
	if container.Auth.PlatformURL != "https://example.test" || container.Auth.AccessTokenTTL != time.Minute {
		t.Fatalf("unexpected auth config: %+v", container.Auth)
	}
	if string(container.configEncryptionKey()) != "01234567890123456789012345678901" {
		t.Fatal("encryption key was not retained")
	}
}

func TestBuildKubernetesAdaptersUsesExplicitClient(t *testing.T) {
	if adapters := BuildKubernetesAdapters(nil); adapters.Nodes != nil {
		t.Fatal("nil Kubernetes client should produce no node adapter")
	}

	client := &k8s.Client{}
	adapters := BuildKubernetesAdapters(client)
	if adapters.Nodes != client {
		t.Fatalf("node adapter = %T, want the explicitly supplied client", adapters.Nodes)
	}
	if adapters.SystemComponent == nil {
		t.Fatal("expected system component adapter for an explicit client")
	}
	if adapters.Monitoring.Query == nil || adapters.Monitoring.Component == nil || adapters.Monitoring.Consumers == nil {
		t.Fatal("expected monitoring adapters for an explicit client")
	}
	if adapters.Logging.Query == nil || adapters.Logging.Component == nil || adapters.Logging.FilterReader == nil {
		t.Fatal("expected logging adapters for an explicit client")
	}
	if adapters.Alerting.Alertmanager == nil || adapters.Alerting.Component == nil || adapters.Alerting.Secrets == nil {
		t.Fatal("expected alerting adapters for an explicit client")
	}
	if adapters.Network.Ingress == nil || adapters.Network.StandardIngress == nil || adapters.Network.DNS == nil || adapters.Network.Certificate == nil {
		t.Fatal("expected network adapters for an explicit client")
	}
	if adapters.Storage == nil {
		t.Fatal("expected storage adapter for an explicit client")
	}
	if adapters.Registry.ManagedResources == nil || adapters.Registry.ManagedStatus == nil || adapters.Registry.ProxyResources == nil || adapters.Registry.ProxyDiagnostics == nil {
		t.Fatal("expected registry adapters for an explicit client")
	}
}

func TestBuildServicesComposesDomainServices(t *testing.T) {
	db, err := NewRepositories(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	services := BuildServices(db, &k8s.Client{}, BuildKubernetesAdapters(&k8s.Client{}), []byte("01234567890123456789012345678901"))
	if services.Cluster == nil || services.Network == nil || services.Storage == nil {
		t.Fatal("expected core services to be composed")
	}
	if services.ApplicationQuery == nil || services.PlatformRelease == nil {
		t.Fatal("expected application and platform services to be composed")
	}
	if services.RegistryMirror == nil || services.RegistryManaged == nil || services.RegistryProxy == nil {
		t.Fatal("expected registry services to be composed")
	}
	if services.Monitoring == nil || services.LoggingQuery == nil {
		t.Fatal("expected observability query services to be composed")
	}
}
