package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
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
	projectID, err := apiShared.ParseID(c.Param("projectID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "项目 ID 无效")
		return
	}
	if _, err := h.queries.GetProject(projectID); err != nil {
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
		apiShared.K8sUnavailable(c)
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
	environment, err := h.queries.ResolveEnvironment(projectID, environmentID)
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
				apiShared.K8sUnavailable(c)
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
	environment, err := h.queries.ResolveEnvironment(projectID, environmentID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "环境不存在")
		return
	}
	if K8s == nil || K8s.Clientset == nil {
		apiShared.K8sUnavailable(c)
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
	if _, err := h.queries.ResolveEnvironment(projectID, environmentID); err != nil {
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
	projectID, err := apiShared.ParseID(c.Param("projectID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "项目 ID 无效")
		return 0, 0, false
	}
	environmentID, err := apiShared.ParseID(c.Param("environmentID"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "环境 ID 无效")
		return 0, 0, false
	}
	return projectID, environmentID, true
}
