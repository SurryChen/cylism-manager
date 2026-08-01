package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/application"
	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type applicationStackTemplateRequest struct {
	EnvironmentID uint                  `json:"environment_id"`
	Name          string                `json:"name"`
	Description   string                `json:"description"`
	Enabled       bool                  `json:"enabled"`
	Spec          application.StackSpec `json:"spec"`
}

type applicationStackTemplateInfo struct {
	ID            uint                  `json:"id"`
	EnvironmentID uint                  `json:"environment_id"`
	Name          string                `json:"name"`
	Description   string                `json:"description"`
	Enabled       bool                  `json:"enabled"`
	Revision      uint                  `json:"revision"`
	Spec          application.StackSpec `json:"spec"`
	UpdatedAt     time.Time             `json:"updated_at"`
}

type applicationStackReleaseRequest struct {
	Versions map[string]string `json:"versions"`
}

type stackComponentRun struct {
	ApplicationID   uint   `json:"application_id,omitempty"`
	ApplicationName string `json:"application_name"`
	ReleaseID       uint   `json:"release_id,omitempty"`
	Version         string `json:"version,omitempty"`
	Status          string `json:"status"`
	Detail          string `json:"detail,omitempty"`
}

type karakeepStackPresetRequest struct {
	EnvironmentID  uint   `json:"environment_id"`
	Name           string `json:"name"`
	DataPVC        string `json:"data_pvc"`
	MeilisearchPVC string `json:"meilisearch_pvc"`
	NextAuthSecret string `json:"nextauth_secret"`
	MeiliMasterKey string `json:"meili_master_key"`
}

func (h *ApplicationHandler) ListStackTemplates(c *gin.Context) {
	environment, ok := h.stackEnvironment(c.Query("environment_id"), c)
	if !ok {
		return
	}
	templates, err := h.store.ListApplicationStackTemplates(environment.ID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取应用栈失败")
		return
	}
	result := make([]applicationStackTemplateInfo, 0, len(templates))
	for index := range templates {
		info, err := h.stackTemplateInfo(&templates[index])
		if err != nil {
			model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取应用栈定义失败")
			return
		}
		result = append(result, *info)
	}
	model.Success(c, result)
}

func (h *ApplicationHandler) CreateStackTemplate(c *gin.Context) {
	var req applicationStackTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用栈定义无效")
		return
	}
	environment, ok := h.stackEnvironmentID(req.EnvironmentID, c)
	if !ok {
		return
	}
	template, err := h.buildStackTemplate(environment, &req, 0, "")
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	template.UpdatedBy = getUserID(c)
	if err := h.store.CreateApplicationStackTemplate(template); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "应用栈名称已存在")
		return
	}
	info, _ := h.stackTemplateInfo(template)
	model.Success(c, info)
}

func (h *ApplicationHandler) GetStackTemplate(c *gin.Context) {
	template, ok := h.stackTemplateByParam(c)
	if !ok {
		return
	}
	info, err := h.stackTemplateInfo(template)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取应用栈定义失败")
		return
	}
	model.Success(c, info)
}

func (h *ApplicationHandler) UpdateStackTemplate(c *gin.Context) {
	current, ok := h.stackTemplateByParam(c)
	if !ok {
		return
	}
	var req applicationStackTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用栈定义无效")
		return
	}
	environment, err := h.store.GetEnvironmentByID(current.EnvironmentID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "环境不存在")
		return
	}
	req.EnvironmentID = environment.ID
	template, err := h.buildStackTemplate(environment, &req, current.ID, current.EncryptedSecrets)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	template.UpdatedBy = getUserID(c)
	if err := h.store.UpdateApplicationStackTemplate(template); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "更新应用栈失败")
		return
	}
	info, _ := h.stackTemplateInfo(template)
	model.Success(c, info)
}

func (h *ApplicationHandler) DeleteStackTemplate(c *gin.Context) {
	template, ok := h.stackTemplateByParam(c)
	if !ok {
		return
	}
	if err := h.store.DeleteApplicationStackTemplate(template.EnvironmentID, template.ID); err != nil {
		status := http.StatusConflict
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		model.Error(c, status, model.CodeConflict, err.Error())
		return
	}
	model.Success(c, gin.H{"id": template.ID})
}

func (h *ApplicationHandler) ListStackReleases(c *gin.Context) {
	template, ok := h.stackTemplateByParam(c)
	if !ok {
		return
	}
	releases, err := h.store.ListApplicationStackReleasesByTemplate(template.ID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取应用栈发布失败")
		return
	}
	model.Success(c, releases)
}

func (h *ApplicationHandler) CreateStackRelease(c *gin.Context) {
	template, ok := h.stackTemplateByParam(c)
	if !ok {
		return
	}
	var req applicationStackReleaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用栈发布定义无效")
		return
	}
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	if !template.Enabled {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "应用栈已停用")
		return
	}
	spec, err := h.restoreStackSpec(template)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	for index := range spec.Components {
		version := strings.TrimSpace(req.Versions[spec.Components[index].ApplicationName])
		if version == "" {
			version = strings.TrimSpace(spec.Components[index].Spec.Version)
		}
		if version == "" {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "应用 "+spec.Components[index].ApplicationName+" 缺少版本号")
			return
		}
		spec.Components[index].Spec.ImageRepository = spec.Components[index].Spec.Image
		image, err := imageWithVersion(spec.Components[index].Spec.Image, version)
		if err != nil {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
			return
		}
		spec.Components[index].Spec.Version = version
		spec.Components[index].Spec.Image = image
	}
	snapshot, err := json.Marshal(sanitizeStackSpec(spec))
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存应用栈快照失败")
		return
	}
	release := &model.ApplicationStackRelease{EnvironmentID: template.EnvironmentID, TemplateID: template.ID, DesiredSpec: string(snapshot), Status: "accepted", CreatedBy: getUserID(c)}
	if err := h.store.CreateApplicationStackRelease(release); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "创建应用栈发布失败")
		return
	}
	go h.executeStackRelease(release.ID, template, spec, getUserID(c))
	model.SuccessWithMessage(c, release, "应用栈发布已创建")
}

func (h *ApplicationHandler) CreateKarakeepStackPreset(c *gin.Context) {
	var req karakeepStackPresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "Karakeep 预设无效")
		return
	}
	environment, ok := h.stackEnvironmentID(req.EnvironmentID, c)
	if !ok {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		req.Name = "Karakeep"
	}
	stack := application.StackSpec{EntryApplication: "karakeep", Components: []application.StackComponent{
		{ApplicationName: "karakeep-meilisearch", Description: "Karakeep 搜索服务", Spec: application.ReleaseSpec{Image: "getmeili/meilisearch", Version: "latest", ContainerPort: 7700, Replicas: 1, Config: map[string]string{"MEILI_NO_ANALYTICS": "true"}, Secrets: map[string]string{"MEILI_MASTER_KEY": req.MeiliMasterKey}, Volumes: []application.VolumeMountSpec{{ClaimName: req.MeilisearchPVC, MountPath: "/meili_data"}}, Resources: application.ResourceSpec{RequestsCPU: "100m", RequestsMemory: "128Mi", LimitsCPU: "500m", LimitsMemory: "512Mi"}, Health: application.HealthSpec{ReadinessEnabled: true, ReadinessType: "tcp"}, Service: application.ServiceSpec{Port: 7700, TargetPort: 7700}, Endpoint: application.EndpointSpec{Exposure: application.ExposureCluster}}},
		{ApplicationName: "karakeep-chrome", Description: "Karakeep 浏览器服务", Spec: application.ReleaseSpec{Image: "gcr.io/zenika-hub/alpine-chrome", Version: "latest", ContainerPort: 9222, Replicas: 1, Args: []string{"--no-sandbox", "--disable-dev-shm-usage", "--remote-debugging-address=0.0.0.0", "--remote-debugging-port=9222"}, Resources: application.ResourceSpec{RequestsCPU: "100m", RequestsMemory: "128Mi", LimitsCPU: "500m", LimitsMemory: "512Mi"}, Service: application.ServiceSpec{Port: 9222, TargetPort: 9222}, Endpoint: application.EndpointSpec{Exposure: application.ExposureCluster}}},
		{ApplicationName: "karakeep", Description: "Karakeep 入口服务", Spec: application.ReleaseSpec{Image: "ghcr.io/karakeep-app/karakeep", Version: "latest", ContainerPort: 3000, Replicas: 1, Config: map[string]string{"DATA_DIR": "/data", "MEILI_ADDR": "http://karakeep-meilisearch:7700", "BROWSER_WEB_URL": "http://karakeep-chrome:9222"}, Secrets: map[string]string{"NEXTAUTH_SECRET": req.NextAuthSecret}, Volumes: []application.VolumeMountSpec{{ClaimName: req.DataPVC, MountPath: "/data"}}, Resources: application.ResourceSpec{RequestsCPU: "200m", RequestsMemory: "256Mi", LimitsCPU: "1000m", LimitsMemory: "1Gi"}, Service: application.ServiceSpec{Port: 3000, TargetPort: 3000}, Endpoint: application.EndpointSpec{Exposure: application.ExposureCluster}}},
	}}
	template, err := h.buildStackTemplate(environment, &applicationStackTemplateRequest{EnvironmentID: environment.ID, Name: req.Name, Description: "Karakeep 应用栈", Enabled: true, Spec: stack}, 0, "")
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	template.UpdatedBy = getUserID(c)
	if err := h.store.CreateApplicationStackTemplate(template); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "应用栈名称已存在")
		return
	}
	info, _ := h.stackTemplateInfo(template)
	model.Success(c, info)
}

func (h *ApplicationHandler) executeStackRelease(releaseID uint, template *model.ApplicationStackTemplate, spec application.StackSpec, userID uint) {
	release, err := h.store.GetApplicationStackRelease(template.EnvironmentID, releaseID)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	release.Status, release.StartedAt = "applying", &now
	_ = h.store.UpdateApplicationStackRelease(release)
	environment, err := h.store.GetEnvironmentByID(template.EnvironmentID)
	if err != nil {
		h.failStackRelease(release, nil, err)
		return
	}
	service := application.NewService(h.store, application.NewKubernetesApplier(K8s))
	runs := make([]stackComponentRun, 0, len(spec.Components))
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	for _, component := range spec.Components {
		run, app, componentTemplate, resolvedSpec, err := h.prepareStackComponent(template, environment, component, userID)
		if err != nil {
			run.Status, run.Detail = "failed", err.Error()
			runs = append(runs, run)
			h.failStackRelease(release, runs, err)
			return
		}
		componentRelease, err := service.CreateRelease(ctx, app.ID, userID, resolvedSpec)
		if err != nil {
			run.Status, run.Detail = "failed", err.Error()
			runs = append(runs, run)
			h.failStackRelease(release, runs, err)
			return
		}
		componentRelease.TemplateID = &componentTemplate.ID
		componentRelease.TemplateRevision = componentTemplate.Revision
		_ = h.store.UpdateRelease(componentRelease)
		run.ReleaseID, run.Status = componentRelease.ID, "applying"
		runs = append(runs, run)
		h.updateStackRuns(release, runs)
		if err := service.ExecuteRelease(ctx, componentRelease.ID, app, resolvedSpec); err != nil {
			runs[len(runs)-1].Status, runs[len(runs)-1].Detail = "failed", err.Error()
			h.failStackRelease(release, runs, err)
			return
		}
		runs[len(runs)-1].Status = "succeeded"
		h.updateStackRuns(release, runs)
	}
	release.Status = "succeeded"
	release.Detail = "所有应用组件已发布成功"
	completedAt := time.Now().UTC()
	release.CompletedAt = &completedAt
	h.updateStackRuns(release, runs)
}

func (h *ApplicationHandler) prepareStackComponent(template *model.ApplicationStackTemplate, environment *model.Environment, component application.StackComponent, userID uint) (stackComponentRun, *model.Application, *model.ApplicationDeploymentTemplate, application.ReleaseSpec, error) {
	run := stackComponentRun{ApplicationName: component.ApplicationName, Version: component.Spec.Version, Status: "pending"}
	app, err := h.store.GetApplicationByEnvironmentName(environment.ID, component.ApplicationName)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		stackID := template.ID
		app = &model.Application{ProjectID: environment.ProjectID, EnvironmentID: environment.ID, Name: component.ApplicationName, WorkloadKind: "deployment", StackTemplateID: &stackID, CreatedBy: userID}
		if err := h.store.CreateApplication(app); err != nil {
			return run, nil, nil, application.ReleaseSpec{}, fmt.Errorf("创建应用 %q: %w", component.ApplicationName, err)
		}
		app, err = h.store.GetApplication(app.ID)
	}
	if err != nil {
		return run, nil, nil, application.ReleaseSpec{}, fmt.Errorf("读取应用 %q: %w", component.ApplicationName, err)
	}
	if app.StackTemplateID == nil || *app.StackTemplateID != template.ID {
		return run, nil, nil, application.ReleaseSpec{}, fmt.Errorf("应用 %q 已由独立应用或其他栈管理", component.ApplicationName)
	}
	if err := h.prepareRegistryReleaseSpec(app, &component.Spec); err != nil {
		return run, nil, nil, application.ReleaseSpec{}, err
	}
	if err := h.applyApplicationEndpointSpec(app, &component.Spec); err != nil {
		return run, nil, nil, application.ReleaseSpec{}, err
	}
	componentTemplate, err := h.saveStackComponentTemplate(template, app, component, userID)
	if err != nil {
		return run, nil, nil, application.ReleaseSpec{}, err
	}
	run.ApplicationID = app.ID
	return run, app, componentTemplate, component.Spec, nil
}

func (h *ApplicationHandler) saveStackComponentTemplate(stack *model.ApplicationStackTemplate, app *model.Application, component application.StackComponent, userID uint) (*model.ApplicationDeploymentTemplate, error) {
	secrets, err := json.Marshal(component.Spec.Secrets)
	if err != nil {
		return nil, err
	}
	encrypted := ""
	if len(component.Spec.Secrets) > 0 {
		if len(h.encKey) == 0 {
			return nil, fmt.Errorf("平台加密密钥未配置，无法保存应用栈 Secret")
		}
		encrypted, err = crypto.Encrypt(h.encKey, string(secrets))
		if err != nil {
			return nil, err
		}
	}
	templateSpec := component.Spec
	templateSpec.Image = templateSpec.ImageRepository
	templateSpec.ImageRepository = ""
	templateSpec.Version = ""
	templateSpec.RegistryEndpoint = ""
	templateSpec.RegistryAuthType = ""
	templateSpec.RegistryUsername = ""
	templateSpec.RegistryCredential = ""
	snapshot, err := json.Marshal(application.SanitizeReleaseSpec(templateSpec))
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("栈 %d", stack.ID)
	templates, err := h.store.ListApplicationDeploymentTemplates(app.ID)
	if err != nil {
		return nil, err
	}
	for index := range templates {
		if templates[index].Name != name {
			continue
		}
		templates[index].Description = "由应用栈管理：" + stack.Name
		templates[index].Spec = string(snapshot)
		templates[index].EncryptedSecrets = encrypted
		templates[index].Enabled = true
		templates[index].UpdatedBy = userID
		if err := h.store.UpdateApplicationDeploymentTemplate(&templates[index]); err != nil {
			return nil, err
		}
		return &templates[index], nil
	}
	template := &model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: name, Description: "由应用栈管理：" + stack.Name, Enabled: true, Spec: string(snapshot), EncryptedSecrets: encrypted, UpdatedBy: userID}
	if err := h.store.CreateApplicationDeploymentTemplate(template, app.DefaultDeploymentTemplateID == nil); err != nil {
		return nil, err
	}
	return template, nil
}

func (h *ApplicationHandler) buildStackTemplate(environment *model.Environment, req *applicationStackTemplateRequest, id uint, previousEncrypted string) (*model.ApplicationStackTemplate, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("应用栈名称必填")
	}
	if issues := application.ValidateStackSpec(req.Spec); len(issues) > 0 {
		return nil, fmt.Errorf("%s", issues[0].Message)
	}
	secrets, err := h.decryptStackSecrets(previousEncrypted)
	if err != nil {
		return nil, err
	}
	for index := range req.Spec.Components {
		component := &req.Spec.Components[index]
		stored := secrets[component.ApplicationName]
		if stored == nil {
			stored = map[string]string{}
		}
		for key, value := range component.Spec.Secrets {
			if strings.TrimSpace(value) != "" {
				stored[key] = value
			}
		}
		if len(stored) > 0 {
			secrets[component.ApplicationName] = stored
		}
	}
	if len(secrets) > 0 && len(h.encKey) == 0 {
		return nil, fmt.Errorf("平台加密密钥未配置，无法保存应用栈 Secret")
	}
	req.Spec = sanitizeStackSpec(req.Spec)
	secretJSON, err := json.Marshal(secrets)
	if err != nil {
		return nil, err
	}
	encrypted := ""
	if len(secrets) > 0 {
		encrypted, err = crypto.Encrypt(h.encKey, string(secretJSON))
		if err != nil {
			return nil, err
		}
	}
	snapshot, err := json.Marshal(req.Spec)
	if err != nil {
		return nil, err
	}
	return &model.ApplicationStackTemplate{ID: id, EnvironmentID: environment.ID, Name: strings.TrimSpace(req.Name), Description: strings.TrimSpace(req.Description), Enabled: req.Enabled, Spec: string(snapshot), EncryptedSecrets: encrypted}, nil
}

func (h *ApplicationHandler) restoreStackSpec(template *model.ApplicationStackTemplate) (application.StackSpec, error) {
	var spec application.StackSpec
	if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
		return spec, err
	}
	secrets, err := h.decryptStackSecrets(template.EncryptedSecrets)
	if err != nil {
		return spec, err
	}
	for index := range spec.Components {
		storedKeys := len(spec.Components[index].Spec.Secrets)
		spec.Components[index].Spec.Secrets = secrets[spec.Components[index].ApplicationName]
		if storedKeys > 0 && len(spec.Components[index].Spec.Secrets) == 0 {
			return spec, fmt.Errorf("应用 %q 的 Secret 尚未配置明文值", spec.Components[index].ApplicationName)
		}
	}
	return spec, nil
}

func (h *ApplicationHandler) stackTemplateInfo(template *model.ApplicationStackTemplate) (*applicationStackTemplateInfo, error) {
	var spec application.StackSpec
	if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
		return nil, err
	}
	return &applicationStackTemplateInfo{ID: template.ID, EnvironmentID: template.EnvironmentID, Name: template.Name, Description: template.Description, Enabled: template.Enabled, Revision: template.Revision, Spec: spec, UpdatedAt: template.UpdatedAt}, nil
}

func (h *ApplicationHandler) stackEnvironment(value string, c *gin.Context) (*model.Environment, bool) {
	id, err := parseID(value)
	if err != nil || id == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "环境 ID 无效")
		return nil, false
	}
	return h.stackEnvironmentID(id, c)
}

func (h *ApplicationHandler) stackEnvironmentID(id uint, c *gin.Context) (*model.Environment, bool) {
	environment, err := h.store.GetEnvironmentByID(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "环境不存在")
		return nil, false
	}
	return environment, true
}

func (h *ApplicationHandler) stackTemplateByParam(c *gin.Context) (*model.ApplicationStackTemplate, bool) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用栈 ID 无效")
		return nil, false
	}
	template, err := h.store.GetApplicationStackTemplateByID(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "应用栈不存在")
		return nil, false
	}
	return template, true
}

func (h *ApplicationHandler) decryptStackSecrets(encrypted string) (map[string]map[string]string, error) {
	result := make(map[string]map[string]string)
	if strings.TrimSpace(encrypted) == "" {
		return result, nil
	}
	if len(h.encKey) == 0 {
		return nil, fmt.Errorf("平台加密密钥未配置")
	}
	plaintext, err := crypto.Decrypt(h.encKey, encrypted)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(plaintext), &result); err != nil {
		return nil, err
	}
	return result, nil
}

func sanitizeStackSpec(spec application.StackSpec) application.StackSpec {
	for index := range spec.Components {
		spec.Components[index].Spec = application.SanitizeReleaseSpec(spec.Components[index].Spec)
	}
	return spec
}

func (h *ApplicationHandler) updateStackRuns(release *model.ApplicationStackRelease, runs []stackComponentRun) {
	value, _ := json.Marshal(runs)
	release.ComponentRuns = string(value)
	_ = h.store.UpdateApplicationStackRelease(release)
}

func (h *ApplicationHandler) failStackRelease(release *model.ApplicationStackRelease, runs []stackComponentRun, err error) {
	release.Status, release.Detail = "failed", err.Error()
	completedAt := time.Now().UTC()
	release.CompletedAt = &completedAt
	h.updateStackRuns(release, runs)
}
