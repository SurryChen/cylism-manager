package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/application"
	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/google/go-containerregistry/pkg/name"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ApplicationHandler struct {
	store  *store.Store
	encKey []byte
}

var imageTagPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

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
	Spec        application.ReleaseSpec `json:"spec"`
}

type releaseRequest struct {
	TemplateID uint   `json:"template_id"`
	Version    string `json:"version"`
}

type applicationEndpointRequest struct {
	DomainID   uint   `json:"domain_id"`
	Path       string `json:"path"`
	TLSEnabled bool   `json:"tls_enabled"`
}

type workloadKindRequest struct {
	WorkloadKind string `json:"workload_kind"`
}

func NewApplicationHandler(store *store.Store, encKey ...[]byte) *ApplicationHandler {
	handler := &ApplicationHandler{store: store}
	if len(encKey) > 0 {
		handler.encKey = encKey[0]
	}
	return handler
}

func (h *ApplicationHandler) ListProjects(c *gin.Context) {
	projects, err := h.store.ListProjects()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, projects)
}

func (h *ApplicationHandler) CreateProject(c *gin.Context) {
	var req projectRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "项目名称必填")
		return
	}
	if req.DefaultImageRegistryID != nil && *req.DefaultImageRegistryID != 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "请在项目创建后设置默认镜像仓库")
		return
	}
	project := &model.Project{Name: req.Name, Description: req.Description, OwnerID: getUserID(c)}
	if err := h.store.CreateProject(project); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "项目名称已存在")
		return
	}
	model.Success(c, project)
}

func (h *ApplicationHandler) UpdateProject(c *gin.Context) {
	projectID, err := parseID(c.Param("projectID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "项目 ID 无效")
		return
	}
	var req projectRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "项目名称必填")
		return
	}
	project, err := h.store.GetProject(projectID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "项目不存在")
		return
	}
	if project.Name != req.Name {
		applicationCount, err := h.store.CountProjectApplications(projectID)
		if err != nil {
			model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
			return
		}
		if applicationCount > 0 {
			model.Error(c, http.StatusConflict, model.CodeConflict, "项目已有应用，不能修改项目名称")
			return
		}
	}
	project.Name = req.Name
	project.Description = req.Description
	if req.DefaultImageRegistryID != nil {
		if *req.DefaultImageRegistryID == 0 {
			project.DefaultImageRegistryID = nil
		} else {
			registry, err := h.store.GetImageRegistryForProject(*req.DefaultImageRegistryID, projectID)
			if err != nil || !registry.Enabled {
				model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "默认镜像仓库未授权当前项目或已停用")
				return
			}
			project.DefaultImageRegistryID = &registry.ID
		}
	}
	if err := h.store.UpdateProject(project); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "项目名称已存在")
		return
	}
	model.Success(c, project)
}

func (h *ApplicationHandler) DeleteProject(c *gin.Context) {
	projectID, err := parseID(c.Param("projectID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "项目 ID 无效")
		return
	}
	if _, err := h.store.GetProject(projectID); err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "项目不存在")
		return
	}
	environmentCount, err := h.store.CountProjectEnvironments(projectID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	applicationCount, err := h.store.CountProjectApplications(projectID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	if environmentCount > 0 || applicationCount > 0 {
		model.Error(c, http.StatusConflict, model.CodeConflict, "项目仍关联环境或应用，无法删除")
		return
	}
	if err := h.store.DeleteProject(projectID); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, gin.H{"id": projectID})
}

func (h *ApplicationHandler) ListEnvironments(c *gin.Context) {
	projectID, err := parseID(c.Param("projectID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "项目 ID 无效")
		return
	}
	environments, err := h.store.ListEnvironments(projectID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	conflicts, err := h.store.ListEnvironmentNamespaceConflicts()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	conflictNamespaces := make(map[string]struct{}, len(conflicts))
	for _, conflict := range conflicts {
		conflictNamespaces[conflict.Namespace] = struct{}{}
	}
	for index := range environments {
		environments[index].NamespaceStatus = environmentNamespaceStatus(c.Request.Context(), environments[index].Namespace)
		_, environments[index].NamespaceConflict = conflictNamespaces[environments[index].Namespace]
	}
	model.Success(c, environments)
}

func (h *ApplicationHandler) CreateEnvironment(c *gin.Context) {
	projectID, err := parseID(c.Param("projectID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "项目 ID 无效")
		return
	}
	if _, err := h.store.GetProject(projectID); err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "项目不存在")
		return
	}
	var req environmentRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Namespace) == "" {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "环境名称和命名空间必填")
		return
	}
	mode, err := normalizeEnvironmentNamespaceMode(req.NamespaceMode)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if K8s == nil || K8s.Clientset == nil {
		k8sUnavailable(c)
		return
	}
	name, namespace := strings.TrimSpace(req.Name), strings.TrimSpace(req.Namespace)
	if err := h.store.EnsureNamespaceAvailable(namespace, 0); err != nil {
		handleEnvironmentNamespaceError(c, err)
		return
	}
	if err := ensureEnvironmentNamespace(c.Request.Context(), projectID, namespace, mode, false); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	environment := &model.Environment{ProjectID: projectID, Name: name, Namespace: namespace, NamespaceStatus: "active"}
	if err := h.store.CreateEnvironment(environment); err != nil {
		if handleEnvironmentNamespaceError(c, err) {
			return
		}
		model.Error(c, http.StatusConflict, model.CodeConflict, "环境名称已存在")
		return
	}
	model.Success(c, environment)
}

func (h *ApplicationHandler) UpdateEnvironment(c *gin.Context) {
	projectID, environmentID, ok := h.environmentRouteIDs(c)
	if !ok {
		return
	}
	var req environmentRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Namespace) == "" {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "环境名称和命名空间必填")
		return
	}
	environment, err := h.store.GetEnvironment(projectID, environmentID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "环境不存在")
		return
	}
	name, namespace := strings.TrimSpace(req.Name), strings.TrimSpace(req.Namespace)
	if environment.Name != name || environment.Namespace != namespace {
		applicationCount, err := h.store.CountEnvironmentApplications(environmentID)
		if err != nil {
			model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
			return
		}
		namespaceConflict, err := h.store.IsEnvironmentNamespaceConflicted(environment.Namespace)
		if err != nil {
			model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
			return
		}
		if applicationCount > 0 && !namespaceConflict {
			model.Error(c, http.StatusConflict, model.CodeConflict, "环境已有应用，不能修改名称或命名空间")
			return
		}
		if environment.Namespace != namespace {
			mode, err := normalizeEnvironmentNamespaceMode(req.NamespaceMode)
			if err != nil {
				model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
				return
			}
			if K8s == nil || K8s.Clientset == nil {
				k8sUnavailable(c)
				return
			}
			if err := h.store.EnsureNamespaceAvailable(namespace, environment.ID); err != nil {
				handleEnvironmentNamespaceError(c, err)
				return
			}
			if err := ensureEnvironmentNamespace(c.Request.Context(), projectID, namespace, mode, false); err != nil {
				model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
				return
			}
		}
	}
	environment.Name = name
	environment.Namespace = namespace
	environment.NamespaceStatus = environmentNamespaceStatus(c.Request.Context(), namespace)
	if err := h.store.UpdateEnvironment(environment); err != nil {
		if handleEnvironmentNamespaceError(c, err) {
			return
		}
		model.Error(c, http.StatusConflict, model.CodeConflict, "环境名称已存在")
		return
	}
	model.Success(c, environment)
}

func (h *ApplicationHandler) SyncEnvironmentNamespace(c *gin.Context) {
	projectID, environmentID, ok := h.environmentRouteIDs(c)
	if !ok {
		return
	}
	environment, err := h.store.GetEnvironment(projectID, environmentID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "环境不存在")
		return
	}
	if K8s == nil || K8s.Clientset == nil {
		k8sUnavailable(c)
		return
	}
	if err := h.store.EnsureNamespaceAvailable(environment.Namespace, environment.ID); err != nil {
		handleEnvironmentNamespaceError(c, err)
		return
	}
	if err := ensureEnvironmentNamespace(c.Request.Context(), projectID, environment.Namespace, "create", true); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	environment.NamespaceStatus = environmentNamespaceStatus(c.Request.Context(), environment.Namespace)
	model.SuccessWithMessage(c, environment, "命名空间已同步")
}

func (h *ApplicationHandler) ListEnvironmentNamespaceConflicts(c *gin.Context) {
	conflicts, err := h.store.ListEnvironmentNamespaceConflicts()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, conflicts)
}

func (h *ApplicationHandler) DeleteEnvironment(c *gin.Context) {
	projectID, environmentID, ok := h.environmentRouteIDs(c)
	if !ok {
		return
	}
	if _, err := h.store.GetEnvironment(projectID, environmentID); err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "环境不存在")
		return
	}
	applicationCount, err := h.store.CountEnvironmentApplications(environmentID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	if applicationCount > 0 {
		model.Error(c, http.StatusConflict, model.CodeConflict, "环境仍关联应用，无法删除")
		return
	}
	if err := h.store.DeleteEnvironment(environmentID); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, gin.H{"id": environmentID})
}

type projectRequest struct {
	Name                   string `json:"name"`
	Description            string `json:"description"`
	DefaultImageRegistryID *uint  `json:"default_image_registry_id"`
}

type environmentRequest struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	NamespaceMode string `json:"namespace_mode"`
}

func normalizeEnvironmentNamespaceMode(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case "", "create":
		return "create", nil
	case "bind":
		return "bind", nil
	default:
		return "", fmt.Errorf("命名空间方式仅支持新建或绑定")
	}
}

func ensureEnvironmentNamespace(ctx context.Context, projectID uint, namespace, mode string, allowExisting bool) error {
	if isSystemNamespace(namespace) {
		return fmt.Errorf("系统命名空间 %q 不能绑定为应用环境", namespace)
	}
	existing, err := K8s.Clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err == nil {
		if existing.Status.Phase != "" && existing.Status.Phase != corev1.NamespaceActive {
			return fmt.Errorf("命名空间 %q 未就绪", namespace)
		}
		if mode == "create" && !allowExisting {
			return fmt.Errorf("命名空间 %q 已存在，请选择绑定已有命名空间", namespace)
		}
		labels := existing.GetLabels()
		owner := strings.TrimSpace(labels["cylism.io/project-id"])
		project := strconv.FormatUint(uint64(projectID), 10)
		if owner != "" && owner != project {
			return fmt.Errorf("命名空间 %q 已属于项目 %s，不能绑定到当前项目", namespace, owner)
		}
		if owner == "" {
			if labels == nil {
				labels = map[string]string{}
			}
			labels["cylism.io/project-id"] = project
			labels["app.kubernetes.io/managed-by"] = "cylism-manager"
			existing.Labels = labels
			if _, err := K8s.Clientset.CoreV1().Namespaces().Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
				return fmt.Errorf("认领命名空间 %q: %w", namespace, err)
			}
		}
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("检查命名空间: %w", err)
	}
	if mode == "bind" {
		return fmt.Errorf("命名空间 %q 不存在，无法绑定", namespace)
	}
	_, err = K8s.Clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace, Labels: map[string]string{
		"app.kubernetes.io/managed-by": "cylism-manager",
		"cylism.io/project-id":         strconv.FormatUint(uint64(projectID), 10),
	}}}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("创建命名空间 %q: %w", namespace, err)
	}
	return nil
}

func isSystemNamespace(namespace string) bool {
	switch namespace {
	case "default", "kube-system", "kube-public", "kube-node-lease":
		return true
	default:
		return false
	}
}

func handleEnvironmentNamespaceError(c *gin.Context, err error) bool {
	var conflict *store.NamespaceConflictError
	if errors.As(err, &conflict) {
		model.Error(c, http.StatusConflict, model.CodeConflict, conflict.Error())
		return true
	}
	return false
}

func environmentNamespaceStatus(ctx context.Context, namespace string) string {
	if K8s == nil || K8s.Clientset == nil {
		return "unknown"
	}
	resource, err := K8s.Clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "missing"
	}
	if err != nil {
		return "unknown"
	}
	if resource.Status.Phase == corev1.NamespaceActive {
		return "active"
	}
	if resource.Status.Phase == corev1.NamespaceTerminating {
		return "terminating"
	}
	return "pending"
}

func (h *ApplicationHandler) environmentRouteIDs(c *gin.Context) (uint, uint, bool) {
	projectID, err := parseID(c.Param("projectID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "项目 ID 无效")
		return 0, 0, false
	}
	environmentID, err := parseID(c.Param("environmentID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "环境 ID 无效")
		return 0, 0, false
	}
	return projectID, environmentID, true
}

func (h *ApplicationHandler) ListApplications(c *gin.Context) {
	projectID, err := optionalQueryID(c, "project_id")
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "项目 ID 无效")
		return
	}
	environmentID, err := optionalQueryID(c, "environment_id")
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "环境 ID 无效")
		return
	}
	applications, err := h.store.ListApplications(projectID, environmentID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, applications)
}

func (h *ApplicationHandler) GetApplication(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	app, err := h.store.GetApplication(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "应用不存在")
		return
	}
	releases, _ := h.store.ListReleases(id)
	model.Success(c, gin.H{"application": app, "releases": releases})
}

func (h *ApplicationHandler) UpdateWorkloadKind(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	applicationID, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	var req workloadKindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "工作负载类型无效")
		return
	}
	req.WorkloadKind = strings.ToLower(strings.TrimSpace(req.WorkloadKind))
	if req.WorkloadKind != application.WorkloadKindDeployment && req.WorkloadKind != application.WorkloadKindStatefulSet {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "工作负载类型必须为 Deployment 或 StatefulSet")
		return
	}
	app, err := h.store.GetApplication(applicationID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "应用不存在")
		return
	}
	if app.WorkloadKind == req.WorkloadKind {
		model.Success(c, app)
		return
	}
	release, err := h.store.GetLatestSuccessfulRelease(app.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		app.WorkloadKind = req.WorkloadKind
		if err := h.store.UpdateApplication(app); err != nil {
			model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
			return
		}
		model.Success(c, app)
		return
	}
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	var spec application.ReleaseSpec
	if err := json.Unmarshal([]byte(release.DesiredSpec), &spec); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取成功发布快照失败")
		return
	}
	context := applicationContextFor(app)
	context.ReleaseSequence = release.Sequence
	if err := application.NewKubernetesApplier(K8s).Preflight(c.Request.Context(), context, spec); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := application.NewKubernetesApplier(K8s).MigrateWorkloadKind(c.Request.Context(), context, spec, req.WorkloadKind); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	app.WorkloadKind = req.WorkloadKind
	if err := h.store.UpdateApplication(app); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, app)
}

func (h *ApplicationHandler) CreateApplication(c *gin.Context) {
	var req struct {
		ProjectID     uint   `json:"project_id"`
		EnvironmentID uint   `json:"environment_id"`
		Name          string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ProjectID == 0 || req.EnvironmentID == 0 || strings.TrimSpace(req.Name) == "" {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "项目、环境和应用名称必填")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if err := application.ValidateApplicationName(req.Name); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	environment, err := h.store.GetEnvironment(req.ProjectID, req.EnvironmentID)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "环境不属于所选项目")
		return
	}
	if err := h.store.EnsureNamespaceAvailable(environment.Namespace, environment.ID); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "环境命名空间存在冲突，请先完成迁移")
		return
	}
	app := &model.Application{ProjectID: req.ProjectID, EnvironmentID: req.EnvironmentID, Name: req.Name, WorkloadKind: "deployment", CreatedBy: getUserID(c)}
	if err := h.store.CreateApplication(app); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "该环境内应用名称已存在")
		return
	}
	model.Success(c, app)
}

func (h *ApplicationHandler) WorkspaceOverview(c *gin.Context) {
	projectID, err := optionalQueryID(c, "project_id")
	if err != nil || projectID == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "项目 ID 必填且必须有效")
		return
	}
	environmentID, err := optionalQueryID(c, "environment_id")
	if err != nil || environmentID == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "环境 ID 必填且必须有效")
		return
	}
	project, err := h.store.GetProject(projectID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "项目不存在")
		return
	}
	environment, err := h.store.GetEnvironment(projectID, environmentID)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "环境不属于所选项目")
		return
	}
	applications, err := h.store.ListApplications(projectID, environmentID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	domains, err := h.store.ListManagedDomains(environmentID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	type workspaceRelease struct {
		model.Release
		ApplicationName string `json:"application_name"`
	}
	failedReleases := make([]workspaceRelease, 0)
	recentReleases := make([]workspaceRelease, 0)
	for _, app := range applications {
		releases, releaseErr := h.store.ListReleases(app.ID)
		if releaseErr != nil {
			continue
		}
		for _, release := range releases {
			item := workspaceRelease{Release: release, ApplicationName: app.Name}
			recentReleases = append(recentReleases, item)
			if release.Status == model.ReleaseStatusFailed {
				failedReleases = append(failedReleases, item)
			}
		}
	}
	sort.Slice(recentReleases, func(i, j int) bool { return recentReleases[i].CreatedAt.After(recentReleases[j].CreatedAt) })
	if len(recentReleases) > 8 {
		recentReleases = recentReleases[:8]
	}
	domainInfos := make([]managedDomainInfo, 0, len(domains))
	for index := range domains {
		domainInfos = append(domainInfos, NewDomainHandler(h.store).domainInfo(&domains[index]))
	}
	model.Success(c, gin.H{"project": project, "environment": environment, "applications": applications, "domains": domainInfos, "failed_releases": failedReleases, "recent_releases": recentReleases})
}

func (h *ApplicationHandler) CreateRelease(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	applicationID, err := parseID(c.Param("id"))
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
	app, err := h.store.GetApplication(applicationID)
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
	var spec application.ReleaseSpec
	if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取应用上线模板失败")
		return
	}
	secrets, err := h.decryptTemplateSecrets(template)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取模板 Secret 失败")
		return
	}
	if len(spec.Secrets) > 0 && len(secrets) == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "模板中的 Secret 尚未配置明文值")
		return
	}
	spec.Secrets = secrets
	spec.Version = version
	image, err := imageWithVersion(spec.Image, version)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	spec.Image = image
	if err := h.prepareRegistryReleaseSpec(app, &spec); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.prepareReleaseImageVerification(&spec); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.applyApplicationEndpointSpec(app, &spec); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	service := application.NewService(h.store, application.NewKubernetesApplier(K8s))
	release, err := service.CreateRelease(c.Request.Context(), applicationID, getUserID(c), spec)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	templateID := template.ID
	release.TemplateID = &templateID
	release.TemplateRevision = template.Revision
	if err := h.store.UpdateRelease(release); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	h.executeAsync(service, release.ID, app, spec)
	model.SuccessWithMessage(c, release, "发布已创建")
}

func (h *ApplicationHandler) ListDeploymentTemplates(c *gin.Context) {
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
	templates, err := h.store.ListApplicationDeploymentTemplates(applicationID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	infos := make([]deploymentTemplateInfo, 0, len(templates))
	for index := range templates {
		info, err := deploymentTemplateFromModel(&templates[index], app.DefaultDeploymentTemplateID)
		if err != nil {
			model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取应用上线模板失败")
			return
		}
		infos = append(infos, *info)
	}
	model.Success(c, infos)
}

func (h *ApplicationHandler) GetDeploymentTemplate(c *gin.Context) {
	applicationID, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	templateID, err := parseID(c.Param("templateID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "上线模板 ID 无效")
		return
	}
	app, err := h.store.GetApplication(applicationID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "应用不存在")
		return
	}
	template, err := h.store.GetApplicationDeploymentTemplate(applicationID, templateID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "上线模板不存在")
		return
	}
	info, err := deploymentTemplateFromModel(template, app.DefaultDeploymentTemplateID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取应用上线模板失败")
		return
	}
	model.Success(c, info)
}

func (h *ApplicationHandler) CreateDeploymentTemplate(c *gin.Context) {
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
	var req deploymentTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "上线模板定义无效")
		return
	}
	template, err := h.templateFromRequest(app, &req, 0)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	template.UpdatedBy = getUserID(c)
	if err := h.store.CreateApplicationDeploymentTemplate(template, app.DefaultDeploymentTemplateID == nil); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	if app.DefaultDeploymentTemplateID == nil {
		app.DefaultDeploymentTemplateID = &template.ID
	}
	info, err := deploymentTemplateFromModel(template, app.DefaultDeploymentTemplateID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取上线模板失败")
		return
	}
	model.Success(c, info)
}

func (h *ApplicationHandler) UpdateDeploymentTemplate(c *gin.Context) {
	applicationID, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	templateID, err := parseID(c.Param("templateID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "上线模板 ID 无效")
		return
	}
	app, err := h.store.GetApplication(applicationID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "应用不存在")
		return
	}
	var req deploymentTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "上线模板定义无效")
		return
	}
	if app.DefaultDeploymentTemplateID != nil && *app.DefaultDeploymentTemplateID == templateID && !req.Enabled {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "默认模板不能停用，请先设置其他启用模板为默认")
		return
	}
	template, err := h.templateFromRequest(app, &req, templateID)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	template.UpdatedBy = getUserID(c)
	if err := h.store.UpdateApplicationDeploymentTemplate(template); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			model.Error(c, http.StatusNotFound, model.CodeNotFound, "上线模板不存在")
			return
		}
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	info, err := deploymentTemplateFromModel(template, app.DefaultDeploymentTemplateID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取上线模板失败")
		return
	}
	model.Success(c, info)
}

func (h *ApplicationHandler) DeleteDeploymentTemplate(c *gin.Context) {
	applicationID, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	templateID, err := parseID(c.Param("templateID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "上线模板 ID 无效")
		return
	}
	if err := h.store.DeleteApplicationDeploymentTemplate(applicationID, templateID); err != nil {
		if strings.Contains(err.Error(), "关联发布") {
			model.Error(c, http.StatusConflict, model.CodeConflict, err.Error())
			return
		}
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "上线模板不存在")
		return
	}
	model.Success(c, gin.H{"id": templateID})
}

func (h *ApplicationHandler) SetDefaultDeploymentTemplate(c *gin.Context) {
	applicationID, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	templateID, err := parseID(c.Param("templateID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "上线模板 ID 无效")
		return
	}
	if err := h.store.SetDefaultApplicationDeploymentTemplate(applicationID, templateID); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "上线模板不存在或已停用")
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
	if strings.TrimSpace(spec.Version) != "" {
		return nil, fmt.Errorf("上线模板不应包含版本号")
	}
	if err := validateImageRepository(spec.Image); err != nil {
		return nil, err
	}
	if err := h.prepareRegistryReleaseSpec(app, &spec); err != nil {
		return nil, err
	}
	encryptedSecrets := ""
	if templateID != 0 {
		current, err := h.store.GetApplicationDeploymentTemplate(app.ID, templateID)
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
		context := application.ApplicationContext{EnvironmentID: app.Environment.ID, Namespace: app.Environment.Namespace}
		if err := application.NewKubernetesApplier(K8s).ValidatePersistentVolumeClaims(context, spec); err != nil {
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

func (h *ApplicationHandler) GetApplicationEndpoint(c *gin.Context) {
	applicationID, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	if _, err := h.store.GetApplication(applicationID); err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "应用不存在")
		return
	}
	endpoint, err := h.store.GetApplicationEndpoint(applicationID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		model.Success(c, nil)
		return
	}
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, endpoint)
}

func (h *ApplicationHandler) UpdateApplicationEndpoint(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
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
	var req applicationEndpointRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.DomainID == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "请选择受管域名")
		return
	}
	servicePort, err := h.applicationServicePort(app)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	spec := application.ReleaseSpec{Endpoint: application.EndpointSpec{Exposure: application.ExposurePublic, DomainID: req.DomainID, Path: strings.TrimSpace(req.Path), TLSEnabled: req.TLSEnabled}}
	if err := h.prepareManagedDomain(app, &spec); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	context := applicationContextFor(app)
	if err := application.NewKubernetesApplier(K8s).SyncEndpoint(c.Request.Context(), context, spec.Endpoint, servicePort); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, fmt.Sprintf("同步应用入口: %v", err))
		return
	}
	endpoint := &model.ApplicationEndpoint{ApplicationID: app.ID, DomainID: spec.Endpoint.DomainID, Exposure: spec.Endpoint.Exposure, Domain: spec.Endpoint.Domain, Path: spec.Endpoint.Path, ServicePort: servicePort, TLSEnabled: spec.Endpoint.TLSEnabled, TLSSecretName: spec.Endpoint.ManagedTLSSecretName, IssuerRef: spec.Endpoint.IssuerRef}
	if err := h.store.ReplaceApplicationEndpoint(endpoint); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, endpoint)
}

func (h *ApplicationHandler) DeleteApplicationEndpoint(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
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
	if err := application.NewKubernetesApplier(K8s).RemoveEndpoint(c.Request.Context(), applicationContextFor(app)); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, fmt.Sprintf("移除应用入口: %v", err))
		return
	}
	if err := h.store.DeleteApplicationEndpoint(applicationID); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, gin.H{"id": applicationID})
}

func (h *ApplicationHandler) applicationServicePort(app *model.Application) (int32, error) {
	if release, err := h.store.GetLatestSuccessfulRelease(app.ID); err == nil {
		var spec application.ReleaseSpec
		if err := json.Unmarshal([]byte(release.DesiredSpec), &spec); err != nil {
			return 0, err
		}
		return spec.Service.Port, nil
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	if template, err := h.store.GetDefaultApplicationDeploymentTemplate(app.ID); err == nil {
		var spec application.ReleaseSpec
		if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
			return 0, err
		}
		return spec.Service.Port, nil
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	if endpoint, err := h.store.GetApplicationEndpoint(app.ID); err == nil && endpoint.ServicePort > 0 {
		return endpoint.ServicePort, nil
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	return 80, nil
}

func (h *ApplicationHandler) applyApplicationEndpointSpec(app *model.Application, spec *application.ReleaseSpec) error {
	endpoint, err := h.store.GetApplicationEndpoint(app.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		spec.Endpoint = application.EndpointSpec{Exposure: application.ExposureCluster}
		return nil
	}
	if err != nil {
		return err
	}
	spec.Endpoint = application.EndpointSpec{Exposure: endpoint.Exposure, DomainID: endpoint.DomainID, Domain: endpoint.Domain, Path: endpoint.Path, TLSEnabled: endpoint.TLSEnabled, IssuerRef: endpoint.IssuerRef, ManagedTLSSecretName: endpoint.TLSSecretName}
	if spec.Endpoint.Exposure != application.ExposurePublic {
		return nil
	}
	return h.prepareManagedDomain(app, spec)
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

func imageWithVersion(repository, version string) (string, error) {
	if err := validateImageRepository(repository); err != nil {
		return "", err
	}
	version = strings.TrimSpace(version)
	if !imageTagPattern.MatchString(version) {
		return "", fmt.Errorf("版本号必须是合法的镜像 Tag")
	}
	return strings.Trim(strings.TrimSpace(repository), "/") + ":" + version, nil
}

func (h *ApplicationHandler) RetryRelease(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	applicationID, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	releaseID, err := parseID(c.Param("releaseID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "发布 ID 无效")
		return
	}
	original, err := h.store.GetRelease(releaseID)
	if err != nil || original.ApplicationID != applicationID {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "发布不存在")
		return
	}
	service := application.NewService(h.store, application.NewKubernetesApplier(K8s))
	release, spec, err := service.RetryRelease(releaseID, getUserID(c))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "无法重试该发布")
		return
	}
	app, _ := h.store.GetApplication(applicationID)
	if err := h.prepareRegistryReleaseSpec(app, &spec); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.prepareReleaseImageVerification(&spec); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.applyApplicationEndpointSpec(app, &spec); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	h.executeAsync(service, release.ID, app, spec)
	model.SuccessWithMessage(c, release, "重试已创建")
}

func (h *ApplicationHandler) RollbackRelease(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	applicationID, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	releaseID, err := parseID(c.Param("releaseID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "发布 ID 无效")
		return
	}
	original, err := h.store.GetRelease(releaseID)
	if err != nil || original.ApplicationID != applicationID {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "发布不存在")
		return
	}
	service := application.NewService(h.store, application.NewKubernetesApplier(K8s))
	release, spec, err := service.RollbackRelease(releaseID, getUserID(c))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "无法回滚该发布")
		return
	}
	app, _ := h.store.GetApplication(applicationID)
	if err := h.prepareRegistryReleaseSpec(app, &spec); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.prepareReleaseImageVerification(&spec); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.applyApplicationEndpointSpec(app, &spec); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	h.executeAsync(service, release.ID, app, spec)
	model.SuccessWithMessage(c, release, "回滚已创建")
}

func (h *ApplicationHandler) prepareRegistryReleaseSpec(app *model.Application, spec *application.ReleaseSpec) error {
	if spec.RegistryID == 0 {
		return nil
	}
	if app == nil {
		return fmt.Errorf("应用不存在")
	}
	registry, err := h.store.GetImageRegistryForProject(spec.RegistryID, app.ProjectID)
	if err != nil || !registry.Enabled {
		return fmt.Errorf("镜像仓库不存在、未授权当前项目或已禁用")
	}
	image, err := registryImageReference(registry.Endpoint, spec.Image)
	if err != nil {
		return err
	}
	spec.Image = image
	spec.RegistryEndpoint = registry.Endpoint
	spec.RegistryAuthType = registry.AuthType
	if registry.AuthType == registryAuthAnonymous {
		return nil
	}
	credential, err := crypto.Decrypt(h.encKey, registry.Credential)
	if err != nil {
		return fmt.Errorf("读取镜像仓库凭据失败")
	}
	spec.RegistryUsername = registry.Username
	if registry.AuthType == registryAuthToken && spec.RegistryUsername == "" {
		spec.RegistryUsername = "token"
	}
	spec.RegistryCredential = credential
	return nil
}

func (h *ApplicationHandler) prepareReleaseImageVerification(spec *application.ReleaseSpec) error {
	spec.ImageVerificationEndpoint = ""
	spec.ImageVerificationUsername = ""
	spec.ImageVerificationCredential = ""
	spec.ImageVerificationInsecureSkipVerify = false
	if spec.NodeName == "" {
		return nil
	}
	ref, err := name.ParseReference(spec.Image)
	if err != nil {
		return fmt.Errorf("镜像地址无效: %w", err)
	}
	mirrors, err := h.store.ListNodeRegistryMirrors()
	if err != nil {
		return fmt.Errorf("读取节点镜像源失败: %w", err)
	}
	for _, mirror := range mirrors {
		if !mirror.Enabled || !sameRegistry(mirror.Registry, ref.Context().RegistryStr()) || !mirrorAppliedToNode(mirror, spec.NodeName) {
			continue
		}
		var endpoints []string
		if err := json.Unmarshal([]byte(mirror.Endpoints), &endpoints); err != nil || len(endpoints) == 0 {
			return fmt.Errorf("节点镜像源 %q 配置损坏", mirror.Name)
		}
		endpoint := strings.TrimSpace(endpoints[0])
		if endpoint == "" {
			return fmt.Errorf("节点镜像源 %q 未配置可用地址", mirror.Name)
		}
		spec.ImageVerificationEndpoint = endpoint
		spec.ImageVerificationUsername = mirror.Username
		spec.ImageVerificationInsecureSkipVerify = mirror.InsecureSkipVerify
		if mirror.Username != "" {
			credential, err := crypto.Decrypt(h.encKey, mirror.Credential)
			if err != nil {
				return fmt.Errorf("读取节点镜像源 %q 凭据失败", mirror.Name)
			}
			spec.ImageVerificationCredential = credential
		}
		return nil
	}
	return nil
}

func sameRegistry(left, right string) bool {
	normalize := func(value string) string {
		value = strings.ToLower(strings.TrimSpace(value))
		switch value {
		case "index.docker.io", "registry-1.docker.io":
			return "docker.io"
		default:
			return value
		}
	}
	return normalize(left) == normalize(right)
}

func mirrorAppliedToNode(mirror model.NodeRegistryMirror, nodeName string) bool {
	for _, status := range mirror.NodeStatuses {
		if status.Status == "success" && status.Server.K8sNodeName == nodeName {
			return true
		}
	}
	return false
}

func (h *ApplicationHandler) prepareManagedDomain(app *model.Application, spec *application.ReleaseSpec) error {
	if spec.Endpoint.DomainID == 0 {
		if spec.Endpoint.ManagedCertificateName != "" || spec.Endpoint.ManagedTLSSecretName != "" {
			return fmt.Errorf("受管证书只能通过受管域名选择")
		}
		return nil
	}
	domain, err := h.store.GetManagedDomain(spec.Endpoint.DomainID)
	if err != nil || !domain.Enabled {
		return fmt.Errorf("域名不存在或已停用")
	}
	if domain.EnvironmentID == 0 || domain.EnvironmentID != app.EnvironmentID {
		return fmt.Errorf("域名仅可用于当前应用环境")
	}
	path := spec.Endpoint.Path
	if path == "" {
		path = "/"
		spec.Endpoint.Path = path
	}
	conflicts, err := h.store.CountApplicationEndpointRoute(domain.ID, path, app.ID)
	if err != nil {
		return fmt.Errorf("检查域名路由冲突: %w", err)
	}
	if conflicts > 0 {
		return fmt.Errorf("域名 %q 的路径 %q 已被其他应用入口使用", domain.Hostname, path)
	}
	spec.Endpoint.Domain = domain.Hostname
	spec.Endpoint.IssuerRef = domain.IssuerRef
	spec.Endpoint.IssuerKind = domain.IssuerKind
	if !spec.Endpoint.TLSEnabled {
		return nil
	}
	if domain.CertificateName == "" || domain.TLSSecretName == "" {
		return fmt.Errorf("域名尚未申请证书")
	}
	certificate, err := K8s.GetCertificate(domain.Namespace, domain.CertificateName)
	if err != nil {
		return fmt.Errorf("读取域名证书: %w", err)
	}
	if certificate.Status != "Ready" {
		detail := certificate.Reason
		if detail == "" {
			detail = "等待 cert-manager 签发"
		}
		return fmt.Errorf("域名证书尚未就绪: %s", detail)
	}
	spec.Endpoint.ManagedCertificateName = domain.CertificateName
	spec.Endpoint.ManagedTLSSecretName = domain.TLSSecretName
	return nil
}

func registryImageReference(endpoint, image string) (string, error) {
	image = strings.Trim(strings.TrimSpace(image), "/")
	if image == "" || strings.ContainsAny(image, " \t\r\n") {
		return "", fmt.Errorf("镜像路径不能为空且不能包含空格")
	}
	prefix := endpoint + "/"
	if strings.HasPrefix(image, prefix) {
		return image, nil
	}
	return prefix + image, nil
}

func (h *ApplicationHandler) GetRelease(c *gin.Context) {
	applicationID, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "应用 ID 无效")
		return
	}
	id, err := parseID(c.Param("releaseID"))
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
	applicationModel, err := h.store.GetApplication(applicationID)
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

func (h *ApplicationHandler) executeAsync(service *application.Service, releaseID uint, app *model.Application, spec application.ReleaseSpec) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		_ = service.ExecuteRelease(ctx, releaseID, app, spec)
	}()
}

func parseID(value string) (uint, error) {
	id, err := strconv.ParseUint(value, 10, 64)
	return uint(id), err
}
