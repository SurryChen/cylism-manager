package api

import (
	"context"
	"strconv"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	systemapi "github.com/cylism/cylism-manager/internal/api/system"
	"github.com/gin-gonic/gin"
)

type clusterObservabilityFactory func(context.Context, uint) (*systemapi.MonitoringHandler, *systemapi.LoggingHandler, *systemapi.AlertingHandler, error)

func registerClusterObservabilityRoutes(apiGroup *gin.RouterGroup, factory clusterObservabilityFactory) {
	g := apiGroup.Group("/clusters/:id/monitoring")
	g.Use(func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil || id == 0 {
			apiShared.BadRequest(c, "集群 ID 无效")
			c.Abort()
			return
		}
		monitoring, logging, alerting, err := factory(c.Request.Context(), uint(id))
		if err != nil {
			apiShared.BadRequest(c, err.Error())
			c.Abort()
			return
		}
		c.Set("clusterMonitoring", monitoring)
		c.Set("clusterLogging", logging)
		c.Set("clusterAlerting", alerting)
	})
	monitoring := func(fn func(*systemapi.MonitoringHandler, *gin.Context)) gin.HandlerFunc {
		return func(c *gin.Context) { fn(c.MustGet("clusterMonitoring").(*systemapi.MonitoringHandler), c) }
	}
	logging := func(fn func(*systemapi.LoggingHandler, *gin.Context)) gin.HandlerFunc {
		return func(c *gin.Context) { fn(c.MustGet("clusterLogging").(*systemapi.LoggingHandler), c) }
	}
	alerting := func(fn func(*systemapi.AlertingHandler, *gin.Context)) gin.HandlerFunc {
		return func(c *gin.Context) { fn(c.MustGet("clusterAlerting").(*systemapi.AlertingHandler), c) }
	}
	g.GET("/status", monitoring((*systemapi.MonitoringHandler).Status))
	g.POST("/install", monitoring((*systemapi.MonitoringHandler).Install))
	g.POST("/storage-migration", monitoring((*systemapi.MonitoringHandler).MigrateLegacyStorage))
	g.DELETE("", monitoring((*systemapi.MonitoringHandler).Uninstall))
	g.GET("/query", monitoring((*systemapi.MonitoringHandler).Query))
	g.GET("/query-range", monitoring((*systemapi.MonitoringHandler).QueryRange))
	g.GET("/dashboard", monitoring((*systemapi.MonitoringHandler).Dashboard))
	g.GET("/disk-growth", monitoring((*systemapi.MonitoringHandler).DiskGrowth))
	g.GET("/targets", monitoring((*systemapi.MonitoringHandler).Targets))
	g.GET("/logs/status", logging((*systemapi.LoggingHandler).Status))
	g.POST("/logs/install", logging((*systemapi.LoggingHandler).Install))
	g.PUT("/logs/config", logging((*systemapi.LoggingHandler).Update))
	g.DELETE("/logs", logging((*systemapi.LoggingHandler).Uninstall))
	g.GET("/logs/filters", logging((*systemapi.LoggingHandler).Filters))
	g.POST("/logs/query", logging((*systemapi.LoggingHandler).Query))
	g.GET("/alerts/status", alerting((*systemapi.AlertingHandler).Status))
	g.POST("/alerts/install", alerting((*systemapi.AlertingHandler).Install))
	g.PUT("/alerts/config", alerting((*systemapi.AlertingHandler).Update))
	g.DELETE("/alerts", alerting((*systemapi.AlertingHandler).Uninstall))
	g.GET("/alerts/overview", alerting((*systemapi.AlertingHandler).Overview))
	g.GET("/alerts/silences", alerting((*systemapi.AlertingHandler).ListSilences))
	g.POST("/alerts/silences", alerting((*systemapi.AlertingHandler).CreateSilence))
	g.DELETE("/alerts/silences/:silenceID", alerting((*systemapi.AlertingHandler).DeleteSilence))
	g.POST("/alerts/test-notification", alerting((*systemapi.AlertingHandler).TestNotification))
}
