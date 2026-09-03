package api

import (
	authapi "github.com/cylism/cylism-manager/internal/api/auth"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes binds all HTTP/WebSocket routes using the fully composed
// dependency graph. No services, adapters, or handlers are constructed here.
func RegisterRoutes(r *gin.Engine, deps RouteDependencies) {
	registerPublicRoutes(r, deps.RuntimeAgent.Artifact, deps.RuntimeAgent.Agent)
	registerAuthRoutes(r, deps.Auth.Handler, deps.Auth.Config)

	apiGroup := r.Group("/api")
	if deps.Auth.Config != nil {
		apiGroup.Use(authapi.JWTAuthMiddleware(deps.Auth.Config.JWTSecret))
	}
	if deps.Auth.Audit != nil {
		apiGroup.Use(deps.Auth.Audit)
	}
	registerRuntimeRoutes(apiGroup, deps.RuntimeAgent.Runtime, deps.RuntimeAgent.AgentOp, deps.RuntimeAgent.Components, deps.RuntimeAgent.Network)
	registerDashboardRoutes(apiGroup, deps.System.Dashboard)
	registerDeliveryRoutes(r, apiGroup, deliveryRouteHandlers{
		image: deps.Delivery.Image, nodeMirrors: deps.Delivery.NodeMirrors, managed: deps.Delivery.Managed,
		proxy: deps.Delivery.Proxy, chart: deps.Delivery.Chart, platform: deps.Delivery.Platform,
	})
	registerMonitoringRoutes(apiGroup, deps.System.Monitoring)
	registerLoggingRoutes(apiGroup, deps.System.Logging)
	registerAlertingRoutes(r, apiGroup, deps.System.Alerting)
	if deps.Auth.Config != nil {
		registerApplicationRoutes(r, apiGroup, deps.Application.Handler, deps.Auth.Config.JWTSecret, deps.Auth.Audit)
	} else {
		registerApplicationRoutes(r, apiGroup, deps.Application.Handler, nil, deps.Auth.Audit)
	}
	infra := deps.Infrastructure
	registerInfrastructureRoutes(apiGroup, infra.Server, infra.NetworkDiag, infra.Terminal, infra.Site,
		infra.Operation, infra.Domain, infra.Node, infra.NodeJoin, infra.Ingress, infra.Certificate,
		infra.K8s, infra.Storage, infra.Network, infra.Tailscale, infra.CRD, infra.AuditLog, infra.DBAdmin)
}
