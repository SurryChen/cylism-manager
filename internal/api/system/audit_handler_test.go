package system

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestAuditHandlerListsLogsWithFiltersAndPagination(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAuditLog(&model.AuditLog{Action: "deploy", ResourceType: "application", ResourceID: 2, Detail: "deploy"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAuditLog(&model.AuditLog{Action: "deploy", ResourceType: "application", ResourceID: 3, Detail: "other"}); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.GET("/api/audit-logs", NewAuditHandler(s).List)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/audit-logs?resource_type=application&action=deploy&keyword=deploy&limit=10&offset=0", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"total":1`) || !strings.Contains(w.Body.String(), `"resource_type":"application"`) || strings.Contains(w.Body.String(), `"resource_id":3`) {
		t.Fatalf("unexpected audit response: %d %s", w.Code, w.Body.String())
	}
}

func TestAuditHandlerDateRangeUsesInclusiveStartAndExclusiveEnd(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	for _, createdAt := range []time.Time{
		time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local),
		time.Date(2026, 9, 8, 0, 0, 0, 0, time.Local),
		time.Date(2026, 9, 9, 0, 0, 0, 0, time.Local),
	} {
		if err := s.CreateAuditLog(&model.AuditLog{Action: "x", ResourceType: "platform", CreatedAt: createdAt}); err != nil {
			t.Fatal(err)
		}
	}
	r := gin.New()
	r.GET("/api/audit-logs", NewAuditHandler(s).List)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/audit-logs?created_from=2026-09-01&created_to=2026-09-08", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"total":2`) {
		t.Fatalf("unexpected date-range response: %d %s", w.Code, w.Body.String())
	}
}

func TestAuditHandlerListsStructuredFiltersAndLegacyFallback(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	for _, log := range []*model.AuditLog{
		{Action: "registry.mirror.verify", ResourceType: "node_registry_mirror", ResourceID: 2, TargetName: "GHCR", ActorType: model.AuditActorUser, ActorName: "admin", Source: model.AuditSourceAPI, Outcome: model.AuditOutcomeSucceeded, Summary: "验证节点镜像源 GHCR"},
		{Action: "registry.mirror.verify", ResourceType: "node_registry_mirror", ResourceID: 3, TargetName: "Docker Hub", ActorType: model.AuditActorUser, ActorName: "admin", Source: model.AuditSourceAPI, Outcome: model.AuditOutcomeFailed, Summary: "验证节点镜像源 Docker Hub"},
	} {
		if err := s.CreateAuditLog(log); err != nil {
			t.Fatal(err)
		}
	}
	r := gin.New()
	r.GET("/api/audit-logs", NewAuditHandler(s).List)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/audit-logs?resource_type=node_registry_mirror&outcome=failed&source=api&actor_type=user&target_name=Docker&keyword=验证", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"total":1`) || !strings.Contains(w.Body.String(), `"target_name":"Docker Hub"`) || strings.Contains(w.Body.String(), `"target_name":"GHCR"`) {
		t.Fatalf("unexpected structured audit response: %d %s", w.Code, w.Body.String())
	}
}
