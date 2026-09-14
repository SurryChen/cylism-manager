package system

import (
	apiShared "github.com/cylism/cylism-manager/internal/api/shared"

	"github.com/cylism/cylism-manager/internal/repository"
	"github.com/gin-gonic/gin"
)

type DashboardHandler struct {
	store repository.DashboardRepository
}

func NewDashboardHandler(s repository.DashboardRepository) *DashboardHandler {
	return &DashboardHandler{store: s}
}

// Get 获取仪表盘概览 GET /api/dashboard
func (h *DashboardHandler) Get(c *gin.Context) {
	stats, err := h.store.GetDashboardStats(30)
	if err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}

	applicationSummary, applicationErr := h.store.GetDashboardApplicationSummary()
	expiringCerts, certErr := h.store.ListExpiringCerts(30)
	logs, _, logErr := h.store.ListAuditLogs("", "", "", 10, 0)
	sectionErrors := gin.H{}
	if applicationErr != nil {
		sectionErrors["applications"] = applicationErr.Error()
	}
	if certErr != nil {
		sectionErrors["expiring_certs"] = certErr.Error()
	}
	if logErr != nil {
		sectionErrors["recent_logs"] = logErr.Error()
	}

	apiShared.Success(c, gin.H{
		"stats":               apiShared.DashboardStatsDTO(stats),
		"application_summary": apiShared.DashboardApplicationSummaryDTO(applicationSummary),
		"expiring_certs":      apiShared.CertsDTO(expiringCerts),
		"recent_logs":         apiShared.AuditLogsDTO(logs),
		"errors":              sectionErrors,
	})
}
