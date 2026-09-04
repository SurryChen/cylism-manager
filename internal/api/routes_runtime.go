package api

import (
	agentapi "github.com/cylism/cylism-manager/internal/api/agent"
	networkapi "github.com/cylism/cylism-manager/internal/api/infrastructure/network"
	runtimeapi "github.com/cylism/cylism-manager/internal/api/runtime"
	systemapi "github.com/cylism/cylism-manager/internal/api/system"
	"github.com/gin-gonic/gin"
)

func registerRuntimeRoutes(apiGroup *gin.RouterGroup, runtime *runtimeapi.RuntimeHandler, operations *agentapi.AgentOperationHandler, components *systemapi.SystemComponentHandler, network *networkapi.NetworkHandler) {
	runtimes := apiGroup.Group("/runtimes")
	runtimes.GET("/catalog", runtime.Catalog)
	runtimes.GET("", runtime.List)
	runtimes.POST("", runtime.Create)
	runtimes.GET("/:id", runtime.Get)
	runtimes.PUT("/:id", runtime.Update)
	runtimes.POST("/:id/deploy", runtime.Deploy)
	runtimes.POST("/:id/agent-tools/install", runtime.InstallAgentTools)
	runtimes.POST("/:id/agent-tools/update", runtime.UpdateAgentTools)
	runtimes.POST("/:id/agent-tools/uninstall", runtime.UninstallAgentTools)
	runtimes.GET("/:id/agent-capability-grants", operations.ListGrants)
	runtimes.PUT("/:id/agent-capability-grants", operations.ReplaceGrants)
	runtimes.GET("/:id/agent-operations", operations.ListOperations)
	runtimes.POST("/:id/health-check", runtime.Health)
	runtimes.POST("/:id/uninstall", runtime.Uninstall)
	runtimes.POST("/:id/chat", runtime.Chat)
	runtimes.GET("/:id/chat/sessions", runtime.ChatSessions)
	runtimes.GET("/:id/chat/sessions/:sid/messages", runtime.ChatSessionMessages)
	runtimes.PATCH("/:id/chat/sessions/:sid", runtime.RenameChatSession)
	runtimes.POST("/:id/chat/sessions/:sid/archive", runtime.ArchiveChatSession)
	runtimes.POST("/:id/chat/sessions/:sid/restore", runtime.RestoreChatSession)
	runtimes.GET("/:id/chat/sessions/:sid/export", runtime.ExportChatSession)
	runtimes.DELETE("/:id/chat/sessions/:sid", runtime.DeleteChatSession)
	apiGroup.POST("/agent-operations/:operationID/approve", operations.Approve)
	apiGroup.POST("/agent-operations/:operationID/reject", operations.Reject)

	systemComponents := apiGroup.Group("/system-components")
	systemComponents.GET("", components.List)
	systemComponents.PUT("/:chart", components.Update)
	systemComponents.POST("/:chart/revert", components.Revert)

	clusterDNS := apiGroup.Group("/cluster-dns")
	clusterDNS.GET("", network.DNSStatus)
	clusterDNS.POST("", network.DNSApply)
	clusterDNS.DELETE("", network.DNSReset)
	clusterDNS.POST("/history/:revision/rollback", network.DNSRollback)
}

func registerDashboardRoutes(apiGroup *gin.RouterGroup, h *systemapi.DashboardHandler) {
	apiGroup.GET("/dashboard", h.Get)
}
