package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/application"
	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ApplicationHandler struct {
	store  *store.Store
	encKey []byte
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
	for index := range environments {
		environments[index].NamespaceStatus = environmentNamespaceStatus(c.Request.Context(), environments[index].Namespace)
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
	if err := ensureEnvironmentNamespace(c.Request.Context(), projectID, namespace, mode, false); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	environment := &model.Environment{ProjectID: projectID, Name: name, Namespace: namespace, NamespaceStatus: "active"}
	if err := h.store.CreateEnvironment(environment); err != nil {
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
		if applicationCount > 0 {
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
	if err := ensureEnvironmentNamespace(c.Request.Context(), projectID, environment.Namespace, "create", true); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	environment.NamespaceStatus = environmentNamespaceStatus(c.Request.Context(), environment.Namespace)
	model.SuccessWithMessage(c, environment, "命名空间已同步")
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
	existing, err := K8s.Clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err == nil {
		if existing.Status.Phase != "" && existing.Status.Phase != corev1.NamespaceActive {
			return fmt.Errorf("命名空间 %q 未就绪", namespace)
		}
		if mode == "create" && !allowExisting {
			return fmt.Errorf("命名空间 %q 已存在，请选择绑定已有命名空间", namespace)
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
	applications, err := h.store.ListApplications()
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
	app := &model.Application{ProjectID: req.ProjectID, EnvironmentID: req.EnvironmentID, Name: req.Name, WorkloadKind: "deployment", CreatedBy: getUserID(c)}
	if err := h.store.CreateApplication(app); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "该环境内应用名称已存在")
		return
	}
	model.Success(c, app)
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
	var spec application.ReleaseSpec
	if err := c.ShouldBindJSON(&spec); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "发布定义无效")
		return
	}
	app, err := h.store.GetApplication(applicationID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "应用不存在")
		return
	}
	if err := h.prepareRegistryReleaseSpec(app, &spec); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.prepareManagedDomain(&spec); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	service := application.NewService(h.store, application.NewKubernetesApplier(K8s))
	release, err := service.CreateRelease(c.Request.Context(), applicationID, getUserID(c), spec)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	h.executeAsync(service, release.ID, app, spec)
	model.SuccessWithMessage(c, release, "发布已创建")
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

func (h *ApplicationHandler) prepareManagedDomain(spec *application.ReleaseSpec) error {
	if spec.Endpoint.DomainID == 0 {
		return nil
	}
	domain, err := h.store.GetManagedDomain(spec.Endpoint.DomainID)
	if err != nil || !domain.Enabled {
		return fmt.Errorf("域名不存在或已停用")
	}
	spec.Endpoint.Domain = domain.Hostname
	if spec.Endpoint.IssuerRef == "" {
		spec.Endpoint.IssuerRef = domain.IssuerRef
	}
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
