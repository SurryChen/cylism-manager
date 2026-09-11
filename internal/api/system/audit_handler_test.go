package system

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
