package applicationapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/auth"
	"github.com/cylism/cylism-manager/internal/model"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type configMapInfo struct {
	ID               uint   `json:"id"`
	ResourceKind     string `json:"resource_kind"`
	ResourceName     string `json:"resource_name"`
	Key              string `json:"key"`
	MountPath        string `json:"mount_path"`
	Format           string `json:"format"`
	Version          uint   `json:"version"`
	TemplateID       uint   `json:"template_id"`
	TemplateRevision uint   `json:"template_revision"`
}

// managedFileInfo remains an internal name for the regular application
// resource page; integration clients use the ConfigMap-specific contract.
type managedFileInfo = configMapInfo

type configMapReplaceRequest struct {
	Content          string `json:"content"`
	ExpectedRevision uint   `json:"expected_revision"`
	Restart          bool   `json:"restart"`
}

var managedFileMutationMu sync.Mutex

func (h *ApplicationHandler) ListManagedFiles(c *gin.Context) {
	app, ok := h.applicationForParam(c)
	if !ok {
		return
	}
	files, err := h.resources.ListApplicationManagedFiles(app.ID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	result := make([]managedFileInfo, 0, len(files))
	for _, file := range files {
		result = append(result, managedFileInfo{ID: file.ID, ResourceKind: file.ResourceKind, ResourceName: file.ResourceName, Key: file.Key, MountPath: file.MountPath, Format: file.Format, Version: file.Version})
	}
	model.Success(c, result)
}

func (h *ApplicationHandler) CreateDelegation(c *gin.Context) {
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	app, err := h.queries.GetApplication(applicationID)
	if err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	var req delegationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "委托参数无效")
		return
	}
	if strings.TrimSpace(req.Capability) == "" {
		apiShared.ValidationError(c, "委托 capability 必填")
		return
	}
	normalized, normalizeErr := model.NormalizeApplicationCapabilities([]string{req.Capability})
	if normalizeErr != nil || !applicationHasCapability(*app, normalized[0]) {
		apiShared.ValidationError(c, "委托 capability 必须属于目标应用")
		return
	}
	if len(req.EnvironmentIDs) == 0 {
		req.EnvironmentIDs = []uint{app.EnvironmentID}
	}
	for _, environmentID := range req.EnvironmentIDs {
		environment, environmentErr := h.queries.GetEnvironment(app.ProjectID, environmentID)
		if environmentErr != nil || environment.ProjectID != app.ProjectID {
			apiShared.ValidationError(c, "委托环境不属于应用项目")
			return
		}
	}
	if len(req.Actions) == 0 {
		req.Actions = []string{"application:read", "configmap:read", "configmap:write", "application:restart"}
	}
	if len(h.delegationSecret) == 0 {
		apiShared.DBError(c, "委托签名未配置")
		return
	}
	token, err := auth.GenerateDelegationToken(h.delegationSecret, auth.DelegationClaims{
		UserID: apiShared.UserID(c), Username: apiShared.Username(c), ProjectID: app.ProjectID,
		EnvironmentIDs: req.EnvironmentIDs, Capability: normalized[0], Actions: req.Actions,
		ApplicationIDs: []uint{app.ID},
	}, auth.MaxDelegationTTL)
	if err != nil {
		apiShared.DBError(c, "签发委托失败")
		return
	}
	model.Success(c, gin.H{"token": token, "expires_in": int(auth.MaxDelegationTTL.Seconds())})
}

func (h *ApplicationHandler) IntegrationDiscoverApplications(c *gin.Context) {
	claims := delegationClaims(c)
	if !claims.Allows("application:read") {
		integrationForbidden(c)
		return
	}
	projectID, err := apiShared.OptionalID(strings.TrimSpace(c.Query("project_id")))
	if err != nil || (projectID != 0 && projectID != claims.ProjectID) {
		integrationForbidden(c)
		return
	}
	environmentID, err := apiShared.OptionalID(strings.TrimSpace(c.Query("environment_id")))
	if err != nil || (environmentID != 0 && !claims.AllowsEnvironment(environmentID)) {
		integrationForbidden(c)
		return
	}
	applications, err := h.queries.ListApplications(claims.ProjectID, environmentID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	filtered := make([]model.Application, 0, len(applications))
	for _, app := range applications {
		if claims.AllowsEnvironment(app.EnvironmentID) && claims.AllowsApplication(app.ID) && applicationHasCapability(app, claims.Capability) {
			filtered = append(filtered, app)
		}
	}
	releases, err := h.queries.ListReleasesByApplications(applicationIDs(filtered))
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	runtimes := h.queries.ApplicationRuntimeInfos(c.Request.Context(), h.kubernetes, filtered, releases)
	infos := make([]applicationDiscoveryInfo, 0, len(filtered))
	for _, app := range filtered {
		infos = append(infos, applicationDiscoveryInfoFromModel(app, runtimes[app.ID]))
	}
	model.Success(c, infos)
}

func (h *ApplicationHandler) IntegrationGetApplicationRuntime(c *gin.Context) {
	app, ok := h.integrationApplication(c, "application:read")
	if !ok {
		return
	}
	releases, err := h.queries.ListReleases(app.ID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	model.Success(c, h.queries.ApplicationRuntimeInfos(c.Request.Context(), h.kubernetes, []model.Application{*app}, map[uint][]model.Release{app.ID: releases})[app.ID])
}

func (h *ApplicationHandler) IntegrationListManagedConfigMaps(c *gin.Context) {
	app, ok := h.integrationApplication(c, "configmap:read")
	if !ok {
		return
	}
	template, err := h.resources.GetDefaultApplicationDeploymentTemplate(app.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apiShared.NotFound(c, "应用没有可用的默认上线模板")
			return
		}
		apiShared.DBError(c, err.Error())
		return
	}
	if !template.Enabled {
		apiShared.NotFound(c, "应用没有可用的默认上线模板")
		return
	}
	files, err := h.resources.ListApplicationManagedFiles(app.ID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	var spec applicationservice.ReleaseSpec
	if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
		apiShared.DBError(c, "读取默认模板 ConfigMap 配置失败")
		return
	}
	result := make([]configMapInfo, 0, len(files))
	for _, file := range files {
		if !file.Enabled || file.ResourceKind != applicationservice.FileMountSourceConfigMap || file.ResourceName != app.Name+"-config" {
			continue
		}
		if !configMapKeyEnabled(spec, file.Key) || !applicationservice.IsConfigKeyManaged(spec, file.Key) {
			continue
		}
		result = append(result, configMapInfo{ID: file.ID, ResourceKind: file.ResourceKind, ResourceName: file.ResourceName, Key: file.Key, MountPath: file.MountPath, Format: file.Format, Version: template.Revision, TemplateID: template.ID, TemplateRevision: template.Revision})
	}
	model.Success(c, result)
}

func (h *ApplicationHandler) IntegrationGetManagedConfigMap(c *gin.Context) {
	app, ok := h.integrationApplication(c, "configmap:read")
	if !ok {
		return
	}
	configMapID, err := apiShared.ParseID(c.Param("configMapID"))
	if err != nil {
		apiShared.BadRequest(c, "ConfigMap 配置 ID 无效")
		return
	}
	file, err := h.resources.GetApplicationManagedFile(app.ID, configMapID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "受管 ConfigMap 配置不存在")
		return
	}
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	template, spec, err := h.templateConfigMap(app, file)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	model.Success(c, gin.H{"id": file.ID, "resource_kind": file.ResourceKind, "resource_name": file.ResourceName, "key": file.Key, "mount_path": file.MountPath, "format": file.Format, "version": template.Revision, "template_id": template.ID, "template_revision": template.Revision, "content": spec.Config[file.Key]})
}

func (h *ApplicationHandler) IntegrationReplaceManagedConfigMap(c *gin.Context) {
	app, ok := h.integrationApplication(c, "configmap:write")
	if !ok {
		return
	}
	configMapID, err := apiShared.ParseID(c.Param("configMapID"))
	if err != nil {
		apiShared.BadRequest(c, "ConfigMap 配置 ID 无效")
		return
	}
	var req configMapReplaceRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ExpectedRevision == 0 {
		apiShared.BadRequest(c, "ConfigMap 配置参数无效")
		return
	}
	if len(req.Content) > 4<<20 {
		apiShared.Error(c, http.StatusRequestEntityTooLarge, model.CodeValidationFail, "ConfigMap 内容不能超过 4 MiB")
		return
	}
	if req.Restart && !delegationClaims(c).Allows("application:restart") {
		integrationForbidden(c)
		return
	}
	managedFileMutationMu.Lock()
	defer managedFileMutationMu.Unlock()
	file, err := h.resources.GetApplicationManagedFile(app.ID, configMapID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "受管 ConfigMap 配置不存在")
		return
	}
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	template, spec, err := h.templateConfigMap(app, file)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if template.Revision != req.ExpectedRevision {
		apiShared.Conflict(c, fmt.Sprintf("模板版本冲突，当前版本为 %d", template.Revision))
		return
	}
	spec.Config[file.Key] = req.Content
	updated, err := h.templateFromRequest(c.Request.Context(), app, &deploymentTemplateRequest{Name: template.Name, Description: template.Description, Enabled: template.Enabled, Spec: spec}, template.ID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	updated.UpdatedBy = apiShared.UserID(c)
	if err := h.resources.UpdateApplicationDeploymentTemplateIfRevision(updated, req.ExpectedRevision); err != nil {
		var conflict *model.TemplateRevisionConflictError
		if errors.As(err, &conflict) {
			apiShared.Conflict(c, conflict.Error())
			return
		}
		apiShared.DBError(c, err.Error())
		return
	}
	response := gin.H{"version": updated.Revision, "template_id": updated.ID, "template_revision": updated.Revision}
	if req.Restart {
		release, restartErr := h.createRestartRelease(c.Request.Context(), app, apiShared.UserID(c))
		if restartErr != nil {
			response["restart_pending"] = true
			response["restart_error"] = "ConfigMap 配置已保存，重新发布创建失败"
		} else {
			response["release_id"] = release.ID
		}
	}
	model.Success(c, response)
}

func (h *ApplicationHandler) IntegrationRestartApplication(c *gin.Context) {
	app, ok := h.integrationApplication(c, "application:restart")
	if !ok {
		return
	}
	release, err := h.createRestartRelease(c.Request.Context(), app, apiShared.UserID(c))
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	model.Success(c, gin.H{"release_id": release.ID, "status": release.Status})
}

func (h *ApplicationHandler) IntegrationGetRelease(c *gin.Context) {
	app, ok := h.integrationApplication(c, "application:read")
	if !ok {
		return
	}
	releaseID, err := apiShared.ParseID(c.Param("releaseID"))
	if err != nil {
		apiShared.BadRequest(c, "发布 ID 无效")
		return
	}
	release, err := h.resources.GetRelease(releaseID)
	if errors.Is(err, gorm.ErrRecordNotFound) || release.ApplicationID != app.ID {
		apiShared.NotFound(c, "发布不存在")
		return
	}
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	model.Success(c, applicationReleaseSummary{ID: release.ID, Sequence: release.Sequence, Version: release.Version, Status: release.Status})
}

func (h *ApplicationHandler) applicationForParam(c *gin.Context) (*model.Application, bool) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return nil, false
	}
	app, err := h.queries.GetApplication(id)
	if err != nil {
		apiShared.NotFound(c, "应用不存在")
		return nil, false
	}
	return app, true
}

func (h *ApplicationHandler) integrationApplication(c *gin.Context, action string) (*model.Application, bool) {
	claims := delegationClaims(c)
	if !claims.Allows(action) {
		integrationForbidden(c)
		return nil, false
	}
	app, ok := h.applicationForParam(c)
	if !ok {
		return nil, false
	}
	if app.ProjectID != claims.ProjectID || !claims.AllowsEnvironment(app.EnvironmentID) || !claims.AllowsApplication(app.ID) || !applicationHasCapability(*app, claims.Capability) {
		integrationForbidden(c)
		return nil, false
	}
	return app, true
}

func delegationClaims(c *gin.Context) *auth.DelegationClaims {
	claims, _ := c.Get("delegation")
	delegation, _ := claims.(*auth.DelegationClaims)
	return delegation
}

func integrationForbidden(c *gin.Context) {
	apiShared.Error(c, http.StatusForbidden, model.CodeUnauthorized, "委托范围不允许该操作")
}

// templateConfigMap returns the ConfigMap section from the default template.
// The rendered Kubernetes ConfigMap is deliberately never read here: it is a
// materialized copy of the template and may be stale until a release applies it.
func (h *ApplicationHandler) templateConfigMap(app *model.Application, file *model.ApplicationManagedFile) (*model.ApplicationDeploymentTemplate, applicationservice.ReleaseSpec, error) {
	if file.ResourceKind != applicationservice.FileMountSourceConfigMap || file.ResourceName != app.Name+"-config" || !file.Enabled {
		return nil, applicationservice.ReleaseSpec{}, fmt.Errorf("受管配置不是当前应用的 ConfigMap")
	}
	template, err := h.resources.GetDefaultApplicationDeploymentTemplate(app.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, applicationservice.ReleaseSpec{}, fmt.Errorf("应用没有可用的默认上线模板")
		}
		return nil, applicationservice.ReleaseSpec{}, fmt.Errorf("读取默认上线模板: %w", err)
	}
	if !template.Enabled {
		return nil, applicationservice.ReleaseSpec{}, fmt.Errorf("应用没有可用的默认上线模板")
	}
	var spec applicationservice.ReleaseSpec
	if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
		return nil, applicationservice.ReleaseSpec{}, fmt.Errorf("读取上线模板配置: %w", err)
	}
	if spec.Config == nil {
		return nil, applicationservice.ReleaseSpec{}, fmt.Errorf("模板没有 ConfigMap 配置")
	}
	if !configMapKeyEnabled(spec, file.Key) {
		return nil, applicationservice.ReleaseSpec{}, fmt.Errorf("模板 ConfigMap 不包含已启用的键 %q", file.Key)
	}
	if !applicationservice.IsConfigKeyManaged(spec, file.Key) {
		return nil, applicationservice.ReleaseSpec{}, fmt.Errorf("ConfigMap 键 %q 未开放外部应用修改", file.Key)
	}
	return template, spec, nil
}

func configMapKeyEnabled(spec applicationservice.ReleaseSpec, key string) bool {
	if spec.Config == nil {
		return false
	}
	if _, exists := spec.Config[key]; !exists {
		return false
	}
	for _, disabled := range spec.ConfigDisabled {
		if disabled == key {
			return false
		}
	}
	return true
}

func (h *ApplicationHandler) createRestartRelease(ctx context.Context, app *model.Application, userID uint) (*model.Release, error) {
	workflow := h.releaseWorkflow()
	prepared, err := workflow.Restart(ctx, app, userID)
	if err != nil {
		return nil, err
	}
	workflow.ExecuteAsync(ctx, app, prepared)
	return prepared.Release, nil
}
