package applicationapi

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type environmentRequest struct {
	Name      string `json:"name"`
	ClusterID uint   `json:"cluster_id"`
}

var namespaceSegmentPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

func generatedEnvironmentNamespace(project *model.Project, name string) (string, error) {
	if project.Slug == "" {
		return "", fmt.Errorf("请先编辑项目，设置项目英文名")
	}
	if !namespaceSegmentPattern.MatchString(name) {
		return "", fmt.Errorf("环境名称只能使用小写英文字母、数字和连字符，且不能以连字符开头或结尾")
	}
	namespace := "project-" + project.Slug + "-" + name
	if len(namespace) > 63 {
		return "", fmt.Errorf("项目英文名和环境名称组合后不能超过 63 位")
	}
	return namespace, nil
}

func (h *ApplicationHandler) ListEnvironments(c *gin.Context) {
	projectID, err := apiShared.ParseID(c.Param("projectID"))
	if err != nil {
		apiShared.BadRequest(c, "项目 ID 无效")
		return
	}
	environments, err := h.applications.ListEnvironments(projectID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	conflicts, err := h.applications.ListEnvironmentNamespaceConflicts()
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	conflictNamespaces := make(map[string]struct{}, len(conflicts))
	for _, conflict := range conflicts {
		conflictNamespaces[fmt.Sprintf("%d/%s", conflict.ClusterID, conflict.Namespace)] = struct{}{}
	}
	for index := range environments {
		scoped, scopeErr := h.forCluster(c.Request.Context(), environments[index].ClusterID)
		if scopeErr == nil {
			environments[index].NamespaceStatus = environmentNamespaceStatus(c.Request.Context(), scoped.namespaces(), environments[index].Namespace)
		} else {
			environments[index].NamespaceStatus = "unknown"
		}
		_, environments[index].NamespaceConflict = conflictNamespaces[fmt.Sprintf("%d/%s", environments[index].ClusterID, environments[index].Namespace)]
	}
	apiShared.Success(c, apiShared.EnvironmentsDTO(environments))
}

func (h *ApplicationHandler) CreateEnvironment(c *gin.Context) {
	projectID, err := apiShared.ParseID(c.Param("projectID"))
	if err != nil {
		apiShared.BadRequest(c, "项目 ID 无效")
		return
	}
	project, err := h.queries.GetProject(projectID)
	if err != nil {
		apiShared.NotFound(c, "项目不存在")
		return
	}
	var req environmentRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		apiShared.BadRequest(c, "环境名称必填")
		return
	}
	name := strings.TrimSpace(req.Name)
	namespace, err := generatedEnvironmentNamespace(project, name)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	scoped, scopeErr := h.forCluster(c.Request.Context(), req.ClusterID)
	if scopeErr != nil {
		apiShared.ValidationError(c, scopeErr.Error())
		return
	}
	if scoped.kubernetes == nil || !scoped.kubernetes.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
	if err := h.applications.EnsureNamespaceAvailable(namespace, 0, req.ClusterID); err != nil {
		handleEnvironmentNamespaceError(c, err)
		return
	}
	if err := ensureEnvironmentNamespace(c.Request.Context(), scoped.namespaces(), projectID, namespace, false); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	environment := &model.Environment{ProjectID: projectID, ClusterID: req.ClusterID, Name: name, Namespace: namespace, NamespaceStatus: "active"}
	if err := h.applications.CreateEnvironment(environment); err != nil {
		if handleEnvironmentNamespaceError(c, err) {
			return
		}
		apiShared.Conflict(c, "环境名称已存在")
		return
	}
	apiShared.Success(c, apiShared.EnvironmentDTO(environment))
}

func (h *ApplicationHandler) UpdateEnvironment(c *gin.Context) {
	projectID, environmentID, ok := h.environmentRouteIDs(c)
	if !ok {
		return
	}
	var req environmentRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		apiShared.BadRequest(c, "环境名称必填")
		return
	}
	environment, err := h.queries.ResolveEnvironment(projectID, environmentID)
	if err != nil {
		apiShared.NotFound(c, "环境不存在")
		return
	}
	name, namespace := strings.TrimSpace(req.Name), environment.Namespace
	if name != environment.Name {
		project, projectErr := h.queries.GetProject(projectID)
		if projectErr != nil {
			apiShared.NotFound(c, "项目不存在")
			return
		}
		namespace, err = generatedEnvironmentNamespace(project, name)
		if err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
	}
	clusterID := req.ClusterID
	if clusterID == 0 {
		clusterID = environment.ClusterID
	}
	if clusterID == 0 && h.clusterClient != nil {
		apiShared.ValidationError(c, "请先为环境选择集群")
		return
	}
	if clusterID != environment.ClusterID && environment.ClusterID != 0 {
		apiShared.Conflict(c, "环境已绑定集群，不能直接变更部署目标，请新建环境")
		return
	}
	scoped, scopeErr := h.forCluster(c.Request.Context(), clusterID)
	if scopeErr != nil {
		apiShared.ValidationError(c, scopeErr.Error())
		return
	}
	if environment.Name != name || environment.Namespace != namespace || environment.ClusterID != clusterID {
		applicationCount, err := h.applications.CountEnvironmentApplications(environmentID)
		if err != nil {
			apiShared.DBError(c, err.Error())
			return
		}
		if applicationCount > 0 && (environment.Name != name || environment.Namespace != namespace) {
			apiShared.Conflict(c, "环境已有应用，不能修改名称或命名空间")
			return
		}
		if environment.Namespace != namespace || environment.ClusterID != clusterID {
			if scoped.kubernetes == nil || !scoped.kubernetes.KubernetesAvailable() {
				apiShared.K8sUnavailable(c)
				return
			}
			if err := h.applications.EnsureNamespaceAvailable(namespace, environment.ID, clusterID); err != nil {
				handleEnvironmentNamespaceError(c, err)
				return
			}
			if err := ensureEnvironmentNamespace(c.Request.Context(), scoped.namespaces(), projectID, namespace, false); err != nil {
				apiShared.ValidationError(c, err.Error())
				return
			}
		}
	}
	environment.Name = name
	environment.ClusterID = clusterID
	environment.Namespace = namespace
	environment.NamespaceStatus = environmentNamespaceStatus(c.Request.Context(), scoped.namespaces(), namespace)
	if err := h.applications.UpdateEnvironment(environment); err != nil {
		if handleEnvironmentNamespaceError(c, err) {
			return
		}
		apiShared.Conflict(c, "环境名称已存在")
		return
	}
	apiShared.Success(c, apiShared.EnvironmentDTO(environment))
}

func (h *ApplicationHandler) SyncEnvironmentNamespace(c *gin.Context) {
	projectID, environmentID, ok := h.environmentRouteIDs(c)
	if !ok {
		return
	}
	environment, err := h.queries.ResolveEnvironment(projectID, environmentID)
	if err != nil {
		apiShared.NotFound(c, "环境不存在")
		return
	}
	scoped, scopeErr := h.forCluster(c.Request.Context(), environment.ClusterID)
	if scopeErr != nil {
		apiShared.ValidationError(c, scopeErr.Error())
		return
	}
	if scoped.kubernetes == nil || !scoped.kubernetes.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
	if err := h.applications.EnsureNamespaceAvailable(environment.Namespace, environment.ID, environment.ClusterID); err != nil {
		handleEnvironmentNamespaceError(c, err)
		return
	}
	if err := ensureEnvironmentNamespace(c.Request.Context(), scoped.namespaces(), projectID, environment.Namespace, true); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	environment.NamespaceStatus = environmentNamespaceStatus(c.Request.Context(), scoped.namespaces(), environment.Namespace)
	apiShared.SuccessWithMessage(c, apiShared.EnvironmentDTO(environment), "命名空间已同步")
}

func (h *ApplicationHandler) ListEnvironmentNamespaceConflicts(c *gin.Context) {
	conflicts, err := h.applications.ListEnvironmentNamespaceConflicts()
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	apiShared.Success(c, conflicts)
}

func (h *ApplicationHandler) DeleteEnvironment(c *gin.Context) {
	projectID, environmentID, ok := h.environmentRouteIDs(c)
	if !ok {
		return
	}
	if _, err := h.queries.ResolveEnvironment(projectID, environmentID); err != nil {
		apiShared.NotFound(c, "环境不存在")
		return
	}
	applicationCount, err := h.applications.CountEnvironmentApplications(environmentID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if applicationCount > 0 {
		apiShared.Conflict(c, "环境仍关联应用，无法删除")
		return
	}
	if err := h.applications.DeleteEnvironment(environmentID); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	apiShared.Success(c, gin.H{"id": environmentID})
}

func ensureEnvironmentNamespace(ctx context.Context, client NamespaceClient, projectID uint, namespace string, allowExisting bool) error {
	if isSystemNamespace(namespace) {
		return fmt.Errorf("系统命名空间 %q 不能绑定为应用环境", namespace)
	}
	if client == nil {
		return fmt.Errorf("K8s 集群未连接")
	}
	existing, err := client.GetNamespace(ctx, namespace)
	if err == nil {
		if existing.Status.Phase != "" && existing.Status.Phase != corev1.NamespaceActive {
			return fmt.Errorf("命名空间 %q 未就绪", namespace)
		}
		labels := existing.GetLabels()
		owner := strings.TrimSpace(labels["cylism.io/project-id"])
		project := strconv.FormatUint(uint64(projectID), 10)
		if !allowExisting && owner != project {
			return fmt.Errorf("命名空间 %q 已存在，不能自动创建", namespace)
		}
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
			if _, err := client.UpdateNamespace(ctx, existing); err != nil {
				return fmt.Errorf("认领命名空间 %q: %w", namespace, err)
			}
		}
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("检查命名空间: %w", err)
	}
	_, err = client.CreateNamespace(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace, Labels: map[string]string{
		"app.kubernetes.io/managed-by": "cylism-manager",
		"cylism.io/project-id":         strconv.FormatUint(uint64(projectID), 10),
	}}})
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
	var conflict *model.NamespaceConflictError
	if errors.As(err, &conflict) {
		apiShared.Conflict(c, conflict.Error())
		return true
	}
	return false
}

func environmentNamespaceStatus(ctx context.Context, client NamespaceClient, namespace string) string {
	if client == nil {
		return "unknown"
	}
	resource, err := client.GetNamespace(ctx, namespace)
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
	projectID, err := apiShared.ParseID(c.Param("projectID"))
	if err != nil {
		apiShared.BadRequest(c, "项目 ID 无效")
		return 0, 0, false
	}
	environmentID, err := apiShared.ParseID(c.Param("environmentID"))
	if err != nil {
		apiShared.BadRequest(c, "环境 ID 无效")
		return 0, 0, false
	}
	return projectID, environmentID, true
}
