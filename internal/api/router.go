package api

import (
	agentapi "github.com/cylism/cylism-manager/internal/api/agent"
	applicationapi "github.com/cylism/cylism-manager/internal/api/application"
	authapi "github.com/cylism/cylism-manager/internal/api/auth"
	deliveryapi "github.com/cylism/cylism-manager/internal/api/delivery"
	infrastructureapi "github.com/cylism/cylism-manager/internal/api/infrastructure"
	runtimeapi "github.com/cylism/cylism-manager/internal/api/runtime"
	systemapi "github.com/cylism/cylism-manager/internal/api/system"
	"github.com/gin-gonic/gin"
)

// RouteDependencies is the fully composed HTTP dependency graph. The API
// package only binds these already-created handlers; construction belongs to
// internal/bootstrap.
type RouteDependencies struct {
	Auth           AuthDependencies
	Application    ApplicationDependencies
	RuntimeAgent   RuntimeAgentDependencies
	Delivery       DeliveryDependencies
	Infrastructure InfrastructureDependencies
	System         SystemDependencies
}

type AuthDependencies struct {
	Config  *authapi.AuthConfig
	Audit   gin.HandlerFunc
	Handler *authapi.AuthHandler
}

type ApplicationDependencies struct {
	Handler *applicationapi.ApplicationHandler
}

type RuntimeAgentDependencies struct {
	Artifact   *agentapi.AgentArtifactHandler
	Agent      *agentapi.AgentHandler
	Runtime    *runtimeapi.RuntimeHandler
	AgentOp    *agentapi.AgentOperationHandler
	Components *systemapi.SystemComponentHandler
	Network    *infrastructureapi.NetworkHandler
}

type DeliveryDependencies struct {
	Platform    *deliveryapi.PlatformHandler
	Image       *deliveryapi.ImageRegistryHandler
	NodeMirrors *deliveryapi.NodeRegistryMirrorHandler
	Managed     *deliveryapi.ManagedOCIRegistryHandler
	Proxy       *deliveryapi.RegistryProxyHandler
	Chart       *deliveryapi.ChartRepositoryHandler
}

type InfrastructureDependencies struct {
	Network     *infrastructureapi.NetworkHandler
	Server      *infrastructureapi.ServerHandler
	NetworkDiag *infrastructureapi.ServerNetworkDiagnosticsHandler
	Terminal    *infrastructureapi.ServerTerminalHandler
	Site        *infrastructureapi.SiteHandler
	Operation   *systemapi.OperationHandler
	Domain      *infrastructureapi.DomainHandler
	Node        *infrastructureapi.NodeHandler
	NodeJoin    *infrastructureapi.NodeJoinProgressHandler
	Ingress     *infrastructureapi.IngressHandler
	Certificate *infrastructureapi.CertHandler
	K8s         *infrastructureapi.K8sHandler
	Storage     *infrastructureapi.StorageHandler
	Tailscale   *systemapi.TailscaleHandler
	CRD         *infrastructureapi.CRDHandler
	AuditLog    *systemapi.AuditHandler
	DBAdmin     *systemapi.DBAdminHandler
}

type SystemDependencies struct {
	Dashboard  *systemapi.DashboardHandler
	Monitoring *systemapi.MonitoringHandler
	Alerting   *systemapi.AlertingHandler
	Logging    *systemapi.LoggingHandler
}

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
