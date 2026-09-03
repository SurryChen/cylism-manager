package applicationapi

import (
	"errors"
	"strings"

	"context"
	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/application"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
)

type NamespaceClient interface {
	GetNamespace(context.Context, string) (*corev1.Namespace, error)
	UpdateNamespace(context.Context, *corev1.Namespace) (*corev1.Namespace, error)
	CreateNamespace(context.Context, *corev1.Namespace) (*corev1.Namespace, error)
}

type ApplicationHandler struct {
	resources        applicationResourceRepository
	applications     applicationManagementRepository
	sessions         repository.IntegrationSessionRepository
	queries          *applicationservice.QueryService
	encKey           []byte
	delegationSecret []byte
	kubernetes       KubernetesAdapter
	workflow         *applicationservice.ReleaseService
	workloads        *applicationservice.WorkloadService
}

// applicationManagementRepository is the mutation surface used by the
// application aggregate handlers. Keeping it local prevents handlers from
// depending on release-state, integration-session, or domain persistence.
type applicationManagementRepository interface {
	repository.ApplicationQueryRepository
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

type applicationResourceRepository interface {
	repository.ReleaseWorkflowRepository
	repository.ApplicationManagedFileRepository
	repository.ApplicationDomainReader
	GetApplicationEndpoint(uint, uint) (*model.ApplicationEndpoint, error)
	GetApplicationDeploymentTemplate(uint, uint) (*model.ApplicationDeploymentTemplate, error)
	UpdateApplicationDeploymentTemplateIfRevision(*model.ApplicationDeploymentTemplate, uint) error
}

// ApplicationHandlerDependencies is the complete persistence contract needed
// to compose the application HTTP aggregate. It replaces the broader shared
// repository aggregate at this package boundary.
type ApplicationHandlerDependencies interface {
	applicationResourceRepository
	applicationManagementRepository
	repository.IntegrationSessionRepository
}

func (h *ApplicationHandler) WithDelegationSecret(secret []byte) *ApplicationHandler {
	h.delegationSecret = append([]byte(nil), secret...)
	return h
}

type workloadKindRequest struct {
	WorkloadKind string `json:"workload_kind"`
}

type applicationCapabilitiesRequest struct {
	Capabilities []string `json:"capabilities"`
}

// applicationDiscoveryInfo is intentionally limited to metadata needed by
// authorized management UIs. It never embeds templates or Secret references.

// NewApplicationHandlerWithDependencies constructs a handler from Bootstrap
// dependencies. It never creates a query service when one was not provided.
func NewApplicationHandlerWithDependencies(resources ApplicationHandlerDependencies, queries *applicationservice.QueryService, encKey []byte, dependencies KubernetesAdapter) *ApplicationHandler {
	return newApplicationHandler(resources, queries, encKey, dependencies)
}

func newApplicationHandler(resources ApplicationHandlerDependencies, queries *applicationservice.QueryService, encKey []byte, dependencies KubernetesAdapter) *ApplicationHandler {
	var applier application.ResourceApplier
	if dependencies != nil {
		applier = dependencies
	}
	handler := &ApplicationHandler{
		resources: resources, applications: resources, sessions: resources,
		queries: queries, encKey: append([]byte(nil), encKey...), kubernetes: dependencies,
		workflow: applicationservice.NewReleaseService(resources, encKey, applier),
	}
	handler.workloads = applicationservice.NewWorkloadService(resources, dependencies)
	return handler
}

func (h *ApplicationHandler) releaseWorkflow() *applicationservice.ReleaseService {
	return h.workflow
}

func (h *ApplicationHandler) namespaces() NamespaceClient {
	return h.kubernetes
}

func (h *ApplicationHandler) ListApplications(c *gin.Context) {
	projectID, err := apiShared.OptionalID(strings.TrimSpace(c.Query("project_id")))
	if err != nil {
		apiShared.BadRequest(c, "项目 ID 无效")
		return
	}
	environmentID, err := apiShared.OptionalID(strings.TrimSpace(c.Query("environment_id")))
	if err != nil {
		apiShared.BadRequest(c, "环境 ID 无效")
		return
	}
	applications, err := h.queries.ListApplications(projectID, environmentID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	model.Success(c, applications)
}

// UpdateCapabilities replaces opaque application metadata. Capability values
// are not interpreted as authorization and do not affect a release.

func (h *ApplicationHandler) UpdateCapabilities(c *gin.Context) {
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	var req applicationCapabilitiesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "能力标签定义无效")
		return
	}
	app, err := h.applications.ReplaceApplicationCapabilities(applicationID, req.Capabilities)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	model.Success(c, app)
}

// DiscoverApplications provides a project-scoped, sanitized view for other
// control-plane UIs. It deliberately excludes deployment template data and
// all Secret references.

func (h *ApplicationHandler) GetApplication(c *gin.Context) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	app, err := h.queries.GetApplication(id)
	if err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	releases, _ := h.queries.ListReleases(id)
	model.Success(c, gin.H{"application": app, "releases": releases})
}

func (h *ApplicationHandler) UpdateWorkloadKind(c *gin.Context) {
	if h.kubernetes == nil || !h.kubernetes.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	var req workloadKindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "工作负载类型无效")
		return
	}
	req.WorkloadKind = strings.ToLower(strings.TrimSpace(req.WorkloadKind))
	if req.WorkloadKind != application.WorkloadKindDeployment && req.WorkloadKind != application.WorkloadKindStatefulSet {
		apiShared.ValidationError(c, "工作负载类型必须为 Deployment 或 StatefulSet")
		return
	}
	app, err := h.queries.GetApplication(applicationID)
	if err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	if app.WorkloadKind == req.WorkloadKind {
		model.Success(c, app)
		return
	}
	if err := h.workloads.UpdateKind(c.Request.Context(), app, req.WorkloadKind); err != nil {
		if errors.Is(err, applicationservice.ErrWorkloadSnapshot) {
			apiShared.DBError(c, "读取成功发布快照失败")
			return
		}
		if errors.Is(err, applicationservice.ErrWorkloadPersistence) {
			apiShared.DBError(c, err.Error())
			return
		}
		apiShared.ValidationError(c, err.Error())
		return
	}
	model.Success(c, app)
}

func (h *ApplicationHandler) CreateApplication(c *gin.Context) {
	var req struct {
		ProjectID     uint   `json:"project_id"`
		EnvironmentID uint   `json:"environment_id"`
		Name          string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ProjectID == 0 || req.EnvironmentID == 0 || strings.TrimSpace(req.Name) == "" {
		apiShared.BadRequest(c, "项目、环境和应用名称必填")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if err := application.ValidateApplicationName(req.Name); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	environment, err := h.queries.ResolveEnvironment(req.ProjectID, req.EnvironmentID)
	if err != nil {
		apiShared.ValidationError(c, "环境不属于所选项目")
		return
	}
	if err := h.applications.EnsureNamespaceAvailable(environment.Namespace, environment.ID); err != nil {
		apiShared.Conflict(c, "环境命名空间存在冲突，请先完成迁移")
		return
	}
	app := &model.Application{ProjectID: req.ProjectID, EnvironmentID: req.EnvironmentID, Name: req.Name, WorkloadKind: "deployment", CreatedBy: apiShared.UserID(c)}
	if err := h.applications.CreateApplication(app); err != nil {
		apiShared.Conflict(c, "该环境内应用名称已存在")
		return
	}
	model.Success(c, app)
}
