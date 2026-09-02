package api

import (
	agentapi "github.com/cylism/cylism-manager/internal/api/agent"
	applicationapi "github.com/cylism/cylism-manager/internal/api/application"
	authapi "github.com/cylism/cylism-manager/internal/api/auth"
	deliveryapi "github.com/cylism/cylism-manager/internal/api/delivery"
	infrastructureapi "github.com/cylism/cylism-manager/internal/api/infrastructure"
	runtimeapi "github.com/cylism/cylism-manager/internal/api/runtime"
	systemapi "github.com/cylism/cylism-manager/internal/api/system"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

// RouteDependencies is the fully composed HTTP dependency graph. The API
// package only binds these already-created handlers; construction belongs to
// internal/bootstrap.
type RouteDependencies struct {
	Store       *store.Store
	AuthConfig  *authapi.AuthConfig
	Audit       gin.HandlerFunc
	Artifact    *agentapi.AgentArtifactHandler
	Agent       *agentapi.AgentHandler
	Auth        *authapi.AuthHandler
	Runtime     *runtimeapi.RuntimeHandler
	AgentOp     *agentapi.AgentOperationHandler
	Components  *systemapi.SystemComponentHandler
	Network     *infrastructureapi.NetworkHandler
	Dashboard   *systemapi.DashboardHandler
	Platform    *deliveryapi.PlatformHandler
	Image       *deliveryapi.ImageRegistryHandler
	NodeMirrors *deliveryapi.NodeRegistryMirrorHandler
	Managed     *deliveryapi.ManagedOCIRegistryHandler
	Proxy       *deliveryapi.RegistryProxyHandler
	Chart       *deliveryapi.ChartRepositoryHandler
	Monitoring  *systemapi.MonitoringHandler
	Alerting    *systemapi.AlertingHandler
	Logging     *systemapi.LoggingHandler
	Application *applicationapi.ApplicationHandler
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
