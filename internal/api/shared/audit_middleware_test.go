package shared

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestBuildDetailRedactsNestedReleaseSecrets(t *testing.T) {
	detail := buildDetail("POST", "/api/applications/1/releases", []byte(`{"image":"nginx:1.27","secrets":{"DATABASE_PASSWORD":"do-not-log"},"token":"also-private"}`), nil)
	if strings.Contains(detail, "do-not-log") || strings.Contains(detail, "also-private") || !strings.Contains(detail, "[REDACTED]") {
		t.Fatalf("audit detail leaked sensitive data: %s", detail)
	}
}

func TestRedactConfigMapContent(t *testing.T) {
	value := map[string]interface{}{"content": "private-config", "expected_revision": float64(1)}
	encoded, _ := json.Marshal(redactAuditValue(value))
	if strings.Contains(string(encoded), "private-config") || !strings.Contains(string(encoded), "[REDACTED]") {
		t.Fatalf("ConfigMap content leaked in audit detail: %s", encoded)
	}
}

func TestAuditMiddlewareOnlyAddsRequestContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(AuditMiddleware(s))
	r.POST("/api/node-registry-mirrors/:id/verify", func(c *gin.Context) {
		if RequestID(c) == "" {
			t.Fatal("request id was not added")
		}
		Success(c, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/node-registry-mirrors/1/verify", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", w.Code)
	}
	logs, total, err := s.ListAuditLogs("", "", "", 20, 0)
	if err != nil || total != 0 || len(logs) != 0 {
		t.Fatalf("middleware must not infer an audit event: %#v total=%d err=%v", logs, total, err)
	}
}

func TestAuditMiddlewareRecordsOnlyCatalogedResourceChanges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(AuditMiddleware(s))
	r.POST("/api/applications/:id/releases", func(c *gin.Context) { Success(c, gin.H{"ok": true}) })
	r.POST("/api/unmapped-action", func(c *gin.Context) { Success(c, gin.H{"ok": true}) })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/applications/7/releases", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/unmapped-action", nil))
	logs, total, err := s.ListAuditLogsFiltered(model.AuditLogFilter{Limit: 20})
	if err != nil || total != 1 || len(logs) != 1 || logs[0].Action != "application.release" || logs[0].ResourceID != 7 || logs[0].Source != model.AuditSourceAPI {
		t.Fatalf("unexpected cataloged event: %#v total=%d err=%v", logs, total, err)
	}
}

func TestCatalogedAuditActionCoversResourceMutationDomains(t *testing.T) {
	tests := []struct {
		method, path, action, resource string
	}{
		{http.MethodPost, "/api/projects", "project.create", "project"},
		{http.MethodPost, "/api/applications/:id/releases/:releaseID/rollback", "application.rollback", "application"},
		{http.MethodPost, "/api/servers/:id/unbind", "server.operate", "server"},
		{http.MethodPost, "/api/certs/:namespace/:name/install", "certificate.install", "certificate"},
		{http.MethodPost, "/api/image-registries/:id/verify", "image_registry.verify", "image_registry"},
		{http.MethodPost, "/api/managed-oci-registries/:id/repair", "managed_registry.repair", "managed_registry"},
		{http.MethodPost, "/api/registry-proxies/:id/cleanup", "registry_proxy.cleanup", "registry_proxy"},
		{http.MethodPost, "/api/chart-repositories/:id/verify", "chart_repository.verify", "chart_repository"},
		{http.MethodPost, "/api/k8s/persistent-volume-claims/:name/backups", "storage.backup", "storage"},
		{http.MethodPatch, "/api/k8s/deployments/:namespace/:name/scale", "workload.scale", "workload"},
		{http.MethodPost, "/api/monitoring/alerts/install", "alerting.install", "alerting"},
		{http.MethodPut, "/api/platform/endpoint", "platform.update", "platform"},
		{http.MethodGet, "/api/k8s/secrets/:namespace/:name", "secret.read", "secret"},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			action, resource, ok := catalogedAuditAction(test.method, test.path)
			if !ok || action != test.action || resource != test.resource {
				t.Fatalf("%s %s = (%q, %q, %t), want (%q, %q, true)", test.method, test.path, action, resource, ok, test.action, test.resource)
			}
		})
	}
}
