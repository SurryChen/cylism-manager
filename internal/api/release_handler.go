package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/application"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type releaseRequest struct {
	TemplateID uint   `json:"template_id"`
	Version    string `json:"version"`
}

func (h *ApplicationHandler) CreateRelease(c *gin.Context) {
	if K8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	var req releaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "发布定义无效")
		return
	}
	version := strings.TrimSpace(req.Version)
	if req.TemplateID == 0 || version == "" {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "请选择上线模板并填写版本号")
		return
	}
	app, err := h.queries.GetApplication(applicationID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "应用不存在")
		return
	}
	template, err := h.store.GetApplicationDeploymentTemplate(applicationID, req.TemplateID)
	if errors.Is(err, gorm.ErrRecordNotFound) || !template.Enabled {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "上线模板不存在或已停用")
		return
	}
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	workflow := h.releaseWorkflow()
	prepared, err := workflow.CreateFromTemplate(c.Request.Context(), app, template, version, apiShared.UserID(c))
	if err != nil {
		if errors.Is(err, application.ErrReleaseTemplateRead) {
			model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取应用上线模板失败")
			return
		}
		if errors.Is(err, application.ErrReleaseSecretRead) {
			model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取模板 Secret 失败")
			return
		}
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := workflow.SyncManagedFiles(app, prepared.Spec, apiShared.UserID(c)); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "登记受管文件失败")
		return
	}
	workflow.ExecuteAsync(app, prepared)
	model.SuccessWithMessage(c, prepared.Release, "发布已创建")
}

// RestartApplication recreates the latest successful release with the current
// managed resources, without requiring the caller to choose a template/version.

func (h *ApplicationHandler) RestartApplication(c *gin.Context) {
	if K8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	app, err := h.queries.GetApplication(applicationID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "应用不存在")
		return
	}
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	workflow := h.releaseWorkflow()
	prepared, err := workflow.Restart(c.Request.Context(), app, apiShared.UserID(c))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	workflow.ExecuteAsync(app, prepared)
	model.SuccessWithMessage(c, prepared.Release, "应用重启已创建")
}

func (h *ApplicationHandler) RetryRelease(c *gin.Context) {
	if K8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	releaseID, err := apiShared.ParseID(c.Param("releaseID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "发布 ID 无效")
		return
	}
	original, err := h.store.GetRelease(releaseID)
	if err != nil || original.ApplicationID != applicationID {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "发布不存在")
		return
	}
	workflow := h.releaseWorkflow()
	prepared, err := workflow.Retry(releaseID, apiShared.UserID(c))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "无法重试该发布")
		return
	}
	app, err := h.queries.GetApplication(applicationID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "应用不存在")
		return
	}
	workflow.ExecuteAsync(app, prepared)
	model.SuccessWithMessage(c, prepared.Release, "重试已创建")
}

func (h *ApplicationHandler) RollbackRelease(c *gin.Context) {
	if K8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	releaseID, err := apiShared.ParseID(c.Param("releaseID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "发布 ID 无效")
		return
	}
	original, err := h.store.GetRelease(releaseID)
	if err != nil || original.ApplicationID != applicationID {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "发布不存在")
		return
	}
	workflow := h.releaseWorkflow()
	prepared, err := workflow.Rollback(releaseID, apiShared.UserID(c))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "无法回滚该发布")
		return
	}
	app, err := h.queries.GetApplication(applicationID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "应用不存在")
		return
	}
	workflow.ExecuteAsync(app, prepared)
	model.SuccessWithMessage(c, prepared.Release, "回滚已创建")
}

func (h *ApplicationHandler) GetRelease(c *gin.Context) {
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	id, err := apiShared.ParseID(c.Param("releaseID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "发布 ID 无效")
		return
	}
	release, err := h.store.GetRelease(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "发布不存在")
		return
	}
	if release.ApplicationID != applicationID {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "发布不存在")
		return
	}
	if !release.PodTrackingEnabled {
		release.Runtime = &model.ReleaseRuntime{Tracking: "legacy_untracked", Pods: []model.ReleasePodRuntime{}, Diagnostic: "该历史发布未记录 Pod 关联标签，无法精确查询当前运行态"}
		model.Success(c, release)
		return
	}
	if K8s == nil {
		release.Runtime = &model.ReleaseRuntime{Tracking: "unavailable", Pods: []model.ReleasePodRuntime{}, Diagnostic: "Kubernetes 集群未连接，无法读取 Pod 运行态"}
		model.Success(c, release)
		return
	}
	applicationModel, err := h.queries.GetApplication(applicationID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "应用不存在")
		return
	}
	runtime, err := application.NewKubernetesApplier(K8s).InspectReleasePods(c.Request.Context(), application.ApplicationContext{Namespace: applicationModel.Environment.Namespace, ApplicationName: applicationModel.Name, ReleaseSequence: release.Sequence})
	if err != nil {
		release.Runtime = &model.ReleaseRuntime{Tracking: "unavailable", Pods: []model.ReleasePodRuntime{}, Diagnostic: "读取 Pod 运行态失败: " + err.Error()}
	} else {
		release.Runtime = runtime
	}
	model.Success(c, release)
}

func applicationContextFor(app *model.Application) application.ApplicationContext {
	return application.ApplicationContext{ProjectID: app.ProjectID, EnvironmentID: app.EnvironmentID, ProjectName: app.Project.Name, EnvironmentName: app.Environment.Name, ApplicationName: app.Name, Namespace: app.Environment.Namespace, WorkloadKind: app.WorkloadKind}
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

func (h *ApplicationHandler) syncApplicationEndpoints(ctx context.Context, app *model.Application, service application.ServiceSpec) error {
	return h.releaseWorkflow().SyncApplicationEndpoints(ctx, app, service)
}
