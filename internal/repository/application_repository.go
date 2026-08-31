package repository

import (
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
)

// ApplicationReader loads a single application with the project and
// environment data required to render or release it.
type ApplicationReader interface {
	GetApplication(uint) (*model.Application, error)
}

// ApplicationQueryRepository is the read model used by application views.
// It deliberately excludes mutation and Kubernetes operations.
type ApplicationQueryRepository interface {
	ApplicationReader
	GetProject(uint) (*model.Project, error)
	GetEnvironment(uint, uint) (*model.Environment, error)
	ListApplications(...uint) ([]model.Application, error)
	ListReleases(uint) ([]model.Release, error)
	ListReleasesByApplications([]uint) (map[uint][]model.Release, error)
}

// ApplicationManagementRepository contains the application-owned persistence
// operations used by project, environment, template, and endpoint handlers.
// Cross-domain resources, such as image registries and managed domains, stay
// outside this interface so their ownership remains explicit.
type ApplicationManagementRepository interface {
	ApplicationQueryRepository

	ListProjects() ([]model.Project, error)
	CreateProject(*model.Project) error
	UpdateProject(*model.Project) error
	DeleteProject(uint) error
	CountProjectEnvironments(uint) (int64, error)
	CountProjectApplications(uint) (int64, error)

	ListEnvironments(uint) ([]model.Environment, error)
	CreateEnvironment(*model.Environment) error
	UpdateEnvironment(*model.Environment) error
	DeleteEnvironment(uint) error
	EnsureNamespaceAvailable(string, uint) error
	ListEnvironmentNamespaceConflicts() ([]model.NamespaceConflict, error)
	IsEnvironmentNamespaceConflicted(string) (bool, error)
	CountEnvironmentApplications(uint) (int64, error)

	CreateApplication(*model.Application) error
	UpdateApplication(*model.Application) error
	ReplaceApplicationCapabilities(uint, []string) (*model.Application, error)
	GetLatestSuccessfulRelease(uint) (*model.Release, error)

	ListApplicationDeploymentTemplates(uint) ([]model.ApplicationDeploymentTemplate, error)
	GetApplicationDeploymentTemplate(uint, uint) (*model.ApplicationDeploymentTemplate, error)
	GetDefaultApplicationDeploymentTemplate(uint) (*model.ApplicationDeploymentTemplate, error)
	CreateApplicationDeploymentTemplate(*model.ApplicationDeploymentTemplate, bool) error
	UpdateApplicationDeploymentTemplate(*model.ApplicationDeploymentTemplate) error
	UpdateApplicationDeploymentTemplateIfRevision(*model.ApplicationDeploymentTemplate, uint) error
	DeleteApplicationDeploymentTemplate(uint, uint) error
	SetDefaultApplicationDeploymentTemplate(uint, uint) error

	ListApplicationEndpoints(uint) ([]model.ApplicationEndpoint, error)
	GetApplicationEndpoint(uint, uint) (*model.ApplicationEndpoint, error)
	CreateApplicationEndpoint(*model.ApplicationEndpoint) error
	UpdateApplicationEndpoint(*model.ApplicationEndpoint) error
	DeleteApplicationEndpoint(uint, uint) error
	CountApplicationEndpointRoute(uint, string, uint) (int64, error)
}

// ReleaseRepository contains the durable state transitions used by the
// release state machine. The service owns the transition order; the
// repository only persists the resulting records.
type ReleaseRepository interface {
	ApplicationReader
	ListReleases(uint) ([]model.Release, error)
	CreateRelease(*model.Release) error
	GetRelease(uint) (*model.Release, error)
	UpdateRelease(*model.Release) error
	CreateReleaseOperation(*model.ReleaseOperation) error
	UpdateReleaseOperation(*model.ReleaseOperation) error
	CreateOperationLog(*model.OperationLog) error
	UpdateOperationLog(*model.OperationLog) error
}

// ReleaseWorkflowRepository extends the release state repository with the
// current editable application resources used to prepare a release.
type ReleaseWorkflowRepository interface {
	ReleaseRepository
	GetDefaultApplicationDeploymentTemplate(uint) (*model.ApplicationDeploymentTemplate, error)
	GetLatestSuccessfulRelease(uint) (*model.Release, error)
	UpsertApplicationManagedFile(*model.ApplicationManagedFile) error
	DisableApplicationManagedFilesNotIn(uint, map[string]struct{}) error
	GetImageRegistryForProject(uint, uint) (*model.ImageRegistry, error)
	ListNodeRegistryMirrors() ([]model.NodeRegistryMirror, error)
	ListApplicationEndpoints(uint) ([]model.ApplicationEndpoint, error)
	UpdateApplicationEndpoint(*model.ApplicationEndpoint) error
}

// ApplicationManagedFileRepository persists the ConfigMap/Secret bindings
// exposed by application integrations. It is intentionally separate from the
// release state machine: integration callers need to read and edit a binding
// without gaining release-transition methods.
type ApplicationManagedFileRepository interface {
	ListApplicationManagedFiles(uint) ([]model.ApplicationManagedFile, error)
	GetApplicationManagedFile(uint, uint) (*model.ApplicationManagedFile, error)
	AdvanceApplicationManagedFileVersion(uint, uint, uint) (uint, error)
}

// ApplicationDomainReader is the small cross-domain read dependency required
// when an application endpoint is bound to a managed domain or rendered in a
// workspace view.
type ApplicationDomainReader interface {
	GetManagedDomain(uint) (*model.ManagedDomain, error)
	ListManagedDomains(...uint) ([]model.ManagedDomain, error)
	CountApplicationEndpointsByDomain(uint) (int64, error)
}

// ApplicationHandlerRepository composes the exact persistence capabilities
// used by the application HTTP aggregate. It is a handler-local composition,
// not a new generic Store abstraction.
type ApplicationHandlerRepository interface {
	ApplicationManagementRepository
	ReleaseWorkflowRepository
	ApplicationManagedFileRepository
	ApplicationDomainReader
	IntegrationSessionRepository
}

// IntegrationSessionRepository persists short-lived opaque handoff sessions
// used by external application integrations.
type IntegrationSessionRepository interface {
	CreateIntegrationSession(*model.IntegrationSession) error
	ExchangeIntegrationSession(string, string, time.Time, time.Time) (*model.IntegrationSession, error)
	GetActiveIntegrationSession(string, time.Time) (*model.IntegrationSession, error)
}

// Store is the current SQLite implementation. These assertions make any
// persistence drift visible at compile time while consumers depend only on
// the narrow contracts above.
var (
	_ ApplicationQueryRepository       = (*store.Store)(nil)
	_ ApplicationManagementRepository  = (*store.Store)(nil)
	_ ReleaseRepository                = (*store.Store)(nil)
	_ ReleaseWorkflowRepository        = (*store.Store)(nil)
	_ ApplicationManagedFileRepository = (*store.Store)(nil)
	_ ApplicationDomainReader          = (*store.Store)(nil)
	_ ApplicationHandlerRepository     = (*store.Store)(nil)
	_ IntegrationSessionRepository     = (*store.Store)(nil)
)
