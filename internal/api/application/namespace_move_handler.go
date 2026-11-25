package applicationapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type namespaceMover interface {
	StageApplicationNamespace(context.Context, k8s.ApplicationNamespaceMove, bool) (k8s.ApplicationNamespaceMoveStatus, error)
	CheckApplicationNamespaceRoutes(context.Context, k8s.ApplicationNamespaceMove, []model.ApplicationEndpoint) error
}

func (h *ApplicationHandler) applicationForNamespaceMove(c *gin.Context) (*model.Application, bool) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return nil, false
	}
	app, err := h.queries.GetApplication(id)
	owner := apiShared.UserID(c)
	if err != nil || owner == 0 || app.Project.OwnerID != owner {
		apiShared.NotFound(c, "应用不存在")
		return nil, false
	}
	return app, true
}

func (h *ApplicationHandler) ApplicationNamespaceMoveOptions(c *gin.Context) {
	app, ok := h.applicationForNamespaceMove(c)
	if !ok {
		return
	}
	project, err := h.queries.GetProject(app.OwningProjectID())
	if err != nil || project.OwnerID != apiShared.UserID(c) {
		apiShared.NotFound(c, "所属项目不存在")
		return
	}
	environments, err := h.applications.ListEnvironments(project.ID)
	if err != nil {
		apiShared.DBError(c, "读取目标环境失败")
		return
	}
	targets := []model.Environment{}
	for _, target := range environments {
		if target.ClusterID != 0 && target.ClusterID == app.Environment.ClusterID && target.Namespace != app.Environment.Namespace {
			targets = append(targets, target)
		}
	}
	release, err := h.applications.GetLatestSuccessfulRelease(app.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.DBError(c, "读取当前部署版本失败")
		return
	}
	if err != nil {
		release = nil
	}
	maintenance, err := h.applications.GetApplicationNamespaceMaintenance(app.ID)
	if err != nil {
		apiShared.DBError(c, "读取迁移状态失败")
		return
	}
	apiShared.Success(c, gin.H{"maintenance": maintenance, "project_name": project.Name, "source": apiShared.EnvironmentDTO(&app.Environment), "targets": apiShared.EnvironmentsDTO(targets), "release": apiShared.ReleaseDTO(release)})
}

func (h *ApplicationHandler) MoveApplicationNamespace(c *gin.Context) {
	app, ok := h.applicationForNamespaceMove(c)
	if !ok {
		return
	}
	var input struct {
		SourceEnvironmentID uint   `json:"source_environment_id" binding:"required"`
		TargetEnvironmentID uint   `json:"target_environment_id" binding:"required"`
		ReleaseID           uint   `json:"release_id" binding:"required"`
		Action              string `json:"action" binding:"required,oneof=check prepare adopt prepare_maintenance start switch restore cancel"`
		Maintenance         bool   `json:"maintenance"`
		Confirmed           bool   `json:"confirmed"`
	}
	if c.ShouldBindJSON(&input) != nil || input.SourceEnvironmentID == input.TargetEnvironmentID ||
		(input.Action != "check" && !input.Confirmed) {
		apiShared.BadRequest(c, "迁移参数无效或尚未确认")
		return
	}
	move := model.ApplicationNamespaceMove{ApplicationID: app.ID, OwnerID: apiShared.UserID(c), SourceEnvironmentID: input.SourceEnvironmentID,
		TargetEnvironmentID: input.TargetEnvironmentID, ReleaseID: input.ReleaseID}
	if err := h.applications.CheckApplicationNamespaceMove(move); err != nil {
		apiShared.Conflict(c, fmt.Sprintf("无法迁移应用: %v", err))
		return
	}
	if app.EnvironmentID == move.TargetEnvironmentID {
		apiShared.Success(c, gin.H{"adopted": true, "environment_id": app.EnvironmentID, "project_id": app.ProjectID})
		return
	}
	task, err := h.applications.GetApplicationNamespaceMaintenance(app.ID)
	if err != nil {
		apiShared.DBError(c, "读取迁移状态失败")
		return
	}
	target, err := h.queries.ResolveEnvironment(app.OwningProjectID(), move.TargetEnvironmentID)
	if err != nil {
		apiShared.Conflict(c, "目标环境已变更")
		return
	}
	release, err := h.resources.GetRelease(move.ReleaseID)
	if err != nil || release.ApplicationID != app.ID {
		apiShared.Conflict(c, "源发布记录已变更")
		return
	}
	if input.Action != "restore" && input.Action != "cancel" {
		if err := h.namespaceMoveDependencies(app, target, release, &move); err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
	}
	scoped, err := h.forCluster(c.Request.Context(), target.ClusterID)
	if err != nil {
		apiShared.K8sUnavailable(c)
		return
	}
	mover, ok := scoped.kubernetes.(namespaceMover)
	if !ok {
		apiShared.K8sUnavailable(c)
		return
	}
	ctx := c.Request.Context()
	runtime := k8s.ApplicationNamespaceMove{ApplicationID: app.ID, Name: app.Name, SourceSequence: release.Sequence,
		SourceProjectID: app.ProjectID, SourceEnvironmentID: app.EnvironmentID, SourceNamespace: app.Environment.Namespace, TargetProjectID: target.ProjectID,
		TargetNamespace: target.Namespace, TargetEnvironmentID: target.ID}
	runtime.CertificatesBySecret = make(map[string]string)
	if input.Action != "restore" && input.Action != "cancel" {
		for _, endpoint := range app.Endpoints {
			if !endpoint.TLSEnabled || endpoint.DomainID == 0 {
				continue
			}
			domain, err := h.resources.GetManagedDomain(endpoint.DomainID)
			if err != nil {
				apiShared.Conflict(c, "读取迁移入口证书失败")
				return
			}
			if domain.CertificateName != "" && domain.TLSSecretName != "" {
				runtime.CertificatesBySecret[domain.TLSSecretName] = domain.CertificateName
			}
		}
	}
	if task != nil && input.Action == "prepare" {
		apiShared.Conflict(c, model.ErrApplicationNamespaceMaintenanceActive.Error())
		return
	}
	if task != nil || input.Maintenance || (input.Action != "check" && input.Action != "prepare" && input.Action != "adopt") || (app.WorkloadKind != "" && app.WorkloadKind != applicationservice.WorkloadKindDeployment) {
		scoped.handleNamespaceMaintenance(c, app, move, runtime, task, input.Action)
		return
	}
	if input.Action != "check" {
		unlock, ok := h.acquireApplicationRuntimeMutation(c, app.ID)
		if !ok {
			return
		}
		defer unlock()
	}
	status, err := mover.StageApplicationNamespace(ctx, runtime, input.Action == "prepare")
	if err != nil {
		if errors.Is(err, k8s.ErrNamespaceMoveNeedsMaintenance) && input.Action == "check" {
			scoped.handleNamespaceMaintenance(c, app, move, runtime, nil, input.Action)
			return
		}
		apiShared.Conflict(c, err.Error())
		return
	}
	if input.Action != "adopt" {
		apiShared.Success(c, status)
		return
	}
	if !status.Ready {
		apiShared.Conflict(c, "目标 Deployment 或 Service 还未就绪")
		return
	}
	if err := mover.CheckApplicationNamespaceRoutes(ctx, runtime, app.Endpoints); err != nil {
		apiShared.Conflict(c, err.Error())
		return
	}
	for _, endpoint := range app.Endpoints {
		if !endpoint.TLSEnabled || !endpointUsesIngress(endpoint) {
			continue
		}
		domain, err := h.resources.GetManagedDomain(endpoint.DomainID)
		if err != nil || domain.CertificateName == "" || domain.TLSSecretName != endpoint.TLSSecretName {
			apiShared.Conflict(c, "入口证书配置已变更")
			return
		}
		cert, err := scoped.kubernetes.GetCertificateContext(ctx, target.Namespace, domain.CertificateName)
		if err != nil || cert.Status != "Ready" || cert.SecretName != domain.TLSSecretName {
			apiShared.Conflict(c, "目标环境的入口证书尚未就绪")
			return
		}
	}
	if err := h.applications.AdoptApplicationNamespace(move); err != nil {
		apiShared.Conflict(c, fmt.Sprintf("无法接管目标环境: %v", err))
		return
	}
	apiShared.Success(c, gin.H{"adopted": true, "project_id": target.ProjectID, "environment_id": target.ID})
}

func (h *ApplicationHandler) namespaceMoveDependencies(app *model.Application, target *model.Environment, release *model.Release, move *model.ApplicationNamespaceMove) error {
	domains, err := h.applications.PlanApplicationNamespaceMoveDomains(*move)
	if err != nil {
		return err
	}
	move.DomainIDs = domains
	registries := map[uint]bool{}
	if release.ImageRegistryID != nil {
		registries[*release.ImageRegistryID] = true
	}
	templates, err := h.applications.ListApplicationDeploymentTemplates(app.ID)
	if err != nil {
		return err
	}
	for _, template := range templates {
		if !template.Enabled {
			continue
		}
		var spec applicationservice.ReleaseSpec
		if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
			return fmt.Errorf("无法读取上线模板 %s", template.Name)
		}
		if spec.RegistryID != 0 {
			registries[spec.RegistryID] = true
		}
	}
	for id := range registries {
		registry, err := h.resources.GetImageRegistryForProject(id, target.ProjectID)
		if err != nil || !registry.Enabled {
			return fmt.Errorf("请先向目标项目授权镜像仓库 #%d", id)
		}
	}
	return nil
}
