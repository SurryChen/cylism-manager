package system

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

type dashboardRepositoryFake struct {
	applicationErr error
	certErr        error
	logErr         error
}

func (f dashboardRepositoryFake) GetDashboardApplicationSummary() (*model.DashboardApplicationSummary, error) {
	return nil, f.applicationErr
}

func (f dashboardRepositoryFake) GetDashboardStats(int) (*model.DashboardStats, error) {
	return &model.DashboardStats{}, nil
}
func (f dashboardRepositoryFake) ListExpiringCerts(int) ([]model.Cert, error) { return nil, f.certErr }
func (f dashboardRepositoryFake) ListAuditLogs(string, string, string, int, int) ([]model.AuditLog, int64, error) {
	return nil, 0, f.logErr
}

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

func TestDashboardHandlerExposesSectionErrors(t *testing.T) {
	r := gin.New()
	r.GET("/api/dashboard", NewDashboardHandler(dashboardRepositoryFake{certErr: errors.New("cert store down"), logErr: errors.New("audit store down")}).Get)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/dashboard", nil))
	var response struct {
		Data struct {
			Errors map[string]string `json:"errors"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.Errors["expiring_certs"] == "" || response.Data.Errors["recent_logs"] == "" {
		t.Fatalf("expected section errors, got %#v", response.Data.Errors)
	}
}

func TestDashboardHandlerExposesApplicationSummaryError(t *testing.T) {
	r := gin.New()
	r.GET("/api/dashboard", NewDashboardHandler(dashboardRepositoryFake{applicationErr: errors.New("application store down")}).Get)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/dashboard", nil))
	var response struct {
		Data struct {
			Errors map[string]string `json:"errors"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.Errors["applications"] == "" {
		t.Fatalf("expected application summary error, got %#v", response.Data.Errors)
	}
}
