package api

import (
	"net/http"

	agentapi "github.com/cylism/cylism-manager/internal/api/agent"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

func registerPublicRoutes(r *gin.Engine, artifact http.Handler, agent *agentapi.AgentHandler) {
	r.GET("/health", func(c *gin.Context) {
		model.Success(c, gin.H{"status": "ok"})
	})
	r.GET(agentapi.CLIArtifactPath, gin.WrapH(artifact))
	r.GET(agentapi.CLIArtifactManifestPath, gin.WrapH(artifact))
	registerAgentRoutes(r, agent)
}

func registerAgentRoutes(r *gin.Engine, h *agentapi.AgentHandler) {
	r.GET("/api/agent/v1/cluster/status", gin.WrapF(h.ClusterStatus))
	r.GET("/api/agent/v1/capabilities/status", gin.WrapF(h.CapabilityStatus))
	r.GET("/api/agent/v1/workloads/get", gin.WrapF(h.WorkloadGet))
	r.GET("/api/agent/v1/workloads/logs", gin.WrapF(h.WorkloadLogs))
	r.GET("/api/agent/v1/pods/get", gin.WrapF(h.PodGet))
	r.GET("/api/agent/v1/events/list", gin.WrapF(h.EventList))
	r.GET("/api/agent/v1/pvcs/get", gin.WrapF(h.PVCGet))
	r.GET("/api/agent/v1/nodes/get", gin.WrapF(h.NodeGet))
	r.GET("/api/agent/v1/registries/status", gin.WrapF(h.RegistryStatus))
	r.GET("/api/agent/v1/registries/proxy-diagnose", gin.WrapF(h.RegistryProxyDiagnose))
	r.GET("/api/agent/v1/images/diagnose", gin.WrapF(h.ImageDiagnose))
	r.GET("/api/agent/v1/dns/status", gin.WrapF(h.DNSStatus))
	r.GET("/api/agent/v1/dns/resolve", gin.WrapF(h.DNSResolve))
	r.GET("/api/agent/v1/registries/node-verify", gin.WrapF(h.RegistryNodeVerify))
	r.POST("/api/agent/v1/registries/node-pull-check", gin.WrapF(h.RegistryNodePullCheck))
	r.POST("/api/agent/v1/deployments/scale", gin.WrapF(h.DeploymentScale))
	r.GET("/api/agent/v1/approvals/:operationID", gin.WrapF(h.ApprovalGet))
	r.GET("/api/agent/v1/alerts/get", gin.WrapF(h.AlertGet))
	r.GET("/api/agent/v1/alerts/list", gin.WrapF(h.AlertList))
	r.GET("/api/agent/v1/monitoring/disk-growth", gin.WrapF(h.MonitoringDiskGrowth))
	r.GET("/api/agent/v1/maintenance/disk-inspect", gin.WrapF(h.MaintenanceDiskInspect))
	r.POST("/api/agent/v1/maintenance/cleanup-request", gin.WrapF(h.MaintenanceCleanupRequest))
}

func registerAuthRoutes(r *gin.Engine, h *AuthHandler, cfg *AuthConfig) {
	g := r.Group("/api/auth")
	g.POST("/login", h.Login)
	g.POST("/refresh", h.Refresh)
	g.GET("/me", JWTAuthMiddleware(cfg.JWTSecret), h.Me)
}
