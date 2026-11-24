package applicationapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	crypto "github.com/cylism/cylism-manager/internal/security"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	auditservice "github.com/cylism/cylism-manager/internal/service/audit"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type deploymentTemplateInfo struct {
	ID             uint                           `json:"id"`
	ApplicationID  uint                           `json:"application_id"`
	Name           string                         `json:"name"`
	Description    string                         `json:"description"`
	Enabled        bool                           `json:"enabled"`
	DefaultVersion string                         `json:"default_version"`
	IsDefault      bool                           `json:"is_default"`
	Revision       uint                           `json:"revision"`
	Spec           applicationservice.ReleaseSpec `json:"spec"`
	UpdatedAt      time.Time                      `json:"updated_at"`
}

type deploymentTemplateRequest struct {
	Name              string                         `json:"name"`
	Description       string                         `json:"description"`
	Enabled           bool                           `json:"enabled"`
	DefaultVersion    string                         `json:"default_version"`
	Revision          uint                           `json:"revision,omitempty"`
	Spec              applicationservice.ReleaseSpec `json:"spec"`
	SecretFileEdits   []templateSecretFileEdit       `json:"secret_file_edits,omitempty"`
	DeleteSecretKeys  []string                       `json:"delete_secret_keys,omitempty"`
	deletedSecretKeys []string
}

func (h *ApplicationHandler) ListDeploymentTemplates(c *gin.Context) {
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
	templates, err := h.applications.ListApplicationDeploymentTemplates(applicationID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	infos := make([]deploymentTemplateInfo, 0, len(templates))
	for index := range templates {
		info, err := deploymentTemplateFromModel(&templates[index], app.DefaultDeploymentTemplateID)
		if err != nil {
			apiShared.DBError(c, "读取应用上线模板失败")
			return
		}
		infos = append(infos, *info)
	}
	apiShared.Success(c, infos)
}

func (h *ApplicationHandler) GetDeploymentTemplate(c *gin.Context) {
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	templateID, err := apiShared.ParseID(c.Param("templateID"))
	if err != nil {
		apiShared.BadRequest(c, "上线模板 ID 无效")
		return
	}
	app, err := h.queries.GetApplication(applicationID)
	if err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	template, err := h.applications.GetApplicationDeploymentTemplate(applicationID, templateID)
	if err != nil {
		apiShared.NotFound(c, "上线模板不存在")
		return
	}
	info, err := deploymentTemplateFromModel(template, app.DefaultDeploymentTemplateID)
	if err != nil {
		apiShared.DBError(c, "读取应用上线模板失败")
		return
	}
	if c.Query("include_unused_secret_keys") == "true" {
		if !canManageTemplateSecrets(c) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "message": "请使用平台管理账号查看无引用的敏感配置"})
			return
		}
		c.Header("Cache-Control", "no-store")
		values, err := h.decryptTemplateSecrets(template)
		if err != nil {
			apiShared.DBError(c, "无法读取敏感配置清单")
			return
		}
		unusedKeys := make([]string, 0)
		for key := range values {
			if !applicationservice.TemplateSecretReferenced(info.Spec, app.Name, key) {
				unusedKeys = append(unusedKeys, key)
			}
		}
		slices.Sort(unusedKeys)
		apiShared.Success(c, gin.H{"revision": template.Revision, "unused_secret_keys": unusedKeys})
		return
	}
	apiShared.Success(c, info)
}

func canManageTemplateSecrets(c *gin.Context) bool {
	_, delegated := c.Get("delegation")
	return apiShared.UserID(c) != 0 && !delegated
}

func (h *ApplicationHandler) CreateDeploymentTemplate(c *gin.Context) {
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
	var req deploymentTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "上线模板定义无效")
		return
	}
	if len(req.SecretFileEdits) > 0 || len(req.DeleteSecretKeys) > 0 {
		apiShared.ValidationError(c, "新模板请直接填写文件内容")
		return
	}
	template, err := h.templateFromRequest(c.Request.Context(), app, &req, nil)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	template.UpdatedBy = apiShared.UserID(c)
	makeDefault := app.DefaultDeploymentTemplateID == nil && template.Enabled
	if err := h.applications.CreateApplicationDeploymentTemplate(template, makeDefault); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if makeDefault {
		app.DefaultDeploymentTemplateID = &template.ID
	}
	if app.DefaultDeploymentTemplateID != nil && *app.DefaultDeploymentTemplateID == template.ID {
		if err := h.releaseWorkflow().SyncManagedFiles(app, req.Spec, apiShared.UserID(c)); err != nil {
			apiShared.DBError(c, "登记模板 ConfigMap 配置失败")
			return
		}
	}
	info, err := deploymentTemplateFromModel(template, app.DefaultDeploymentTemplateID)
	if err != nil {
		apiShared.DBError(c, "读取上线模板失败")
		return
	}
	apiShared.Success(c, info)
}

func (h *ApplicationHandler) UpdateDeploymentTemplate(c *gin.Context) {
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	templateID, err := apiShared.ParseID(c.Param("templateID"))
	if err != nil {
		apiShared.BadRequest(c, "上线模板 ID 无效")
		return
	}
	app, err := h.queries.GetApplication(applicationID)
	if err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	var req deploymentTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "上线模板定义无效")
		return
	}
	if app.DefaultDeploymentTemplateID != nil && *app.DefaultDeploymentTemplateID == templateID && !req.Enabled {
		apiShared.ValidationError(c, "默认模板不能停用，请先设置其他启用模板为默认")
		return
	}
	currentTemplate, err := h.applications.GetApplicationDeploymentTemplate(applicationID, templateID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "上线模板不存在")
		return
	}
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if req.Revision != 0 && currentTemplate.Revision != req.Revision {
		apiShared.Conflict(c, fmt.Sprintf("模板版本冲突，当前版本为 %d", currentTemplate.Revision))
		return
	}
	if len(req.DeleteSecretKeys) > 0 && req.Revision == 0 {
		apiShared.Conflict(c, "删除敏感配置需要当前模板版本，请重新打开模板")
		return
	}
	if err := h.applyTemplateSecretFileEdits(currentTemplate, &req); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	binding, err := h.applications.GetDatabaseBindingForTemplate(templateID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.DBError(c, err.Error())
		return
	}
	if err == nil {
		if err := validateBoundDatabaseTemplate(currentTemplate, req.Spec, binding.Prefix); err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
	}
	template, err := h.templateFromRequest(c.Request.Context(), app, &req, currentTemplate)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	template.UpdatedBy = apiShared.UserID(c)
	files := applicationservice.TemplateManagedFiles(app, req.Spec, apiShared.UserID(c))
	var event *auditservice.AuditEventInput
	if len(req.deletedSecretKeys) > 0 {
		if !canManageTemplateSecrets(c) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "message": "请使用平台管理账号删除模板敏感配置"})
			return
		}
		event = &auditservice.AuditEventInput{
			Action: "application.template.delete-sensitive-keys", ResourceType: "application", ResourceID: applicationID,
			Actor: apiShared.ActorFromContext(c), Source: model.AuditSourceAPI, Outcome: model.AuditOutcomeSucceeded,
			Summary: "删除上线模板中的敏感配置", RequestID: apiShared.RequestID(c),
			Metadata: map[string]any{"template_id": templateID, "deleted_keys": req.deletedSecretKeys},
		}
	}
	if err := h.applications.SaveDeploymentTemplateEdit(template, currentTemplate.Revision, files, event); err != nil {
		var conflict *model.TemplateRevisionConflictError
		if errors.As(err, &conflict) {
			apiShared.Conflict(c, conflict.Error())
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apiShared.NotFound(c, "上线模板不存在")
			return
		}
		apiShared.DBError(c, "保存上线模板失败")
		return
	}
	info, err := deploymentTemplateFromModel(template, app.DefaultDeploymentTemplateID)
	if err != nil {
		apiShared.DBError(c, "读取上线模板失败")
		return
	}
	apiShared.Success(c, info)
}

func validateBoundDatabaseTemplate(current *model.ApplicationDeploymentTemplate, next applicationservice.ReleaseSpec, prefix string) error {
	var previous applicationservice.ReleaseSpec
	if err := json.Unmarshal([]byte(current.Spec), &previous); err != nil {
		return fmt.Errorf("读取已绑定的数据库配置失败: %w", err)
	}
	for _, suffix := range []string{"HOST", "PORT", "NAME", "USER"} {
		key := prefix + "_" + suffix
		if next.Config[key] != previous.Config[key] || containsString(next.ConfigDisabled, key) || containsString(next.ConfigFileKeys, key) {
			return fmt.Errorf("%s 由数据库绑定管理，请先解除绑定", key)
		}
	}
	key := prefix + "_PASSWORD"
	value, exists := next.Secrets[key]
	if !exists || value != "" || containsString(next.SecretsDisabled, key) || containsString(next.SecretFileKeys, key) {
		return fmt.Errorf("%s 由数据库绑定管理，请先解除绑定", key)
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (h *ApplicationHandler) DeleteDeploymentTemplate(c *gin.Context) {
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	templateID, err := apiShared.ParseID(c.Param("templateID"))
	if err != nil {
		apiShared.BadRequest(c, "上线模板 ID 无效")
		return
	}
	if err := h.applications.DeleteApplicationDeploymentTemplate(applicationID, templateID); err != nil {
		if strings.Contains(err.Error(), "关联发布") {
			apiShared.Conflict(c, err.Error())
			return
		}
		apiShared.NotFound(c, "上线模板不存在")
		return
	}
	apiShared.Success(c, gin.H{"id": templateID})
}

func (h *ApplicationHandler) SetDefaultDeploymentTemplate(c *gin.Context) {
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	templateID, err := apiShared.ParseID(c.Param("templateID"))
	if err != nil {
		apiShared.BadRequest(c, "上线模板 ID 无效")
		return
	}
	app, err := h.queries.GetApplication(applicationID)
	if err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	template, err := h.applications.GetApplicationDeploymentTemplate(applicationID, templateID)
	if err != nil || !template.Enabled {
		apiShared.ValidationError(c, "上线模板不存在或已停用")
		return
	}
	if err := h.applications.SetDefaultApplicationDeploymentTemplate(applicationID, templateID); err != nil {
		apiShared.ValidationError(c, "上线模板不存在或已停用")
		return
	}
	var spec applicationservice.ReleaseSpec
	if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
		apiShared.DBError(c, "读取上线模板失败")
		return
	}
	if err := h.releaseWorkflow().SyncManagedFiles(app, spec, apiShared.UserID(c)); err != nil {
		apiShared.DBError(c, "登记模板 ConfigMap 配置失败")
		return
	}
	apiShared.Success(c, gin.H{"id": templateID})
}

func (h *ApplicationHandler) templateFromRequest(ctx context.Context, app *model.Application, req *deploymentTemplateRequest, current *model.ApplicationDeploymentTemplate) (*model.ApplicationDeploymentTemplate, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("模板名称必填")
	}
	if len([]rune(req.Name)) > 128 {
		return nil, fmt.Errorf("模板名称不能超过 128 个字符")
	}
	defaultVersion := strings.TrimSpace(req.DefaultVersion)
	if defaultVersion != "" && !applicationservice.ValidReleaseVersion(defaultVersion) {
		return nil, fmt.Errorf("默认版本号必须是合法的镜像 Tag")
	}
	spec := req.Spec
	if spec.Migration != nil && spec.Migration.Target != "" {
		return nil, fmt.Errorf("迁移目标属于发布参数，不能保存在上线模板中")
	}
	applicationservice.NormalizeManagedKeys(&spec)
	if strings.TrimSpace(spec.Version) != "" {
		return nil, fmt.Errorf("上线模板不应包含版本号")
	}
	if err := validateImageRepository(spec.Image); err != nil {
		return nil, err
	}
	if err := h.releaseWorkflow().PrepareRegistrySpec(app, &spec); err != nil {
		return nil, err
	}
	encryptedSecrets, err := h.updateTemplateSecrets(app.Name, current, &spec, req)
	if err != nil {
		return nil, err
	}
	// Domain binding belongs to the application endpoint, never to a rollout template.
	spec.Endpoint = applicationservice.EndpointSpec{Exposure: applicationservice.ExposureCluster}
	if issues := applicationservice.ValidateReleaseSpec(spec); len(issues) > 0 {
		return nil, fmt.Errorf("%s", issues[0].Message)
	}
	if len(spec.Volumes) > 0 {
		if h.kubernetes == nil || !h.kubernetes.KubernetesAvailable() {
			return nil, fmt.Errorf("K8s 集群未连接，无法验证 PVC")
		}
		applicationContext := applicationservice.ApplicationContext{EnvironmentID: app.Environment.ID, Namespace: app.Environment.Namespace, ApplicationName: app.Name}
		if err := h.kubernetes.ValidatePersistentVolumeClaims(ctx, applicationContext, spec); err != nil {
			return nil, err
		}
	}
	snapshot, err := json.Marshal(applicationservice.SanitizeReleaseSpec(spec))
	if err != nil {
		return nil, fmt.Errorf("保存上线模板失败: %w", err)
	}
	req.Spec = spec
	template := &model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: strings.TrimSpace(req.Name), Description: strings.TrimSpace(req.Description), Enabled: req.Enabled, DefaultVersion: defaultVersion, Spec: string(snapshot), EncryptedSecrets: encryptedSecrets}
	if current != nil {
		template.ID = current.ID
		template.CreatedAt = current.CreatedAt
	}
	return template, nil
}

func (h *ApplicationHandler) decryptTemplateSecrets(template *model.ApplicationDeploymentTemplate) (map[string]string, error) {
	return h.decryptTemplateSecretsByValue(template.EncryptedSecrets)
}

func (h *ApplicationHandler) updateTemplateSecrets(appName string, current *model.ApplicationDeploymentTemplate, spec *applicationservice.ReleaseSpec, req *deploymentTemplateRequest) (string, error) {
	values := map[string]string{}
	if current != nil {
		var err error
		values, err = h.decryptTemplateSecrets(current)
		if err != nil {
			return "", fmt.Errorf("读取旧模板敏感配置失败")
		}
	}
	explicit := make(map[string]bool)
	for _, key := range req.DeleteSecretKeys {
		if explicit[key] || applicationservice.TemplateSecretReferenced(*spec, appName, key) {
			return "", fmt.Errorf("敏感配置 %s 仍有引用或重复选择", key)
		}
		if _, exists := values[key]; !exists {
			return "", fmt.Errorf("敏感配置 %s 已不存在，请重新打开模板", key)
		}
		explicit[key] = true
	}
	for key, value := range spec.Secrets {
		if explicit[key] {
			if value != "" {
				return "", fmt.Errorf("不能同时修改和删除敏感配置 %s", key)
			}
			delete(spec.Secrets, key)
			continue
		}
		if strings.TrimSpace(value) != "" {
			values[key] = value
		} else if strings.TrimSpace(values[key]) == "" {
			return "", fmt.Errorf("请填写敏感配置 %s 的内容", key)
		}
	}
	if current != nil {
		var previous applicationservice.ReleaseSpec
		if err := json.Unmarshal([]byte(current.Spec), &previous); err != nil {
			return "", fmt.Errorf("读取旧模板定义失败")
		}
		for key := range previous.Secrets {
			if _, kept := spec.Secrets[key]; !kept && !applicationservice.TemplateSecretReferenced(*spec, appName, key) {
				explicit[key] = true
			}
		}
	}
	if len(explicit) > 0 && req.Revision == 0 {
		return "", fmt.Errorf("删除敏感配置需要当前模板版本，请重新打开模板")
	}
	for key := range explicit {
		delete(values, key)
		req.deletedSecretKeys = append(req.deletedSecretKeys, key)
	}
	slices.Sort(req.deletedSecretKeys)
	spec.SecretFileKeys = slices.DeleteFunc(spec.SecretFileKeys, func(key string) bool { return explicit[key] })
	spec.SecretsDisabled = slices.DeleteFunc(spec.SecretsDisabled, func(key string) bool { return explicit[key] })
	spec.SecretManagedKeys = slices.DeleteFunc(spec.SecretManagedKeys, func(key string) bool { return explicit[key] })
	if len(values) == 0 {
		return "", nil
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("编码模板敏感配置失败")
	}
	return crypto.Encrypt(h.encKey, string(payload))
}

func (h *ApplicationHandler) decryptTemplateSecretsByValue(encrypted string) (map[string]string, error) {
	if strings.TrimSpace(encrypted) == "" {
		return map[string]string{}, nil
	}
	if len(h.encKey) == 0 {
		return nil, fmt.Errorf("平台加密密钥未配置")
	}
	plaintext, err := crypto.Decrypt(h.encKey, encrypted)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string)
	if err := json.Unmarshal([]byte(plaintext), &values); err != nil {
		return nil, err
	}
	return values, nil
}

func deploymentTemplateFromModel(template *model.ApplicationDeploymentTemplate, defaultTemplateID *uint) (*deploymentTemplateInfo, error) {
	var spec applicationservice.ReleaseSpec
	if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
		return nil, err
	}
	return &deploymentTemplateInfo{ID: template.ID, ApplicationID: template.ApplicationID, Name: template.Name, Description: template.Description, Enabled: template.Enabled, DefaultVersion: template.DefaultVersion, IsDefault: defaultTemplateID != nil && *defaultTemplateID == template.ID, Revision: template.Revision, Spec: spec, UpdatedAt: template.UpdatedAt}, nil
}
