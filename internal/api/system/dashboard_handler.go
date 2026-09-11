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

	expiringCerts, _ := h.store.ListExpiringCerts(30)
	logs, _, _ := h.store.ListAuditLogs("", "", "", 10, 0)

	apiShared.Success(c, gin.H{
		"stats":          apiShared.DashboardStatsDTO(stats),
		"expiring_certs": apiShared.CertsDTO(expiringCerts),
		"recent_logs":    apiShared.AuditLogsDTO(logs),
	})
}
