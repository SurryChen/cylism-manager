package api

import (
	authapi "github.com/cylism/cylism-manager/internal/api/auth"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes binds all HTTP/WebSocket routes using the fully composed
// dependency graph. No services, adapters, or handlers are constructed here.
func RegisterRoutes(r *gin.Engine, deps RouteDependencies) {
	registerPublicRoutes(r, deps.Artifact, deps.Agent)
	registerAuthRoutes(r, deps.Auth, deps.AuthConfig)

	apiGroup := r.Group("/api")
	if deps.AuthConfig != nil {
		apiGroup.Use(authapi.JWTAuthMiddleware(deps.AuthConfig.JWTSecret))
	}
	if deps.Audit != nil {
		apiGroup.Use(deps.Audit)
	}
	registerRuntimeRoutes(apiGroup, deps.Runtime, deps.AgentOp, deps.Components, deps.Network)
	registerDashboardRoutes(apiGroup, deps.Dashboard)
	registerDeliveryRoutes(r, apiGroup, deliveryRouteHandlers{
		image: deps.Image, nodeMirrors: deps.NodeMirrors, managed: deps.Managed,
		proxy: deps.Proxy, chart: deps.Chart, platform: deps.Platform,
	})
	registerMonitoringRoutes(apiGroup, deps.Monitoring)
	registerLoggingRoutes(apiGroup, deps.Logging)
	registerAlertingRoutes(r, apiGroup, deps.Alerting)
	if deps.AuthConfig != nil {
		registerApplicationRoutes(r, apiGroup, deps.Application, deps.AuthConfig.JWTSecret, deps.Audit)
	} else {
		registerApplicationRoutes(r, apiGroup, deps.Application, nil, deps.Audit)
	}
	registerInfrastructureRoutes(apiGroup, deps.Server, deps.NetworkDiag, deps.Terminal, deps.Site,
		deps.Operation, deps.Domain, deps.Node, deps.NodeJoin, deps.Ingress, deps.Certificate,
		deps.K8s, deps.Storage, deps.Network, deps.Tailscale, deps.CRD, deps.AuditLog, deps.DBAdmin)
}
