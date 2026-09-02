package api

import (
	systemapi "github.com/cylism/cylism-manager/internal/api/system"
	"github.com/gin-gonic/gin"
)

func registerMonitoringRoutes(apiGroup *gin.RouterGroup, h *systemapi.MonitoringHandler) {
	g := apiGroup.Group("/monitoring")
	g.GET("/status", h.Status)
	g.POST("/install", h.Install)
	g.POST("/storage-migration", h.MigrateLegacyStorage)
	g.DELETE("", h.Uninstall)
	g.GET("/query", h.Query)
	g.GET("/query-range", h.QueryRange)
	g.GET("/dashboard", h.Dashboard)
	g.GET("/disk-growth", h.DiskGrowth)
	g.GET("/targets", h.Targets)
}

func registerAlertingRoutes(r *gin.Engine, apiGroup *gin.RouterGroup, h *systemapi.AlertingHandler) {
	g := apiGroup.Group("/monitoring/alerts")
	g.GET("/status", h.Status)
	g.POST("/install", h.Install)
	g.PUT("/config", h.Update)
	g.DELETE("", h.Uninstall)
	g.GET("/overview", h.Overview)
	g.GET("/silences", h.ListSilences)
	g.POST("/silences", h.CreateSilence)
	g.DELETE("/silences/:id", h.DeleteSilence)
	g.POST("/test-notification", h.TestNotification)
	g.GET("/automation-policy", h.AutomationPolicy)
	g.PUT("/automation-policy", h.UpdateAutomationPolicy)
	g.GET("/automation-events", h.ListAutomationEvents)
	// Alertmanager calls this endpoint with its per-install bearer token.
	r.POST("/api/monitoring/alerts/notify", h.Notify)
}

func registerLoggingRoutes(apiGroup *gin.RouterGroup, h *systemapi.LoggingHandler) {
	g := apiGroup.Group("/monitoring/logs")
	g.GET("/status", h.Status)
	g.POST("/install", h.Install)
	g.PUT("/config", h.Update)
	g.DELETE("", h.Uninstall)
	g.GET("/filters", h.Filters)
	g.POST("/query", h.Query)
}
