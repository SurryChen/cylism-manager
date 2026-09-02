package applicationapi

import (
	"encoding/json"
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
	resources        repository.ApplicationHandlerRepository
	applications     repository.ApplicationManagementRepository
	sessions         repository.IntegrationSessionRepository
	queries          *applicationservice.QueryService
	encKey           []byte
	delegationSecret []byte
	kubernetes       KubernetesDependencies
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

func NewApplicationHandler(resources repository.ApplicationHandlerRepository, encKey []byte, dependencies KubernetesDependencies) *ApplicationHandler {
	return &ApplicationHandler{resources: resources, applications: resources, sessions: resources, queries: applicationservice.NewQueryService(resources), encKey: append([]byte(nil), encKey...), kubernetes: dependencies}
}

func (h *ApplicationHandler) releaseWorkflow() *application.ReleaseWorkflow {
	var applier application.ResourceApplier
	if h.kubernetes != nil {
		applier = h.kubernetes
	}
	return application.NewReleaseWorkflow(h.resources, h.encKey, applier)
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
	release, err := h.applications.GetLatestSuccessfulRelease(app.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		app.WorkloadKind = req.WorkloadKind
		if err := h.applications.UpdateApplication(app); err != nil {
			apiShared.DBError(c, err.Error())
			return
		}
		model.Success(c, app)
		return
	}
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	var spec application.ReleaseSpec
	if err := json.Unmarshal([]byte(release.DesiredSpec), &spec); err != nil {
		apiShared.DBError(c, "读取成功发布快照失败")
		return
	}
	context := applicationContextFor(app)
	context.ReleaseSequence = release.Sequence
	if err := h.kubernetes.Preflight(c.Request.Context(), context, spec); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.kubernetes.MigrateWorkloadKind(c.Request.Context(), context, spec, req.WorkloadKind); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	app.WorkloadKind = req.WorkloadKind
	if err := h.applications.UpdateApplication(app); err != nil {
		apiShared.DBError(c, err.Error())
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
