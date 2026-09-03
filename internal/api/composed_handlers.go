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
