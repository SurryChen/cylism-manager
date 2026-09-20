package bootstrap

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	coreauth "github.com/cylism/cylism-manager/internal/service/auth"
	"github.com/gin-gonic/gin"
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
	services := BuildServices(BuildRepositories(db), &k8s.Client{}, BuildKubernetesAdapters(&k8s.Client{}), []byte("01234567890123456789012345678901"))
	if services.Cluster == nil || services.Network == nil || services.Storage == nil {
		t.Fatal("expected core services to be composed")
	}
	if services.ApplicationQuery == nil || services.PlatformRelease == nil {
		t.Fatal("expected application and platform services to be composed")
	}
	if services.RegistryMirror == nil || services.RegistryManaged == nil || services.RegistryProxy == nil || services.RegistryProxyReconciler == nil {
		t.Fatal("expected registry services to be composed")
	}
	if services.Monitoring == nil || services.LoggingQuery == nil {
		t.Fatal("expected observability query services to be composed")
	}
	if services.AuthTemporaryTokens == nil || services.AlertingAutomation == nil {
		t.Fatal("expected auth and alerting workflow services to be composed")
	}
	if services.RuntimeManager == nil || services.RuntimeRegistry == nil || services.SystemComponentList == nil || services.SystemComponent == nil || services.AlertingQuery == nil {
		t.Fatal("expected runtime and system-component services to be composed")
	}
}

func TestBuildRouteDependenciesComposesAllHandlerGroups(t *testing.T) {
	container, err := NewContainer(Config{
		DBPath:          ":memory:",
		EncryptionKey:   []byte("01234567890123456789012345678901"),
		JWTSecret:       []byte("secret"),
		AccessTokenTTL:  time.Minute,
		RefreshTokenTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewContainer: %v", err)
	}

	deps := container.BuildRouteDependencies()
	if deps.Auth.Config != container.Auth || deps.Auth.Handler == nil || deps.Auth.Audit == nil {
		t.Fatal("expected auth dependencies composed by bootstrap")
	}
	if deps.Application.Handler == nil {
		t.Fatal("expected application handler composed by bootstrap")
	}
	if deps.RuntimeAgent.Artifact == nil || deps.RuntimeAgent.Agent == nil || deps.RuntimeAgent.Runtime == nil || deps.RuntimeAgent.AgentOp == nil || deps.RuntimeAgent.Components == nil || deps.RuntimeAgent.Network == nil {
		t.Fatal("expected runtime and agent handlers composed by bootstrap")
	}
	if deps.Delivery.Platform == nil || deps.Delivery.Image == nil || deps.Delivery.NodeMirrors == nil || deps.Delivery.Managed == nil || deps.Delivery.Proxy == nil || deps.Delivery.Chart == nil {
		t.Fatal("expected delivery handlers composed by bootstrap")
	}
	infra := deps.Infrastructure
	if infra.Network != deps.RuntimeAgent.Network || infra.Server == nil || infra.NetworkDiag == nil || infra.Terminal == nil || infra.Site == nil || infra.Operation == nil || infra.Domain == nil || infra.Node == nil || infra.NodeJoin == nil || infra.Ingress == nil || infra.Certificate == nil || infra.K8s == nil || infra.Platform == nil || infra.Storage == nil || infra.CRD == nil || infra.AuditLog == nil || infra.DBAdmin == nil {
		t.Fatal("expected infrastructure handlers composed by bootstrap")
	}
	if deps.System.Dashboard == nil || deps.System.Monitoring == nil || deps.System.Alerting == nil || deps.System.Logging == nil {
		t.Fatal("expected system handlers composed by bootstrap")
	}
}

func TestBuildRouteDependenciesUsesJWTSecretForIntegrationDelegation(t *testing.T) {
	const sessionToken = "integration-session-token"
	jwtSecret := []byte("jwt-secret")
	container, err := NewContainer(Config{
		DBPath:          ":memory:",
		EncryptionKey:   []byte("01234567890123456789012345678901"),
		JWTSecret:       jwtSecret,
		AccessTokenTTL:  time.Minute,
		RefreshTokenTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewContainer: %v", err)
	}
	hash := sha256.Sum256([]byte(sessionToken))
	hashValue := fmt.Sprintf("%x", hash[:])
	now := time.Now()
	if err := container.Store.CreateIntegrationSession(&model.IntegrationSession{
		HandoffCodeHash:  "handoff-hash",
		SessionTokenHash: &hashValue,
		UserID:           7,
		ProjectID:        9,
		ApplicationID:    11,
		EnvironmentID:    13,
		ActionsData:      "application:read",
		HandoffExpiresAt: now.Add(time.Minute),
		ExpiresAt:        now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("create integration session: %v", err)
	}

	deps := container.BuildRouteDependencies()
	r := gin.New()
	r.POST("/delegation", deps.Application.Handler.CreateIntegrationDelegation)
	req := httptest.NewRequest(http.MethodPost, "/delegation", strings.NewReader(`{"capability":"hysteria2"}`))
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("delegation status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var envelope struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode delegation response: %v", err)
	}
	if _, err := coreauth.ParseDelegationToken(jwtSecret, envelope.Data.Token); err != nil {
		t.Fatalf("delegation token was not signed with JWT secret: %v", err)
	}
}
