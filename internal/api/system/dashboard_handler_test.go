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

func TestDashboardHandlerReturnsOverviewAndRecentAuditLogs(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAuditLog(&model.AuditLog{Action: "deploy", ResourceType: "application", ResourceID: 1, Detail: "ok"}); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.GET("/api/dashboard", NewDashboardHandler(s).Get)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/dashboard", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"stats"`) || !strings.Contains(w.Body.String(), `"recent_logs"`) {
		t.Fatalf("unexpected dashboard response: %d %s", w.Code, w.Body.String())
	}
}
