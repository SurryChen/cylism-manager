package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cylism/cylism-manager/internal/application"
	"github.com/cylism/cylism-manager/internal/auth"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var managedDocumentMutationMu sync.Mutex

type managedDocumentRequest struct {
	ResourceKind string   `json:"resource_kind"`
	ResourceName string   `json:"resource_name"`
	Key          string   `json:"key"`
	Format       string   `json:"format"`
	AllowedPaths []string `json:"allowed_paths"`
	SummaryPath  string   `json:"summary_path,omitempty"`
}

type managedDocumentInfo struct {
	ID           uint     `json:"id"`
	Format       string   `json:"format"`
	AllowedPaths []string `json:"allowed_paths"`
	Version      uint     `json:"version"`
	Enabled      bool     `json:"enabled"`
	ResourceKind string   `json:"resource_kind,omitempty"`
	ResourceName string   `json:"resource_name,omitempty"`
	Key          string   `json:"key,omitempty"`
	SummaryPath  string   `json:"summary_path,omitempty"`
	Keys         []string `json:"keys,omitempty"`
}

type managedDocumentPatchRequest struct {
	ExpectedVersion uint                     `json:"expected_version"`
	Operations      []managedDocumentPatchOp `json:"operations"`
	Restart         bool                     `json:"restart"`
}

type managedDocumentPatchOp struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

type delegationRequest struct {
	EnvironmentIDs []uint   `json:"environment_ids"`
	Capability     string   `json:"capability"`
	Actions        []string `json:"actions"`
}

const (
	consoleHandoffTTL = 60 * time.Second
	consoleSessionTTL = 8 * time.Hour
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

func handoffURL(managerURL, code string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(managerURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("invalid manager URL")
	}
	query := parsed.Query()
	query.Set("handoff_code", code)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (h *ApplicationHandler) CreateConsoleSession(c *gin.Context) {
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
	capability := "hysteria2"
	if !applicationHasCapability(*app, capability) {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "该应用不支持 Hysteria2 管理")
		return
	}
	actions := []string{"application:read", "managed_document:write", "application:restart"}
	managerURL := strings.TrimRight(h.hysteriaManagerURL, "/")
	if managerURL == "" {
		model.Error(c, http.StatusInternalServerError, model.CodeValidationFail, "Hysteria Manager 地址未配置")
		return
	}
	now := time.Now()
	code, err := randomOpaqueValue(32)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "创建管理会话失败")
		return
	}
	session := &model.IntegrationConsoleSession{HandoffCodeHash: opaqueHash(code), UserID: getUserID(c), ProjectID: app.ProjectID, ApplicationID: app.ID, EnvironmentID: app.EnvironmentID, Capability: capability, ActionsData: strings.Join(actions, ","), HandoffExpiresAt: now.Add(consoleHandoffTTL), ExpiresAt: now.Add(consoleSessionTTL)}
	if err := h.store.CreateIntegrationConsoleSession(session); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "创建管理会话失败")
		return
	}
	redirectURL, err := handoffURL(managerURL, code)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeValidationFail, "Hysteria Manager 地址无效")
		return
	}
	model.Success(c, gin.H{"handoff_code": code, "handoff_url": redirectURL, "expires_in": int(consoleHandoffTTL.Seconds())})
}

func bearerValue(c *gin.Context) string {
	parts := strings.SplitN(c.GetHeader("Authorization"), " ", 2)
	if len(parts) == 2 && parts[0] == "Bearer" {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func (h *ApplicationHandler) ExchangeConsoleSession(c *gin.Context) {
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
	session, err := h.store.ExchangeIntegrationConsoleSession(opaqueHash(code), opaqueHash(token), now.Add(consoleSessionTTL), now)
	if err != nil {
		model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "跳转码无效或已过期")
		return
	}
	model.Success(c, gin.H{"session_token": token, "expires_at": session.ExpiresAt})
}

func (h *ApplicationHandler) CreateConsoleDelegation(c *gin.Context) {
	token := bearerValue(c)
	if token == "" {
		model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "未提供管理会话")
		return
	}
	session, err := h.store.GetActiveIntegrationConsoleSession(opaqueHash(token), time.Now())
	if err != nil {
		model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "管理会话无效或已过期")
		return
	}
	actions := strings.Split(session.ActionsData, ",")
	delegation, err := auth.GenerateDelegationToken(h.delegationSecret, auth.DelegationClaims{UserID: session.UserID, ProjectID: session.ProjectID, EnvironmentIDs: []uint{session.EnvironmentID}, ApplicationIDs: []uint{session.ApplicationID}, Capability: session.Capability, Actions: actions}, auth.MaxDelegationTTL)
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
		req.Actions = []string{"application:read", "managed_document:write", "application:restart"}
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

func (h *ApplicationHandler) ListManagedDocuments(c *gin.Context) {
	app, ok := h.applicationForParam(c)
	if !ok {
		return
	}
	documents, err := h.store.ListManagedDocuments(app.ID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	infos := make([]managedDocumentInfo, 0, len(documents))
	for _, document := range documents {
		info, infoErr := documentInfo(document, true)
		if infoErr != nil {
			model.Error(c, http.StatusInternalServerError, model.CodeDBError, infoErr.Error())
			return
		}
		infos = append(infos, info)
	}
	model.Success(c, infos)
}

func (h *ApplicationHandler) CreateManagedDocument(c *gin.Context) {
	app, ok := h.applicationForParam(c)
	if !ok {
		return
	}
	var req managedDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "受控文档参数无效")
		return
	}
	if strings.TrimSpace(req.ResourceName) == "" || strings.TrimSpace(req.Key) == "" {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "资源名称和键名必填")
		return
	}
	kind, format, paths, err := model.NormalizeManagedDocument(req.ResourceKind, req.Format, req.AllowedPaths)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if req.SummaryPath != "" && !pathsPermitRaw(paths, strings.TrimSpace(req.SummaryPath)) {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "摘要路径必须位于允许路径内")
		return
	}
	if err := h.validateManagedDocumentBinding(c.Request.Context(), app, kind, req.ResourceName, req.Key); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	document := &model.ManagedDocument{ApplicationID: app.ID, ResourceKind: kind, ResourceName: strings.TrimSpace(req.ResourceName), Key: strings.TrimSpace(req.Key), Format: format, SummaryPath: strings.TrimSpace(req.SummaryPath), CreatedBy: getUserID(c)}
	if err := document.SetAllowedPaths(paths); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.store.CreateManagedDocument(document); err != nil {
		model.Error(c, http.StatusConflict, model.CodeValidationFail, "该应用资源键已存在受控文档")
		return
	}
	info, _ := documentInfo(*document, true)
	model.Success(c, info)
}

func (h *ApplicationHandler) DeleteManagedDocument(c *gin.Context) {
	app, ok := h.applicationForParam(c)
	if !ok {
		return
	}
	documentID, err := parseID(c.Param("documentID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "受控文档 ID 无效")
		return
	}
	if err := h.store.DeleteManagedDocument(app.ID, documentID); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, gin.H{"id": documentID})
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

func (h *ApplicationHandler) IntegrationListManagedDocuments(c *gin.Context) {
	app, ok := h.integrationApplication(c, "application:read")
	if !ok {
		return
	}
	documents, err := h.store.ListManagedDocuments(app.ID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	infos := make([]managedDocumentInfo, 0, len(documents))
	for _, document := range documents {
		if !document.Enabled {
			continue
		}
		info, err := h.integrationDocumentInfo(c.Request.Context(), app, document)
		if err != nil {
			model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "读取受控文档摘要失败")
			return
		}
		infos = append(infos, info)
	}
	model.Success(c, infos)
}

func (h *ApplicationHandler) IntegrationPatchManagedDocument(c *gin.Context) {
	app, ok := h.integrationApplication(c, "managed_document:write")
	if !ok {
		return
	}
	documentID, err := parseID(c.Param("documentID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "受控文档 ID 无效")
		return
	}
	var req managedDocumentPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ExpectedVersion == 0 || len(req.Operations) == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "受控文档 patch 参数无效")
		return
	}
	if req.Restart && !delegationClaims(c).Allows("application:restart") {
		integrationForbidden(c)
		return
	}
	document, err := h.store.GetManagedDocument(app.ID, documentID)
	if errors.Is(err, gorm.ErrRecordNotFound) || !document.Enabled {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "受控文档不存在")
		return
	}
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	managedDocumentMutationMu.Lock()
	defer managedDocumentMutationMu.Unlock()
	// Check the version while holding the process-wide mutation lock. Otherwise
	// a waiting request could write to Kubernetes after another request advanced
	// the database version.
	document, err = h.store.GetManagedDocument(app.ID, documentID)
	if err != nil || !document.Enabled {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "受控文档不存在")
		return
	}
	if document.Version != req.ExpectedVersion {
		model.Error(c, http.StatusConflict, model.CodeValidationFail, fmt.Sprintf("受控文档版本冲突，当前版本为 %d", document.Version))
		return
	}
	if err := h.applyManagedDocumentPatch(c.Request.Context(), app, document, req.Operations); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	version, err := h.store.AdvanceManagedDocumentVersion(app.ID, document.ID, req.ExpectedVersion)
	if err != nil {
		model.Error(c, http.StatusConflict, model.CodeValidationFail, "受控文档版本冲突")
		return
	}
	response := gin.H{"version": version}
	if req.Restart {
		release, restartErr := h.createRestartRelease(c.Request.Context(), app, getUserID(c))
		if restartErr != nil {
			response["restart_pending"] = true
			response["restart_error"] = "配置已保存，重新发布创建失败"
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

func documentInfo(document model.ManagedDocument, includeBinding bool) (managedDocumentInfo, error) {
	paths, err := document.Paths()
	if err != nil {
		return managedDocumentInfo{}, err
	}
	info := managedDocumentInfo{ID: document.ID, Format: document.Format, AllowedPaths: paths, Version: document.Version, Enabled: document.Enabled, SummaryPath: document.SummaryPath}
	if includeBinding {
		info.ResourceKind, info.ResourceName, info.Key = document.ResourceKind, document.ResourceName, document.Key
	}
	return info, nil
}

func (h *ApplicationHandler) validateManagedDocumentBinding(ctx context.Context, app *model.Application, kind, resourceName, key string) error {
	template, err := h.store.GetDefaultApplicationDeploymentTemplate(app.ID)
	if err != nil || !template.Enabled {
		return fmt.Errorf("应用没有可用的默认上线模板")
	}
	var spec application.ReleaseSpec
	if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
		return fmt.Errorf("读取上线模板失败")
	}
	found := false
	for _, mount := range spec.FileMounts {
		if mount.SourceType == kind && mount.SourceName == resourceName && mount.Key == key {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("受控文档必须精确对应默认模板中的只读文件挂载")
	}
	if K8s == nil || K8s.Clientset == nil {
		return fmt.Errorf("Kubernetes 集群未连接")
	}
	if kind == model.ManagedDocumentResourceSecret {
		secret, err := K8s.Clientset.CoreV1().Secrets(app.Environment.Namespace).Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil || secret.Data[key] == nil {
			return fmt.Errorf("文件挂载 Secret 或键不存在")
		}
		return nil
	}
	configMap, err := K8s.Clientset.CoreV1().ConfigMaps(app.Environment.Namespace).Get(ctx, resourceName, metav1.GetOptions{})
	if err != nil || configMap.Data[key] == "" {
		return fmt.Errorf("文件挂载 ConfigMap 或键不存在")
	}
	return nil
}

func (h *ApplicationHandler) integrationDocumentInfo(ctx context.Context, app *model.Application, document model.ManagedDocument) (managedDocumentInfo, error) {
	info, err := documentInfo(document, false)
	if err != nil {
		return managedDocumentInfo{}, err
	}
	if document.SummaryPath == "" || !pathsPermit(document, document.SummaryPath) || K8s == nil || K8s.Clientset == nil {
		return info, nil
	}
	var raw []byte
	if document.ResourceKind == model.ManagedDocumentResourceSecret {
		secret, getErr := K8s.Clientset.CoreV1().Secrets(app.Environment.Namespace).Get(ctx, document.ResourceName, metav1.GetOptions{})
		if getErr != nil {
			return managedDocumentInfo{}, getErr
		}
		raw = secret.Data[document.Key]
	} else {
		configMap, getErr := K8s.Clientset.CoreV1().ConfigMaps(app.Environment.Namespace).Get(ctx, document.ResourceName, metav1.GetOptions{})
		if getErr != nil {
			return managedDocumentInfo{}, getErr
		}
		raw = []byte(configMap.Data[document.Key])
	}
	root := map[string]interface{}{}
	if document.Format == model.ManagedDocumentFormatJSON {
		err = json.Unmarshal(raw, &root)
	} else {
		err = yaml.Unmarshal(raw, &root)
	}
	if err != nil {
		return managedDocumentInfo{}, err
	}
	value := interface{}(root)
	for _, segment := range strings.Split(strings.TrimPrefix(document.SummaryPath, "/"), "/") {
		object, ok := value.(map[string]interface{})
		if !ok {
			value = nil
			break
		}
		value = object[segment]
	}
	if object, ok := value.(map[string]interface{}); ok {
		for key := range object {
			info.Keys = append(info.Keys, key)
		}
		sort.Strings(info.Keys)
	}
	return info, nil
}

func (h *ApplicationHandler) applyManagedDocumentPatch(ctx context.Context, app *model.Application, document *model.ManagedDocument, operations []managedDocumentPatchOp) error {
	if K8s == nil || K8s.Clientset == nil {
		return fmt.Errorf("Kubernetes 集群未连接")
	}
	paths, err := document.Paths()
	if err != nil {
		return err
	}
	var raw []byte
	var write func([]byte) error
	if document.ResourceKind == model.ManagedDocumentResourceSecret {
		secret, getErr := K8s.Clientset.CoreV1().Secrets(app.Environment.Namespace).Get(ctx, document.ResourceName, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("读取受控 Secret 失败")
		}
		value, exists := secret.Data[document.Key]
		if !exists {
			return fmt.Errorf("受控 Secret 键不存在")
		}
		raw = value
		write = func(content []byte) error {
			secret.Data[document.Key] = content
			_, updateErr := K8s.Clientset.CoreV1().Secrets(app.Environment.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
			return updateErr
		}
	} else {
		configMap, getErr := K8s.Clientset.CoreV1().ConfigMaps(app.Environment.Namespace).Get(ctx, document.ResourceName, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("读取受控 ConfigMap 失败")
		}
		value, exists := configMap.Data[document.Key]
		if !exists {
			return fmt.Errorf("受控 ConfigMap 键不存在")
		}
		raw = []byte(value)
		write = func(content []byte) error {
			configMap.Data[document.Key] = string(content)
			_, updateErr := K8s.Clientset.CoreV1().ConfigMaps(app.Environment.Namespace).Update(ctx, configMap, metav1.UpdateOptions{})
			return updateErr
		}
	}
	root := map[string]interface{}{}
	if document.Format == model.ManagedDocumentFormatJSON {
		err = json.Unmarshal(raw, &root)
	} else {
		err = yaml.Unmarshal(raw, &root)
	}
	if err != nil || root == nil {
		return fmt.Errorf("受控文档格式无效")
	}
	for _, operation := range operations {
		if !pathsPermit(*document, operation.Path) || !pathsPermitRaw(paths, operation.Path) {
			return fmt.Errorf("patch 路径不在允许范围内")
		}
		if err := applyObjectPatch(root, operation); err != nil {
			return err
		}
	}
	if document.Format == model.ManagedDocumentFormatJSON {
		raw, err = json.MarshalIndent(root, "", "  ")
	} else {
		raw, err = yaml.Marshal(root)
	}
	if err != nil {
		return fmt.Errorf("编码受控文档失败")
	}
	if err := write(raw); err != nil {
		return fmt.Errorf("更新受控文档失败")
	}
	return nil
}

func pathsPermit(document model.ManagedDocument, pointer string) bool {
	paths, err := document.Paths()
	return err == nil && pathsPermitRaw(paths, pointer)
}

func pathsPermitRaw(paths []string, pointer string) bool {
	if !validPatchPointer(pointer) {
		return false
	}
	for _, allowed := range paths {
		if pointer == allowed || strings.HasPrefix(pointer, allowed+"/") {
			return true
		}
	}
	return false
}

func validPatchPointer(pointer string) bool {
	return strings.HasPrefix(pointer, "/") && pointer != "/" && !strings.Contains(pointer, "//") && !strings.Contains(pointer, "..") && !strings.Contains(pointer, "~")
}

func applyObjectPatch(root map[string]interface{}, operation managedDocumentPatchOp) error {
	if operation.Op != "add" && operation.Op != "replace" && operation.Op != "remove" {
		return fmt.Errorf("不支持的 patch 操作")
	}
	segments := strings.Split(strings.TrimPrefix(operation.Path, "/"), "/")
	current := root
	for _, segment := range segments[:len(segments)-1] {
		next, exists := current[segment]
		if !exists {
			if operation.Op == "add" {
				child := map[string]interface{}{}
				current[segment] = child
				current = child
				continue
			}
			return fmt.Errorf("patch 父路径不存在")
		}
		child, ok := next.(map[string]interface{})
		if !ok {
			return fmt.Errorf("patch 父路径不是对象")
		}
		current = child
	}
	leaf := segments[len(segments)-1]
	_, exists := current[leaf]
	switch operation.Op {
	case "add":
		if exists {
			return fmt.Errorf("patch 键已存在")
		}
		current[leaf] = operation.Value
	case "replace":
		if !exists {
			return fmt.Errorf("patch 键不存在")
		}
		current[leaf] = operation.Value
	case "remove":
		if !exists {
			return fmt.Errorf("patch 键不存在")
		}
		delete(current, leaf)
	}
	return nil
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
