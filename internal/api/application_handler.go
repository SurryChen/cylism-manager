package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/cylism/cylism-manager/internal/application"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

type ApplicationHandler struct{ store *store.Store }

func NewApplicationHandler(store *store.Store) *ApplicationHandler {
	return &ApplicationHandler{store: store}
}

func (h *ApplicationHandler) ListProjects(c *gin.Context) {
	projects, err := h.store.ListProjects()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, projects)
}

func (h *ApplicationHandler) CreateProject(c *gin.Context) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "项目名称必填")
		return
	}
	project := &model.Project{Name: req.Name, Description: req.Description, OwnerID: getUserID(c)}
	if err := h.store.CreateProject(project); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "项目名称已存在")
		return
	}
	model.Success(c, project)
}

func (h *ApplicationHandler) ListEnvironments(c *gin.Context) {
	projectID, err := parseID(c.Param("projectID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "项目 ID 无效")
		return
	}
	environments, err := h.store.ListEnvironments(projectID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, environments)
}

func (h *ApplicationHandler) CreateEnvironment(c *gin.Context) {
	projectID, err := parseID(c.Param("projectID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "项目 ID 无效")
		return
	}
	var req struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || req.Namespace == "" {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "环境名称和命名空间必填")
		return
	}
	environment := &model.Environment{ProjectID: projectID, Name: req.Name, Namespace: req.Namespace}
	if err := h.store.CreateEnvironment(environment); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "环境名称已存在")
		return
	}
	model.Success(c, environment)
}

func (h *ApplicationHandler) ListApplications(c *gin.Context) {
	applications, err := h.store.ListApplications()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, applications)
}

func (h *ApplicationHandler) GetApplication(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	app, err := h.store.GetApplication(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "应用不存在")
		return
	}
	releases, _ := h.store.ListReleases(id)
	model.Success(c, gin.H{"application": app, "releases": releases})
}

func (h *ApplicationHandler) CreateApplication(c *gin.Context) {
	var req struct {
		ProjectID     uint   `json:"project_id"`
		EnvironmentID uint   `json:"environment_id"`
		Name          string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ProjectID == 0 || req.EnvironmentID == 0 || req.Name == "" {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "项目、环境和应用名称必填")
		return
	}
	app := &model.Application{ProjectID: req.ProjectID, EnvironmentID: req.EnvironmentID, Name: req.Name, WorkloadKind: "deployment", CreatedBy: getUserID(c)}
	if err := h.store.CreateApplication(app); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "该环境内应用名称已存在")
		return
	}
	model.Success(c, app)
}

func (h *ApplicationHandler) CreateRelease(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	applicationID, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	var spec application.ReleaseSpec
	if err := c.ShouldBindJSON(&spec); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "发布定义无效")
		return
	}
	service := application.NewService(h.store, application.NewKubernetesApplier(K8s))
	release, err := service.CreateRelease(c.Request.Context(), applicationID, getUserID(c), spec)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	app, _ := h.store.GetApplication(applicationID)
	h.executeAsync(service, release.ID, app, spec)
	model.SuccessWithMessage(c, release, "发布已创建")
}

func (h *ApplicationHandler) RetryRelease(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	applicationID, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	releaseID, err := parseID(c.Param("releaseID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "发布 ID 无效")
		return
	}
	original, err := h.store.GetRelease(releaseID)
	if err != nil || original.ApplicationID != applicationID {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "发布不存在")
		return
	}
	service := application.NewService(h.store, application.NewKubernetesApplier(K8s))
	release, spec, err := service.RetryRelease(releaseID, getUserID(c))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "无法重试该发布")
		return
	}
	app, _ := h.store.GetApplication(applicationID)
	h.executeAsync(service, release.ID, app, spec)
	model.SuccessWithMessage(c, release, "重试已创建")
}

func (h *ApplicationHandler) RollbackRelease(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	applicationID, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	releaseID, err := parseID(c.Param("releaseID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "发布 ID 无效")
		return
	}
	original, err := h.store.GetRelease(releaseID)
	if err != nil || original.ApplicationID != applicationID {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "发布不存在")
		return
	}
	service := application.NewService(h.store, application.NewKubernetesApplier(K8s))
	release, spec, err := service.RollbackRelease(releaseID, getUserID(c))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "无法回滚该发布")
		return
	}
	app, _ := h.store.GetApplication(applicationID)
	h.executeAsync(service, release.ID, app, spec)
	model.SuccessWithMessage(c, release, "回滚已创建")
}

func (h *ApplicationHandler) GetRelease(c *gin.Context) {
	id, err := parseID(c.Param("releaseID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "发布 ID 无效")
		return
	}
	release, err := h.store.GetRelease(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "发布不存在")
		return
	}
	model.Success(c, release)
}

func (h *ApplicationHandler) executeAsync(service *application.Service, releaseID uint, app *model.Application, spec application.ReleaseSpec) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		_ = service.ExecuteRelease(ctx, releaseID, app, spec)
	}()
}

func parseID(value string) (uint, error) {
	id, err := strconv.ParseUint(value, 10, 64)
	return uint(id), err
}
