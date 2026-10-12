package applicationapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type catalogItemRequest struct {
	Name           string                    `json:"name"`
	Slug           string                    `json:"slug"`
	Description    string                    `json:"description"`
	Revision       uint                      `json:"revision"`
	Enabled        bool                      `json:"enabled"`
	EntryComponent string                    `json:"entry_component"`
	Components     []catalogComponentRequest `json:"components"`
}

var catalogConfigKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type catalogComponentRequest struct {
	Name            string                         `json:"name"`
	ApplicationName string                         `json:"application_name"`
	DefaultVersion  string                         `json:"default_version"`
	Spec            applicationservice.ReleaseSpec `json:"spec"`
}

type catalogItemResponse struct {
	model.ApplicationCatalogItem
	Components []catalogComponentRequest `json:"components"`
}

type catalogFromEnvironmentRequest struct {
	ProjectID          uint   `json:"project_id"`
	EnvironmentID      uint   `json:"environment_id"`
	ApplicationIDs     []uint `json:"application_ids"`
	EntryApplicationID uint   `json:"entry_application_id"`
	Name               string `json:"name"`
	Slug               string `json:"slug"`
	Description        string `json:"description"`
}

func catalogResponse(item model.ApplicationCatalogItem) (catalogItemResponse, error) {
	stored, err := item.CatalogComponents()
	if err != nil {
		return catalogItemResponse{}, err
	}
	components := make([]catalogComponentRequest, 0, len(stored))
	for _, component := range stored {
		var spec applicationservice.ReleaseSpec
		if err := json.Unmarshal(component.Spec, &spec); err != nil {
			return catalogItemResponse{}, err
		}
		components = append(components, catalogComponentRequest{Name: component.Name, ApplicationName: component.ApplicationName, DefaultVersion: component.DefaultVersion, Spec: spec})
	}
	item.EntryComponent = item.CatalogEntryComponent()
	return catalogItemResponse{ApplicationCatalogItem: item, Components: components}, nil
}

func catalogItemFromRequest(req catalogItemRequest) (*model.ApplicationCatalogItem, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || len([]rune(name)) > 128 || len([]rune(req.Description)) > 512 {
		return nil, fmt.Errorf("名称必填且不能超过 128 字，说明不能超过 512 字")
	}
	slug := strings.TrimSpace(req.Slug)
	if err := applicationservice.ValidateApplicationName(slug); err != nil {
		return nil, fmt.Errorf("商城应用标识无效: %w", err)
	}
	if len(req.Components) == 0 && !req.Enabled {
		return &model.ApplicationCatalogItem{Name: name, Slug: slug, Description: strings.TrimSpace(req.Description), ComponentsData: "[]", LegacySpec: "{}"}, nil
	}
	if len(req.Components) < 1 || len(req.Components) > 20 {
		return nil, fmt.Errorf("商城应用需要 1 至 20 个部署组件")
	}
	seen := make(map[string]bool, len(req.Components))
	stored := make([]model.ApplicationCatalogComponent, 0, len(req.Components))
	entry := strings.TrimSpace(req.EntryComponent)
	entryFound := false
	for index, component := range req.Components {
		componentName := strings.TrimSpace(component.Name)
		appName := strings.TrimSpace(component.ApplicationName)
		if componentName == "" || len([]rune(componentName)) > 128 {
			return nil, fmt.Errorf("第 %d 个组件名称必填且不能超过 128 字", index+1)
		}
		if err := applicationservice.ValidateApplicationName(appName); err != nil {
			return nil, fmt.Errorf("组件 %s 的应用标识无效: %w", componentName, err)
		}
		if seen[appName] {
			return nil, fmt.Errorf("组件应用标识 %s 重复", appName)
		}
		seen[appName] = true
		spec := component.Spec
		defaultVersion := strings.TrimSpace(component.DefaultVersion)
		if defaultVersion != "" && !applicationservice.ValidReleaseVersion(defaultVersion) {
			return nil, fmt.Errorf("组件 %s 的默认版本号必须是合法的镜像 Tag", componentName)
		}
		if err := validateImageRepository(spec.Image); err != nil {
			return nil, fmt.Errorf("组件 %s: %w", componentName, err)
		}
		if spec.Version != "" || spec.RegistryID != 0 || spec.NodeName != "" || len(spec.Volumes) != 0 || len(spec.FileMounts) != 0 || len(spec.Secrets) != 0 || len(spec.SecretsDisabled) != 0 || len(spec.SecretManagedKeys) != 0 {
			return nil, fmt.Errorf("组件 %s 不能包含版本、仓库绑定、节点、存储卷或 Secret 配置", componentName)
		}
		for key := range spec.Config {
			if !catalogConfigKeyPattern.MatchString(key) || len(key) > 128 {
				return nil, fmt.Errorf("组件 %s 的配置项 %q 不是有效环境变量名", componentName, key)
			}
		}
		spec.Endpoint = applicationservice.EndpointSpec{Exposure: applicationservice.ExposureCluster}
		if issues := applicationservice.ValidateReleaseSpec(spec); len(issues) > 0 {
			return nil, fmt.Errorf("组件 %s: %s", componentName, issues[0].Message)
		}
		if appName == entry {
			if _, hasTCP := spec.Service.PrimaryTCPPort(); !hasTCP {
				return nil, fmt.Errorf("入口组件 %s 需要 TCP Service 端口", componentName)
			}
			entryFound = true
		}
		snapshot, err := json.Marshal(applicationservice.SanitizeReleaseSpec(spec))
		if err != nil {
			return nil, err
		}
		stored = append(stored, model.ApplicationCatalogComponent{Name: componentName, ApplicationName: appName, DefaultVersion: defaultVersion, Spec: snapshot})
	}
	if !entryFound {
		return nil, fmt.Errorf("请选择组合中的一个入口组件")
	}
	snapshot, err := json.Marshal(stored)
	if err != nil {
		return nil, err
	}
	return &model.ApplicationCatalogItem{Name: name, Slug: slug, Description: strings.TrimSpace(req.Description), Enabled: req.Enabled, EntryComponent: entry, ComponentsData: string(snapshot), LegacySpec: "{}"}, nil
}

func (h *ApplicationHandler) ListCatalogItems(c *gin.Context) {
	items, err := h.applications.ListApplicationCatalogItems()
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	result := make([]catalogItemResponse, 0, len(items))
	for _, item := range items {
		response, err := catalogResponse(item)
		if err != nil {
			apiShared.DBError(c, "读取商城模板失败")
			return
		}
		result = append(result, response)
	}
	apiShared.Success(c, result)
}

func (h *ApplicationHandler) GetCatalogItem(c *gin.Context) {
	id, err := apiShared.ParseID(c.Param("catalogID"))
	if err != nil {
		apiShared.BadRequest(c, "商城应用 ID 无效")
		return
	}
	item, err := h.applications.GetApplicationCatalogItem(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "商城应用不存在")
		return
	}
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	response, err := catalogResponse(*item)
	if err != nil {
		apiShared.DBError(c, "读取商城应用配置失败")
		return
	}
	apiShared.Success(c, response)
}

func (h *ApplicationHandler) ListCatalogInstallations(c *gin.Context) {
	projectID, projectErr := apiShared.OptionalID(strings.TrimSpace(c.Query("project_id")))
	environmentID, environmentErr := apiShared.OptionalID(strings.TrimSpace(c.Query("environment_id")))
	if projectErr != nil || environmentErr != nil || projectID == 0 || environmentID == 0 {
		apiShared.BadRequest(c, "请选择项目和环境")
		return
	}
	if _, err := h.queries.ResolveEnvironment(projectID, environmentID); err != nil {
		apiShared.NotFound(c, "目标环境不存在")
		return
	}
	installations, err := h.applications.ListApplicationCatalogInstallations(projectID, environmentID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	apiShared.Success(c, installations)
}

func (h *ApplicationHandler) CreateCatalogItem(c *gin.Context) {
	var req catalogItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "商城模板定义无效")
		return
	}
	if req.Enabled || len(req.Components) != 0 || req.EntryComponent != "" {
		apiShared.ValidationError(c, "请先创建商城应用，再配置组件并启用")
		return
	}
	item, err := catalogItemFromRequest(req)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	item.CreatedBy = apiShared.UserID(c)
	if err := h.applications.CreateApplicationCatalogItem(item); err != nil {
		apiShared.Conflict(c, "商城应用标识已存在")
		return
	}
	response, _ := catalogResponse(*item)
	apiShared.Success(c, response)
}

func (h *ApplicationHandler) CreateCatalogFromEnvironment(c *gin.Context) {
	var req catalogFromEnvironmentRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ProjectID == 0 || req.EnvironmentID == 0 || len(req.ApplicationIDs) < 2 || len(req.ApplicationIDs) > 20 || req.EntryApplicationID == 0 {
		apiShared.BadRequest(c, "请选择来源项目、环境、2 至 20 个应用和入口应用")
		return
	}
	if _, err := h.queries.ResolveEnvironment(req.ProjectID, req.EnvironmentID); err != nil {
		apiShared.NotFound(c, "来源环境不存在")
		return
	}
	applications, err := h.queries.ListApplications(req.ProjectID, req.EnvironmentID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	available := make(map[uint]model.Application, len(applications))
	for _, app := range applications {
		available[app.ID] = app
	}
	components := make([]catalogComponentRequest, 0, len(req.ApplicationIDs))
	seen := make(map[uint]bool, len(req.ApplicationIDs))
	entry := ""
	for _, id := range req.ApplicationIDs {
		app, exists := available[id]
		if id == 0 || seen[id] || !exists {
			apiShared.ValidationError(c, "所选应用不存在于来源环境或重复选择")
			return
		}
		seen[id] = true
		if app.DefaultDeploymentTemplateID == nil {
			apiShared.ValidationError(c, fmt.Sprintf("应用 %s 尚未设置默认上线模板", app.Name))
			return
		}
		template, err := h.applications.GetDefaultApplicationDeploymentTemplate(app.ID)
		if err != nil {
			apiShared.ValidationError(c, fmt.Sprintf("读取应用 %s 的默认上线模板失败", app.Name))
			return
		}
		component, err := catalogComponentFromTemplate(app, *template)
		if err != nil {
			apiShared.ValidationError(c, fmt.Sprintf("应用 %s: %v", app.Name, err))
			return
		}
		if component.DefaultVersion == "" {
			if release, err := h.applications.GetLatestSuccessfulRelease(app.ID); err == nil {
				component.DefaultVersion = release.Version
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				apiShared.DBError(c, err.Error())
				return
			}
		}
		components = append(components, component)
		if id == req.EntryApplicationID {
			entry = app.Name
		}
	}
	if entry == "" {
		apiShared.ValidationError(c, "入口应用必须是所选应用之一")
		return
	}
	item, err := catalogItemFromRequest(catalogItemRequest{Name: req.Name, Slug: req.Slug, Description: req.Description, EntryComponent: entry, Components: components})
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	item.CreatedBy = apiShared.UserID(c)
	if err := h.applications.CreateApplicationCatalogItem(item); err != nil {
		apiShared.Conflict(c, "商城应用标识已存在")
		return
	}
	response, _ := catalogResponse(*item)
	apiShared.Success(c, response)
}

func catalogComponentFromTemplate(app model.Application, template model.ApplicationDeploymentTemplate) (catalogComponentRequest, error) {
	if app.WorkloadKind != "" && app.WorkloadKind != applicationservice.WorkloadKindDeployment {
		return catalogComponentRequest{}, fmt.Errorf("当前仅支持导入 Deployment 应用")
	}
	var spec applicationservice.ReleaseSpec
	if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
		return catalogComponentRequest{}, fmt.Errorf("默认上线模板配置无效")
	}
	if len(spec.Secrets) != 0 || template.EncryptedSecrets != "" || len(spec.Volumes) != 0 || len(spec.FileMounts) != 0 || spec.NodeName != "" || spec.HostNetwork || len(spec.Command) != 0 || len(spec.Args) != 0 || len(spec.ConfigDisabled) != 0 || len(spec.ConfigManagedKeys) != 0 || spec.Health.LivenessEnabled || (spec.Health.ReadinessEnabled && spec.Health.ReadinessType != "" && spec.Health.ReadinessType != "http") {
		return catalogComponentRequest{}, fmt.Errorf("默认上线模板包含商城暂不支持的专属配置，请先调整模板")
	}
	ports := spec.Service.PortSpecs()
	if len(ports) != 1 || (ports[0].Protocol != "" && !strings.EqualFold(ports[0].Protocol, applicationservice.ServiceProtocolTCP)) || ports[0].Port != spec.ContainerPort || ports[0].TargetPort != spec.ContainerPort {
		return catalogComponentRequest{}, fmt.Errorf("当前仅支持容器端口与 Service 端口相同的单 TCP 端口应用")
	}
	spec.Version = ""
	spec.RegistryID = 0
	spec.Service = applicationservice.ServiceSpec{Type: applicationservice.ServiceTypeClusterIP, Ports: []applicationservice.ServicePortSpec{{Name: "http", Protocol: applicationservice.ServiceProtocolTCP, Port: spec.ContainerPort, TargetPort: spec.ContainerPort}}}
	spec.Endpoint = applicationservice.EndpointSpec{Exposure: applicationservice.ExposureCluster}
	return catalogComponentRequest{Name: app.Name, ApplicationName: app.Name, DefaultVersion: template.DefaultVersion, Spec: spec}, nil
}

func (h *ApplicationHandler) UpdateCatalogItem(c *gin.Context) {
	id, err := apiShared.ParseID(c.Param("catalogID"))
	if err != nil {
		apiShared.BadRequest(c, "商城条目 ID 无效")
		return
	}
	var req catalogItemRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Revision == 0 {
		apiShared.BadRequest(c, "商城模板定义或版本无效")
		return
	}
	_, err = h.applications.GetApplicationCatalogItem(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "商城应用不存在")
		return
	}
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	item, err := catalogItemFromRequest(req)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	item.ID = id
	if err := h.applications.UpdateApplicationCatalogItem(item, req.Revision); err != nil {
		apiShared.Conflict(c, err.Error())
		return
	}
	response, _ := catalogResponse(*item)
	apiShared.Success(c, response)
}

func (h *ApplicationHandler) DeleteCatalogItem(c *gin.Context) {
	id, err := apiShared.ParseID(c.Param("catalogID"))
	if err != nil {
		apiShared.BadRequest(c, "商城条目 ID 无效")
		return
	}
	if err := h.applications.DeleteApplicationCatalogItem(id); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	apiShared.Success(c, gin.H{"id": id})
}

func (h *ApplicationHandler) ImportCatalogItems(c *gin.Context) {
	var req struct {
		ProjectID     uint   `json:"project_id"`
		EnvironmentID uint   `json:"environment_id"`
		ItemIDs       []uint `json:"item_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ProjectID == 0 || req.EnvironmentID == 0 || len(req.ItemIDs) == 0 || len(req.ItemIDs) > 50 {
		apiShared.BadRequest(c, "请选择项目、环境和 1 至 50 个应用")
		return
	}
	environment, err := h.queries.ResolveEnvironment(req.ProjectID, req.EnvironmentID)
	if err != nil || environment.ClusterID == 0 {
		apiShared.ValidationError(c, "目标环境不存在或尚未绑定集群")
		return
	}
	if err := h.applications.EnsureNamespaceAvailable(environment.Namespace, environment.ID, environment.ClusterID); err != nil {
		apiShared.Conflict(c, "环境命名空间存在冲突，请先完成迁移")
		return
	}
	items := make([]model.ApplicationCatalogItem, 0, len(req.ItemIDs))
	seen := make(map[uint]bool, len(req.ItemIDs))
	for _, id := range req.ItemIDs {
		if id == 0 || seen[id] {
			apiShared.BadRequest(c, "商城条目不能重复选择")
			return
		}
		seen[id] = true
		item, err := h.applications.GetApplicationCatalogItem(id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apiShared.NotFound(c, "所选商城应用已不存在，请刷新列表")
			return
		}
		if err != nil {
			apiShared.DBError(c, err.Error())
			return
		}
		if !item.Enabled {
			apiShared.ValidationError(c, "商城应用尚未启用，请先完成配置并启用")
			return
		}
		items = append(items, *item)
	}
	installations, err := h.applications.ImportApplicationCatalogItems(req.ProjectID, req.EnvironmentID, apiShared.UserID(c), items)
	if err != nil {
		apiShared.Conflict(c, err.Error())
		return
	}
	apiShared.Success(c, installations)
}
