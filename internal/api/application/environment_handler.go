package applicationapi

import (
	"context"
	"errors"
	"fmt"
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
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	NamespaceMode string `json:"namespace_mode"`
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
		conflictNamespaces[conflict.Namespace] = struct{}{}
	}
	for index := range environments {
		environments[index].NamespaceStatus = environmentNamespaceStatus(c.Request.Context(), h.namespaces(), environments[index].Namespace)
		_, environments[index].NamespaceConflict = conflictNamespaces[environments[index].Namespace]
	}
	apiShared.Success(c, apiShared.EnvironmentsDTO(environments))
}

func (h *ApplicationHandler) CreateEnvironment(c *gin.Context) {
	projectID, err := apiShared.ParseID(c.Param("projectID"))
	if err != nil {
		apiShared.BadRequest(c, "项目 ID 无效")
		return
	}
	if _, err := h.queries.GetProject(projectID); err != nil {
		apiShared.NotFound(c, "项目不存在")
		return
	}
	var req environmentRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Namespace) == "" {
		apiShared.BadRequest(c, "环境名称和命名空间必填")
		return
	}
	mode, err := normalizeEnvironmentNamespaceMode(req.NamespaceMode)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if h.kubernetes == nil || !h.kubernetes.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
	name, namespace := strings.TrimSpace(req.Name), strings.TrimSpace(req.Namespace)
	if err := h.applications.EnsureNamespaceAvailable(namespace, 0); err != nil {
		handleEnvironmentNamespaceError(c, err)
		return
	}
	if err := ensureEnvironmentNamespace(c.Request.Context(), h.namespaces(), projectID, namespace, mode, false); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	environment := &model.Environment{ProjectID: projectID, Name: name, Namespace: namespace, NamespaceStatus: "active"}
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
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Namespace) == "" {
		apiShared.BadRequest(c, "环境名称和命名空间必填")
		return
	}
	environment, err := h.queries.ResolveEnvironment(projectID, environmentID)
	if err != nil {
		apiShared.NotFound(c, "环境不存在")
		return
	}
	name, namespace := strings.TrimSpace(req.Name), strings.TrimSpace(req.Namespace)
	if environment.Name != name || environment.Namespace != namespace {
		applicationCount, err := h.applications.CountEnvironmentApplications(environmentID)
		if err != nil {
			apiShared.DBError(c, err.Error())
			return
		}
		namespaceConflict, err := h.applications.IsEnvironmentNamespaceConflicted(environment.Namespace)
		if err != nil {
			apiShared.DBError(c, err.Error())
			return
		}
		if applicationCount > 0 && !namespaceConflict {
			apiShared.Conflict(c, "环境已有应用，不能修改名称或命名空间")
			return
		}
		if environment.Namespace != namespace {
			mode, err := normalizeEnvironmentNamespaceMode(req.NamespaceMode)
			if err != nil {
				apiShared.ValidationError(c, err.Error())
				return
			}
			if h.kubernetes == nil || !h.kubernetes.KubernetesAvailable() {
				apiShared.K8sUnavailable(c)
				return
			}
			if err := h.applications.EnsureNamespaceAvailable(namespace, environment.ID); err != nil {
				handleEnvironmentNamespaceError(c, err)
				return
			}
			if err := ensureEnvironmentNamespace(c.Request.Context(), h.namespaces(), projectID, namespace, mode, false); err != nil {
				apiShared.ValidationError(c, err.Error())
				return
			}
		}
	}
	environment.Name = name
	environment.Namespace = namespace
	environment.NamespaceStatus = environmentNamespaceStatus(c.Request.Context(), h.namespaces(), namespace)
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
	if h.kubernetes == nil || !h.kubernetes.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
	if err := h.applications.EnsureNamespaceAvailable(environment.Namespace, environment.ID); err != nil {
		handleEnvironmentNamespaceError(c, err)
		return
	}
	if err := ensureEnvironmentNamespace(c.Request.Context(), h.namespaces(), projectID, environment.Namespace, "create", true); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	environment.NamespaceStatus = environmentNamespaceStatus(c.Request.Context(), h.namespaces(), environment.Namespace)
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

func ensureEnvironmentNamespace(ctx context.Context, client NamespaceClient, projectID uint, namespace, mode string, allowExisting bool) error {
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
			if _, err := client.UpdateNamespace(ctx, existing); err != nil {
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
