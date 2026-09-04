package applicationapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type releaseRequest struct {
	TemplateID uint   `json:"template_id"`
	Version    string `json:"version"`
}

func (h *ApplicationHandler) CreateRelease(c *gin.Context) {
	if h.kubernetes == nil || !h.kubernetes.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	var req releaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "发布定义无效")
		return
	}
	version := strings.TrimSpace(req.Version)
	if req.TemplateID == 0 || version == "" {
		apiShared.ValidationError(c, "请选择上线模板并填写版本号")
		return
	}
	app, err := h.queries.GetApplication(applicationID)
	if err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	template, err := h.resources.GetApplicationDeploymentTemplate(applicationID, req.TemplateID)
	if errors.Is(err, gorm.ErrRecordNotFound) || !template.Enabled {
		apiShared.ValidationError(c, "上线模板不存在或已停用")
		return
	}
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	workflow := h.releaseWorkflow()
	prepared, err := workflow.CreateFromTemplate(c.Request.Context(), app, template, version, apiShared.UserID(c))
	if err != nil {
		if errors.Is(err, applicationservice.ErrReleaseTemplateRead) {
			apiShared.DBError(c, "读取应用上线模板失败")
			return
		}
		if errors.Is(err, applicationservice.ErrReleaseSecretRead) {
			apiShared.DBError(c, "读取模板 Secret 失败")
			return
		}
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := workflow.SyncManagedFiles(app, prepared.Spec, apiShared.UserID(c)); err != nil {
		apiShared.DBError(c, "登记受管文件失败")
		return
	}
	workflow.ExecuteAsync(c.Request.Context(), app, prepared)
	apiShared.SuccessWithMessage(c, apiShared.ReleaseDTO(prepared.Release), "发布已创建")
}

// RestartApplication recreates the latest successful release with the current
// managed resources, without requiring the caller to choose a template/version.

func (h *ApplicationHandler) RestartApplication(c *gin.Context) {
	if h.kubernetes == nil || !h.kubernetes.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	app, err := h.queries.GetApplication(applicationID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	workflow := h.releaseWorkflow()
	prepared, err := workflow.Restart(c.Request.Context(), app, apiShared.UserID(c))
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	workflow.ExecuteAsync(c.Request.Context(), app, prepared)
	apiShared.SuccessWithMessage(c, apiShared.ReleaseDTO(prepared.Release), "应用重启已创建")
}

func (h *ApplicationHandler) RetryRelease(c *gin.Context) {
	if h.kubernetes == nil || !h.kubernetes.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	releaseID, err := apiShared.ParseID(c.Param("releaseID"))
	if err != nil {
		apiShared.BadRequest(c, "发布 ID 无效")
		return
	}
	original, err := h.resources.GetRelease(releaseID)
	if err != nil || original.ApplicationID != applicationID {
		apiShared.NotFound(c, "发布不存在")
		return
	}
	workflow := h.releaseWorkflow()
	prepared, err := workflow.Retry(c.Request.Context(), releaseID, apiShared.UserID(c))
	if err != nil {
		apiShared.BadRequest(c, "无法重试该发布")
		return
	}
	app, err := h.queries.GetApplication(applicationID)
	if err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	workflow.ExecuteAsync(c.Request.Context(), app, prepared)
	apiShared.SuccessWithMessage(c, apiShared.ReleaseDTO(prepared.Release), "重试已创建")
}

func (h *ApplicationHandler) RollbackRelease(c *gin.Context) {
	if h.kubernetes == nil || !h.kubernetes.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	releaseID, err := apiShared.ParseID(c.Param("releaseID"))
	if err != nil {
		apiShared.BadRequest(c, "发布 ID 无效")
		return
	}
	original, err := h.resources.GetRelease(releaseID)
	if err != nil || original.ApplicationID != applicationID {
		apiShared.NotFound(c, "发布不存在")
		return
	}
	workflow := h.releaseWorkflow()
	prepared, err := workflow.Rollback(c.Request.Context(), releaseID, apiShared.UserID(c))
	if err != nil {
		apiShared.BadRequest(c, "无法回滚该发布")
		return
	}
	app, err := h.queries.GetApplication(applicationID)
	if err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	workflow.ExecuteAsync(c.Request.Context(), app, prepared)
	apiShared.SuccessWithMessage(c, apiShared.ReleaseDTO(prepared.Release), "回滚已创建")
}

func (h *ApplicationHandler) GetRelease(c *gin.Context) {
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	id, err := apiShared.ParseID(c.Param("releaseID"))
	if err != nil {
		apiShared.BadRequest(c, "发布 ID 无效")
		return
	}
	release, err := h.resources.GetRelease(id)
	if err != nil {
		apiShared.NotFound(c, "发布不存在")
		return
	}
	if release.ApplicationID != applicationID {
		apiShared.NotFound(c, "发布不存在")
		return
	}
	if !release.PodTrackingEnabled {
		release.Runtime = &model.ReleaseRuntime{Tracking: "legacy_untracked", Pods: []model.ReleasePodRuntime{}, Diagnostic: "该历史发布未记录 Pod 关联标签，无法精确查询当前运行态"}
		apiShared.Success(c, apiShared.ReleaseDTO(release))
		return
	}
	if h.kubernetes == nil || !h.kubernetes.KubernetesAvailable() {
		release.Runtime = &model.ReleaseRuntime{Tracking: "unavailable", Pods: []model.ReleasePodRuntime{}, Diagnostic: "Kubernetes 集群未连接，无法读取 Pod 运行态"}
		apiShared.Success(c, apiShared.ReleaseDTO(release))
		return
	}
	applicationModel, err := h.queries.GetApplication(applicationID)
	if err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	runtime, err := h.kubernetes.InspectReleasePods(c.Request.Context(), applicationservice.ApplicationContext{Namespace: applicationModel.Environment.Namespace, ApplicationName: applicationModel.Name, ReleaseSequence: release.Sequence})
	if err != nil {
		release.Runtime = &model.ReleaseRuntime{Tracking: "unavailable", Pods: []model.ReleasePodRuntime{}, Diagnostic: "读取 Pod 运行态失败: " + err.Error()}
	} else {
		release.Runtime = runtime
	}
	apiShared.Success(c, apiShared.ReleaseDTO(release))
}

func applicationContextFor(app *model.Application) applicationservice.ApplicationContext {
	return applicationservice.ApplicationContext{ProjectID: app.ProjectID, EnvironmentID: app.EnvironmentID, ProjectName: app.Project.Name, EnvironmentName: app.Environment.Name, ApplicationName: app.Name, Namespace: app.Environment.Namespace, WorkloadKind: app.WorkloadKind}
}

func validateImageRepository(image string) error {
	image = strings.Trim(strings.TrimSpace(image), "/")
	if image == "" || strings.ContainsAny(image, " \t\r\n") || strings.Contains(image, "@") {
		return fmt.Errorf("镜像路径无效")
	}
	lastSegment := image[strings.LastIndex(image, "/")+1:]
	if strings.Contains(lastSegment, ":") {
		return fmt.Errorf("上线模板镜像路径不应包含 Tag，请在发布时填写版本号")
	}
	return nil
}

func (h *ApplicationHandler) syncApplicationEndpoints(ctx context.Context, app *model.Application, service applicationservice.ServiceSpec) error {
	return h.releaseWorkflow().SyncApplicationEndpoints(ctx, app, service)
}
