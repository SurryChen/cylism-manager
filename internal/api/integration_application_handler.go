package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cylism/cylism-manager/internal/application"
	"github.com/cylism/cylism-manager/internal/auth"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
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
	files, err := h.store.ListApplicationManagedFiles(app.ID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	result := make([]managedFileInfo, 0, len(files))
	for _, file := range files {
		result = append(result, managedFileInfo{ID: file.ID, ResourceKind: file.ResourceKind, ResourceName: file.ResourceName, Key: file.Key, MountPath: file.MountPath, Format: file.Format, Version: file.Version})
	}
	model.Success(c, result)
}

func (h *ApplicationHandler) syncManagedFilesForSpec(app *model.Application, spec application.ReleaseSpec, userID uint) error {
	application.NormalizeManagedKeys(&spec)
	bindings := make(map[string]struct{})
	for _, managed := range []struct {
		keys   []string
		kind   string
		name   string
		secret bool
	}{
		{keys: spec.ConfigManagedKeys, kind: application.FileMountSourceConfigMap, name: app.Name + "-config"},
		{keys: spec.SecretManagedKeys, kind: application.FileMountSourceSecret, name: app.Name + "-secret", secret: true},
	} {
		for _, key := range managed.keys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			bindings[managedFileBinding(managed.kind, managed.name, key)] = struct{}{}
			file := &model.ApplicationManagedFile{ApplicationID: app.ID, ResourceKind: managed.kind, ResourceName: managed.name, Key: key, MountPath: managedMountPath(spec, managed.secret, key), Format: "text", CreatedBy: userID, Enabled: true}
			if err := h.store.UpsertApplicationManagedFile(file); err != nil {
				return err
			}
		}
	}
	// Keep old templates working when they have not yet been saved through the
	// new key-level editor; NormalizeManagedKeys above performs the migration.
	return h.store.DisableApplicationManagedFilesNotIn(app.ID, bindings)
}

func managedMountPath(spec application.ReleaseSpec, secret bool, key string) string {
	wantSource := application.FileMountSourceApplicationConfig
	if secret {
		wantSource = application.FileMountSourceApplicationSecret
	}
	for _, mount := range spec.FileMounts {
		if mount.SourceType == wantSource && mount.Key == key {
			return mount.MountPath
		}
	}
	return ""
}

func managedFileBinding(kind, name, key string) string {
	return strings.Join([]string{kind, name, key}, "\x00")
}

type delegationRequest struct {
	EnvironmentIDs []uint   `json:"environment_ids"`
	Capability     string   `json:"capability"`
	Actions        []string `json:"actions"`
}

const (
	integrationHandoffTTL = 60 * time.Second
	integrationSessionTTL = 8 * time.Hour
)

func randomOpaqueValue(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func opaqueHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum[:])
}

func (h *ApplicationHandler) CreateIntegrationHandoff(c *gin.Context) {
	h.createIntegrationHandoff(c)
}

type integrationHandoffRequest struct {
	EndpointID  uint   `json:"endpoint_id"`
	RedirectURL string `json:"redirect_url"`
}

// createIntegrationHandoff creates a one-time browser handoff for a generic
// application link. The platform does not inspect the link's purpose.
func (h *ApplicationHandler) createIntegrationHandoff(c *gin.Context) {
	applicationID, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	app, err := h.store.GetApplication(applicationID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "应用不存在")
		return
	}
	var req integrationHandoffRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.EndpointID == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用入口必填")
		return
	}
	endpoint, err := h.store.GetApplicationEndpoint(app.ID, req.EndpointID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "应用入口不存在")
		return
	}
	if endpoint.AccessMode != model.ApplicationEndpointAccessProtectedConsole {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "该入口不是受保护控制台")
		return
	}
	// Validate and construct the redirect before persisting the one-time session.
	if endpoint.Domain == "" {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "应用入口尚未绑定域名")
		return
	}
	scheme := "http"
	if endpoint.TLSEnabled {
		scheme = "https"
	}
	endpointURL := fmt.Sprintf("%s://%s%s", scheme, endpoint.Domain, endpoint.Path)
	if strings.TrimSpace(req.RedirectURL) != "" {
		localURL, err := parseLoopbackRedirectURL(req.RedirectURL)
		if err != nil {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "本地联调地址无效，仅支持 http(s)://localhost、127.0.0.1 或 ::1")
			return
		}
		endpointURL = localURL.String()
	}
	actions := []string{"application:read", "configmap:read", "configmap:write", "application:restart"}
	now := time.Now()
	code, err := randomOpaqueValue(32)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "创建管理会话失败")
		return
	}
	session := &model.IntegrationSession{HandoffCodeHash: opaqueHash(code), UserID: getUserID(c), ProjectID: app.ProjectID, ApplicationID: app.ID, EnvironmentID: app.EnvironmentID, ActionsData: strings.Join(actions, ","), HandoffExpiresAt: now.Add(integrationHandoffTTL), ExpiresAt: now.Add(integrationSessionTTL)}
	if err := h.store.CreateIntegrationSession(session); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "创建管理会话失败")
		return
	}
	handoffURL, err := appendHandoffCode(endpointURL, code)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "生成跳转地址失败")
		return
	}
	model.Success(c, gin.H{"handoff_code": code, "handoff_url": handoffURL, "expires_in": int(integrationHandoffTTL.Seconds())})
}

func appendHandoffCode(raw, code string) (string, error) {
	u, err := parseExternalLink(raw)
	if err != nil {
		return "", err
	}
	query := u.Query()
	query.Set("handoff_code", code)
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func parseExternalLink(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("invalid external link")
	}
	return u, nil
}

// parseLoopbackRedirectURL accepts only a browser-local HTTP(S) URL. It is used
// for local development handoffs and prevents the handoff endpoint from acting
// as an arbitrary external redirector.
func parseLoopbackRedirectURL(raw string) (*url.URL, error) {
	u, err := parseExternalLink(raw)
	if err != nil || u.User != nil {
		return nil, errors.New("invalid local redirect URL")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" {
		return u, nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return u, nil
	}
	return nil, errors.New("redirect URL must use a loopback host")
}

func bearerValue(c *gin.Context) string {
	parts := strings.SplitN(c.GetHeader("Authorization"), " ", 2)
	if len(parts) == 2 && parts[0] == "Bearer" {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func (h *ApplicationHandler) ExchangeIntegrationSession(c *gin.Context) {
	code := bearerValue(c)
	if code == "" {
		model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "未提供跳转码")
		return
	}
	now := time.Now()
	token, err := randomOpaqueValue(48)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "交换管理会话失败")
		return
	}
	session, err := h.store.ExchangeIntegrationSession(opaqueHash(code), opaqueHash(token), now.Add(integrationSessionTTL), now)
	if err != nil {
		model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "跳转码无效或已过期")
		return
	}
	model.Success(c, gin.H{"session_token": token, "expires_at": session.ExpiresAt})
}

func (h *ApplicationHandler) CreateIntegrationDelegation(c *gin.Context) {
	h.createIntegrationDelegation(c)
}

type integrationDelegationRequest struct {
	Capability string `json:"capability"`
}

func (h *ApplicationHandler) createIntegrationDelegation(c *gin.Context) {
	token := bearerValue(c)
	if token == "" {
		model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "未提供管理会话")
		return
	}
	session, err := h.store.GetActiveIntegrationSession(opaqueHash(token), time.Now())
	if err != nil {
		model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "管理会话无效或已过期")
		return
	}
	actions := strings.Split(session.ActionsData, ",")
	var req integrationDelegationRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Capability) == "" {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "capability 必填")
		return
	}
	capability, err := model.NormalizeApplicationCapabilities([]string{req.Capability})
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "capability 格式无效")
		return
	}
	delegation, err := auth.GenerateDelegationToken(h.delegationSecret, auth.DelegationClaims{UserID: session.UserID, ProjectID: session.ProjectID, EnvironmentIDs: []uint{session.EnvironmentID}, Capability: capability[0], Actions: actions}, auth.MaxDelegationTTL)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "签发委托失败")
		return
	}
	model.Success(c, gin.H{"token": delegation, "expires_in": int(auth.MaxDelegationTTL.Seconds())})
}

func (h *ApplicationHandler) CreateDelegation(c *gin.Context) {
	applicationID, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	app, err := h.store.GetApplication(applicationID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "应用不存在")
		return
	}
	var req delegationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "委托参数无效")
		return
	}
	if strings.TrimSpace(req.Capability) == "" {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "委托 capability 必填")
		return
	}
	normalized, normalizeErr := model.NormalizeApplicationCapabilities([]string{req.Capability})
	if normalizeErr != nil || !applicationHasCapability(*app, normalized[0]) {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "委托 capability 必须属于目标应用")
		return
	}
	if len(req.EnvironmentIDs) == 0 {
		req.EnvironmentIDs = []uint{app.EnvironmentID}
	}
	for _, environmentID := range req.EnvironmentIDs {
		environment, environmentErr := h.store.GetEnvironment(app.ProjectID, environmentID)
		if environmentErr != nil || environment.ProjectID != app.ProjectID {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "委托环境不属于应用项目")
			return
		}
	}
	if len(req.Actions) == 0 {
		req.Actions = []string{"application:read", "configmap:read", "configmap:write", "application:restart"}
	}
	if len(h.delegationSecret) == 0 {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "委托签名未配置")
		return
	}
	token, err := auth.GenerateDelegationToken(h.delegationSecret, auth.DelegationClaims{
		UserID: getUserID(c), Username: c.GetString("username"), ProjectID: app.ProjectID,
		EnvironmentIDs: req.EnvironmentIDs, Capability: normalized[0], Actions: req.Actions,
		ApplicationIDs: []uint{app.ID},
	}, auth.MaxDelegationTTL)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "签发委托失败")
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
	projectID, err := optionalQueryID(c, "project_id")
	if err != nil || (projectID != 0 && projectID != claims.ProjectID) {
		integrationForbidden(c)
		return
	}
	environmentID, err := optionalQueryID(c, "environment_id")
	if err != nil || (environmentID != 0 && !claims.AllowsEnvironment(environmentID)) {
		integrationForbidden(c)
		return
	}
	applications, err := h.store.ListApplications(claims.ProjectID, environmentID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	filtered := make([]model.Application, 0, len(applications))
	for _, app := range applications {
		if claims.AllowsEnvironment(app.EnvironmentID) && claims.AllowsApplication(app.ID) && applicationHasCapability(app, claims.Capability) {
			filtered = append(filtered, app)
		}
	}
	releases, err := h.store.ListReleasesByApplications(applicationIDs(filtered))
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	runtimes := h.applicationRuntimeInfos(c.Request.Context(), filtered, releases)
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
	releases, err := h.store.ListReleases(app.ID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, h.applicationRuntimeInfos(c.Request.Context(), []model.Application{*app}, map[uint][]model.Release{app.ID: releases})[app.ID])
}

func (h *ApplicationHandler) IntegrationListManagedConfigMaps(c *gin.Context) {
	app, ok := h.integrationApplication(c, "configmap:read")
	if !ok {
		return
	}
	template, err := h.store.GetDefaultApplicationDeploymentTemplate(app.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			model.Error(c, http.StatusNotFound, model.CodeNotFound, "应用没有可用的默认上线模板")
			return
		}
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	if !template.Enabled {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "应用没有可用的默认上线模板")
		return
	}
	files, err := h.store.ListApplicationManagedFiles(app.ID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	var spec application.ReleaseSpec
	if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取默认模板 ConfigMap 配置失败")
		return
	}
	result := make([]configMapInfo, 0, len(files))
	for _, file := range files {
		if !file.Enabled || file.ResourceKind != application.FileMountSourceConfigMap || file.ResourceName != app.Name+"-config" {
			continue
		}
		if !configMapKeyEnabled(spec, file.Key) || !application.IsConfigKeyManaged(spec, file.Key) {
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
	configMapID, err := parseID(c.Param("configMapID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "ConfigMap 配置 ID 无效")
		return
	}
	file, err := h.store.GetApplicationManagedFile(app.ID, configMapID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "受管 ConfigMap 配置不存在")
		return
	}
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	template, spec, err := h.templateConfigMap(app, file)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	model.Success(c, gin.H{"id": file.ID, "resource_kind": file.ResourceKind, "resource_name": file.ResourceName, "key": file.Key, "mount_path": file.MountPath, "format": file.Format, "version": template.Revision, "template_id": template.ID, "template_revision": template.Revision, "content": spec.Config[file.Key]})
}

func (h *ApplicationHandler) IntegrationReplaceManagedConfigMap(c *gin.Context) {
	app, ok := h.integrationApplication(c, "configmap:write")
	if !ok {
		return
	}
	configMapID, err := parseID(c.Param("configMapID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "ConfigMap 配置 ID 无效")
		return
	}
	var req configMapReplaceRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ExpectedRevision == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "ConfigMap 配置参数无效")
		return
	}
	if len(req.Content) > 4<<20 {
		model.Error(c, http.StatusRequestEntityTooLarge, model.CodeValidationFail, "ConfigMap 内容不能超过 4 MiB")
		return
	}
	if req.Restart && !delegationClaims(c).Allows("application:restart") {
		integrationForbidden(c)
		return
	}
	managedFileMutationMu.Lock()
	defer managedFileMutationMu.Unlock()
	file, err := h.store.GetApplicationManagedFile(app.ID, configMapID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "受管 ConfigMap 配置不存在")
		return
	}
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	template, spec, err := h.templateConfigMap(app, file)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if template.Revision != req.ExpectedRevision {
		model.Error(c, http.StatusConflict, model.CodeConflict, fmt.Sprintf("模板版本冲突，当前版本为 %d", template.Revision))
		return
	}
	spec.Config[file.Key] = req.Content
	updated, err := h.templateFromRequest(app, &deploymentTemplateRequest{Name: template.Name, Description: template.Description, Enabled: template.Enabled, Spec: spec}, template.ID)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	updated.UpdatedBy = getUserID(c)
	if err := h.store.UpdateApplicationDeploymentTemplateIfRevision(updated, req.ExpectedRevision); err != nil {
		var conflict *store.TemplateRevisionConflictError
		if errors.As(err, &conflict) {
			model.Error(c, http.StatusConflict, model.CodeConflict, conflict.Error())
			return
		}
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	response := gin.H{"version": updated.Revision, "template_id": updated.ID, "template_revision": updated.Revision}
	if req.Restart {
		release, restartErr := h.createRestartRelease(c.Request.Context(), app, getUserID(c))
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
	release, err := h.createRestartRelease(c.Request.Context(), app, getUserID(c))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	model.Success(c, gin.H{"release_id": release.ID, "status": release.Status})
}

func (h *ApplicationHandler) IntegrationGetRelease(c *gin.Context) {
	app, ok := h.integrationApplication(c, "application:read")
	if !ok {
		return
	}
	releaseID, err := parseID(c.Param("releaseID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "发布 ID 无效")
		return
	}
	release, err := h.store.GetRelease(releaseID)
	if errors.Is(err, gorm.ErrRecordNotFound) || release.ApplicationID != app.ID {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "发布不存在")
		return
	}
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, applicationReleaseSummary{ID: release.ID, Sequence: release.Sequence, Version: release.Version, Status: release.Status})
}

func (h *ApplicationHandler) applicationForParam(c *gin.Context) (*model.Application, bool) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return nil, false
	}
	app, err := h.store.GetApplication(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "应用不存在")
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
	model.Error(c, http.StatusForbidden, model.CodeUnauthorized, "委托范围不允许该操作")
}

// templateConfigMap returns the ConfigMap section from the default template.
// The rendered Kubernetes ConfigMap is deliberately never read here: it is a
// materialized copy of the template and may be stale until a release applies it.
func (h *ApplicationHandler) templateConfigMap(app *model.Application, file *model.ApplicationManagedFile) (*model.ApplicationDeploymentTemplate, application.ReleaseSpec, error) {
	if file.ResourceKind != application.FileMountSourceConfigMap || file.ResourceName != app.Name+"-config" || !file.Enabled {
		return nil, application.ReleaseSpec{}, fmt.Errorf("受管配置不是当前应用的 ConfigMap")
	}
	template, err := h.store.GetDefaultApplicationDeploymentTemplate(app.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, application.ReleaseSpec{}, fmt.Errorf("应用没有可用的默认上线模板")
		}
		return nil, application.ReleaseSpec{}, fmt.Errorf("读取默认上线模板: %w", err)
	}
	if !template.Enabled {
		return nil, application.ReleaseSpec{}, fmt.Errorf("应用没有可用的默认上线模板")
	}
	var spec application.ReleaseSpec
	if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
		return nil, application.ReleaseSpec{}, fmt.Errorf("读取上线模板配置: %w", err)
	}
	if spec.Config == nil {
		return nil, application.ReleaseSpec{}, fmt.Errorf("模板没有 ConfigMap 配置")
	}
	if !configMapKeyEnabled(spec, file.Key) {
		return nil, application.ReleaseSpec{}, fmt.Errorf("模板 ConfigMap 不包含已启用的键 %q", file.Key)
	}
	if !application.IsConfigKeyManaged(spec, file.Key) {
		return nil, application.ReleaseSpec{}, fmt.Errorf("ConfigMap 键 %q 未开放外部应用修改", file.Key)
	}
	return template, spec, nil
}

func configMapKeyEnabled(spec application.ReleaseSpec, key string) bool {
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
	template, err := h.store.GetDefaultApplicationDeploymentTemplate(app.ID)
	if err != nil || !template.Enabled {
		return nil, fmt.Errorf("应用没有可用的默认上线模板")
	}
	active, err := h.store.GetLatestSuccessfulRelease(app.ID)
	if err != nil {
		return nil, fmt.Errorf("应用尚无可重启的成功发布")
	}
	var spec application.ReleaseSpec
	if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
		return nil, fmt.Errorf("读取上线模板失败")
	}
	secrets, err := h.decryptTemplateSecrets(template)
	if err != nil {
		return nil, fmt.Errorf("读取模板 Secret 失败")
	}
	spec.Secrets = secrets
	spec.Image = active.Image
	spec.Version = active.Version
	// Restart releases must rebuild the same registry and node-mirror runtime
	// settings as a normal publish. The persisted release snapshot deliberately
	// omits these transient credentials and verification endpoints.
	if err := h.prepareRegistryReleaseSpec(app, &spec); err != nil {
		return nil, err
	}
	if err := h.prepareReleaseImageVerification(&spec); err != nil {
		return nil, err
	}
	if err := h.applyApplicationEndpointSpec(app, &spec); err != nil {
		return nil, err
	}
	service := application.NewService(h.store, application.NewKubernetesApplier(K8s))
	release, err := service.CreateRelease(ctx, app.ID, userID, spec)
	if err != nil {
		return nil, err
	}
	templateID := template.ID
	release.TemplateID, release.TemplateRevision = &templateID, template.Revision
	if err := h.store.UpdateRelease(release); err != nil {
		return nil, err
	}
	h.executeAsync(service, release.ID, app, spec)
	return release, nil
}
