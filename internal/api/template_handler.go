package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/application"
	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type deploymentTemplateInfo struct {
	ID            uint                    `json:"id"`
	ApplicationID uint                    `json:"application_id"`
	Name          string                  `json:"name"`
	Description   string                  `json:"description"`
	Enabled       bool                    `json:"enabled"`
	IsDefault     bool                    `json:"is_default"`
	Revision      uint                    `json:"revision"`
	Spec          application.ReleaseSpec `json:"spec"`
	UpdatedAt     time.Time               `json:"updated_at"`
}

type deploymentTemplateRequest struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Enabled     bool                    `json:"enabled"`
	Revision    uint                    `json:"revision,omitempty"`
	Spec        application.ReleaseSpec `json:"spec"`
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
	model.Success(c, infos)
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
	model.Success(c, info)
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
	template, err := h.templateFromRequest(app, &req, 0)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	template.UpdatedBy = apiShared.UserID(c)
	if err := h.applications.CreateApplicationDeploymentTemplate(template, app.DefaultDeploymentTemplateID == nil); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if app.DefaultDeploymentTemplateID == nil {
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
	model.Success(c, info)
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
	template, err := h.templateFromRequest(app, &req, templateID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	template.UpdatedBy = apiShared.UserID(c)
	var updateErr error
	if req.Revision != 0 {
		updateErr = h.applications.UpdateApplicationDeploymentTemplateIfRevision(template, req.Revision)
	} else {
		updateErr = h.applications.UpdateApplicationDeploymentTemplate(template)
	}
	if err := updateErr; err != nil {
		var conflict *model.TemplateRevisionConflictError
		if errors.As(err, &conflict) {
			apiShared.Conflict(c, conflict.Error())
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apiShared.NotFound(c, "上线模板不存在")
			return
		}
		apiShared.DBError(c, err.Error())
		return
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
	model.Success(c, info)
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
	model.Success(c, gin.H{"id": templateID})
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
	var spec application.ReleaseSpec
	if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
		apiShared.DBError(c, "读取上线模板失败")
		return
	}
	if err := h.releaseWorkflow().SyncManagedFiles(app, spec, apiShared.UserID(c)); err != nil {
		apiShared.DBError(c, "登记模板 ConfigMap 配置失败")
		return
	}
	model.Success(c, gin.H{"id": templateID})
}

func (h *ApplicationHandler) templateFromRequest(app *model.Application, req *deploymentTemplateRequest, templateID uint) (*model.ApplicationDeploymentTemplate, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("模板名称必填")
	}
	if len([]rune(req.Name)) > 128 {
		return nil, fmt.Errorf("模板名称不能超过 128 个字符")
	}
	spec := req.Spec
	application.NormalizeManagedKeys(&spec)
	if strings.TrimSpace(spec.Version) != "" {
		return nil, fmt.Errorf("上线模板不应包含版本号")
	}
	if err := validateImageRepository(spec.Image); err != nil {
		return nil, err
	}
	if err := h.releaseWorkflow().PrepareRegistrySpec(app, &spec); err != nil {
		return nil, err
	}
	encryptedSecrets := ""
	if templateID != 0 {
		current, err := h.applications.GetApplicationDeploymentTemplate(app.ID, templateID)
		if err != nil {
			return nil, fmt.Errorf("读取旧模板 Secret: %w", err)
		}
		encryptedSecrets = current.EncryptedSecrets
	}
	if len(spec.Secrets) > 0 {
		currentSecrets, err := h.decryptTemplateSecretsByValue(encryptedSecrets)
		if err != nil {
			return nil, fmt.Errorf("读取旧模板 Secret: %w", err)
		}
		for key, value := range spec.Secrets {
			if strings.TrimSpace(value) != "" {
				currentSecrets[key] = value
			}
		}
		if len(currentSecrets) == 0 {
			return nil, fmt.Errorf("Secret 必须至少配置一个非空值")
		}
		payload, err := json.Marshal(currentSecrets)
		if err != nil {
			return nil, fmt.Errorf("编码模板 Secret: %w", err)
		}
		if len(h.encKey) == 0 {
			return nil, fmt.Errorf("平台加密密钥未配置，无法保存 Secret")
		}
		encryptedSecrets, err = crypto.Encrypt(h.encKey, string(payload))
		if err != nil {
			return nil, fmt.Errorf("加密模板 Secret: %w", err)
		}
	}
	// Domain binding belongs to the application endpoint, never to a rollout template.
	spec.Endpoint = application.EndpointSpec{Exposure: application.ExposureCluster}
	if issues := application.ValidateReleaseSpec(spec); len(issues) > 0 {
		return nil, fmt.Errorf("%s", issues[0].Message)
	}
	if len(spec.Volumes) > 0 {
		if K8s == nil {
			return nil, fmt.Errorf("K8s 集群未连接，无法验证 PVC")
		}
		applicationContext := application.ApplicationContext{EnvironmentID: app.Environment.ID, Namespace: app.Environment.Namespace}
		if err := application.NewKubernetesApplier(K8s).ValidatePersistentVolumeClaims(context.Background(), applicationContext, spec); err != nil {
			return nil, err
		}
	}
	snapshot, err := json.Marshal(application.SanitizeReleaseSpec(spec))
	if err != nil {
		return nil, fmt.Errorf("保存上线模板失败: %w", err)
	}
	return &model.ApplicationDeploymentTemplate{ID: templateID, ApplicationID: app.ID, Name: strings.TrimSpace(req.Name), Description: strings.TrimSpace(req.Description), Enabled: req.Enabled, Spec: string(snapshot), EncryptedSecrets: encryptedSecrets}, nil
}

func (h *ApplicationHandler) decryptTemplateSecrets(template *model.ApplicationDeploymentTemplate) (map[string]string, error) {
	return h.decryptTemplateSecretsByValue(template.EncryptedSecrets)
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
	var spec application.ReleaseSpec
	if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
		return nil, err
	}
	return &deploymentTemplateInfo{ID: template.ID, ApplicationID: template.ApplicationID, Name: template.Name, Description: template.Description, Enabled: template.Enabled, IsDefault: defaultTemplateID != nil && *defaultTemplateID == template.ID, Revision: template.Revision, Spec: spec, UpdatedAt: template.UpdatedAt}, nil
}
