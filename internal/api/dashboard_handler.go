package api

import (
	"net/http"

	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

type DashboardHandler struct {
	store *store.Store
}

func NewDashboardHandler(s *store.Store) *DashboardHandler {
	return &DashboardHandler{store: s}
}

// Get 获取仪表盘概览 GET /api/dashboard
func (h *DashboardHandler) Get(c *gin.Context) {
	stats, err := h.store.GetDashboardStats(30)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	expiringCerts, _ := h.store.ListExpiringCerts(30)
	logs, _, _ := h.store.ListAuditLogs("", "", 10, 0)

	c.JSON(http.StatusOK, gin.H{
		"stats":         stats,
		"expiring_certs": expiringCerts,
		"recent_logs":   logs,
	})
}
