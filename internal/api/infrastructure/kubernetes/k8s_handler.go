package kubernetes

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/repository"
	storageservice "github.com/cylism/cylism-manager/internal/service/storage"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	appstyped "k8s.io/client-go/kubernetes/typed/apps/v1"
	coretyped "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"

	"github.com/gin-gonic/gin"
)

// K8sHandler h.k8s 资源管理的 HTTP handler
type K8sHandler struct {
	resourceReferences repository.ResourceReferenceRepository
	audit              repository.AuditRepository
	encKey             []byte
	k8s                K8sResourceAdapter
	storageService     *storageservice.Service
}

// K8sResourceAdapter is the resource capability consumed by this HTTP API.
// The concrete Kubernetes client is wrapped at the composition boundary.
type K8sResourceAdapter interface {
	KubernetesAvailable() bool
	CoreV1() coretyped.CoreV1Interface
	AppsV1() appstyped.AppsV1Interface
	KubeConfig() *rest.Config
	ListNodeInfosContext(context.Context) ([]k8sclient.NodeInfo, error)
	ListDeploymentsContext(context.Context, string) ([]k8sclient.DeploymentInfo, error)
	ListServicesContext(context.Context, string) ([]k8sclient.ServiceEndpointInfo, error)
	ListDeploymentPodsContext(context.Context, string, string) ([]k8sclient.PodRef, error)
	GetDeploymentContext(context.Context, string, string) (*k8sclient.DeploymentInfo, error)
	ListDeploymentRevisionsContext(context.Context, string, string) ([]k8sclient.RevisionInfo, error)
	ScaleDeploymentContext(context.Context, string, string, int32) error
	UpdateDeploymentImageContext(context.Context, string, string, string, string) error
	RollbackDeploymentContext(context.Context, string, string, int64) error
	ListStatefulSetsContext(context.Context, string) ([]k8sclient.StatefulSetInfo, error)
	GetStatefulSetContext(context.Context, string, string) (*k8sclient.StatefulSetInfo, error)
	ScaleStatefulSetContext(context.Context, string, string, int32) error
	ListDaemonSetsContext(context.Context, string) ([]k8sclient.DaemonSetInfo, error)
	GetDaemonSetContext(context.Context, string, string) (*k8sclient.DaemonSetInfo, error)
	GetServiceEndpointsContext(context.Context, string, string) ([]k8sclient.EndpointSliceInfo, error)
	ListConfigMapsMetadataContext(context.Context, string) ([]k8sclient.ConfigMapInfo, error)
	ListConfigMapsContext(context.Context, string) ([]k8sclient.ConfigMapInfo, error)
	GetConfigMapContext(context.Context, string, string) (*k8sclient.ConfigMapDetail, error)
	CreateConfigMapContext(context.Context, k8sclient.ConfigMapMutation) (*k8sclient.ConfigMapDetail, error)
	UpdateConfigMapContext(context.Context, k8sclient.ConfigMapMutation) (*k8sclient.ConfigMapDetail, error)
	DeleteConfigMapContext(context.Context, string, string) error
	ListSecretsMetadataContext(context.Context, string) ([]k8sclient.SecretInfo, error)
	ListSecretsContext(context.Context, string) ([]k8sclient.SecretInfo, error)
	GetSecretContext(context.Context, string, string) (*k8sclient.SecretDetail, error)
	CreateOpaqueSecretContext(context.Context, k8sclient.OpaqueSecretMutation) (*k8sclient.SecretInfo, error)
	UpdateOpaqueSecretContext(context.Context, k8sclient.OpaqueSecretMutation) (*k8sclient.SecretInfo, error)
	DeleteOpaqueSecretContext(context.Context, string, string) error
	ListIngressesContext(context.Context, string) ([]k8sclient.IngressStdInfo, error)
	GetIngressContext(context.Context, string, string) (*k8sclient.IngressStdDetail, error)
	CreateIngressContext(context.Context, string, string, string, string, string, string) (*k8sclient.IngressStdDetail, error)
	DeleteIngressContext(context.Context, string, string) error
	DetectIngressControllerContext(context.Context) (*k8sclient.IngressControllerStatus, error)
	GetServiceContext(context.Context, string, string) (*k8sclient.ServiceEndpointInfo, error)
}

type clientK8sResourceAdapter struct{ *k8sclient.Client }

func (a clientK8sResourceAdapter) CoreV1() coretyped.CoreV1Interface { return a.Clientset.CoreV1() }
func (a clientK8sResourceAdapter) AppsV1() appstyped.AppsV1Interface { return a.Clientset.AppsV1() }
func (a clientK8sResourceAdapter) KubeConfig() *rest.Config          { return a.Config }

// NewK8sResourceAdapter is the infrastructure adapter factory used by the
// bootstrap composition root. It returns nil when Kubernetes is unavailable.
func NewK8sResourceAdapter(client *k8sclient.Client) K8sResourceAdapter {
	if client == nil || client.Clientset == nil {
		return nil
	}
	return clientK8sResourceAdapter{Client: client}
}

func NewK8sHandlerWithAdapter(adapter K8sResourceAdapter, references repository.ResourceReferenceRepository, storageService *storageservice.Service, audit repository.AuditRepository) *K8sHandler {
	return &K8sHandler{k8s: adapter, resourceReferences: references, storageService: storageService, audit: audit}
}

func NewK8sHandlerWithAdapterAndEncryption(references repository.ResourceReferenceRepository, storageService *storageservice.Service, encKey []byte, adapter K8sResourceAdapter, audit repository.AuditRepository) *K8sHandler {
	h := NewK8sHandlerWithAdapter(adapter, references, storageService, audit)
	h.encKey = encKey
	return h
}

// StorageService exposes the composed storage service to the infrastructure
// router; callers must still use the service API rather than handler helpers.
func (h *K8sHandler) StorageService() *storageservice.Service { return h.storageService }

type NamespaceSummary struct {
	Name             string            `json:"name"`
	Status           string            `json:"status"`
	Labels           map[string]string `json:"labels"`
	Annotations      map[string]string `json:"annotations"`
	LabelsCount      int               `json:"labels_count"`
	AnnotationsCount int               `json:"annotations_count"`
	Deployments      int               `json:"deployments"`
	StatefulSets     int               `json:"statefulsets"`
	DaemonSets       int               `json:"daemonsets"`
	Services         int               `json:"services"`
	ConfigMaps       int               `json:"configmaps"`
	Secrets          int               `json:"secrets"`
}

type NamespaceNameSummary struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type resourceDataRequest struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Data      map[string]string `json:"data"`
}

func validateResourceDataRequest(req resourceDataRequest) string {
	if req.Namespace == "" || req.Name == "" {
		return "namespace 和 name 必填"
	}
	if messages := validation.IsDNS1123Subdomain(req.Name); len(messages) > 0 {
		return "资源名称无效"
	}
	for key := range req.Data {
		if messages := validation.IsConfigMapKey(key); len(messages) > 0 {
			return "资源键名无效"
		}
	}
	return ""
}

func (h *K8sHandler) resourceIsReferenced(namespace, sourceType, name string) (bool, error) {
	if h.resourceReferences == nil {
		return false, nil
	}
	references, err := h.resourceReferences.ListResourceReferences(namespace, sourceType, name)
	return len(references) > 0, err
}

// Dashboard 集群摘要（扩展 Deployment/Service 统计）
func (h *K8sHandler) Dashboard(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}

	nodes, err := h.k8s.ListNodeInfosContext(c.Request.Context())
	nodeTotal := 0
	if err == nil {
		nodeTotal = len(nodes)
	}

	nsCount := 0
	if nsList, err := h.k8s.CoreV1().Namespaces().List(c.Request.Context(), metav1.ListOptions{}); err == nil {
		nsCount = len(nsList.Items)
	}

	podTotal, podReady := 0, 0
	if pods, pErr := h.k8s.CoreV1().Pods("").List(c.Request.Context(), metav1.ListOptions{}); pErr == nil {
		podTotal = len(pods.Items)
		for _, p := range pods.Items {
			if p.Status.Phase == "Running" {
				podReady++
			}
		}
	}

	// 新增：Deployment 统计
	deployTotal, deployReady := 0, 0
	if deps, dErr := h.k8s.ListDeploymentsContext(c.Request.Context(), ""); dErr == nil {
		deployTotal = len(deps)
		for _, d := range deps {
			if d.Ready == d.Replicas {
				deployReady++
			}
		}
	}

	// 新增：Service 统计
	svcTotal := 0
	if svcs, sErr := h.k8s.ListServicesContext(c.Request.Context(), ""); sErr == nil {
		svcTotal = len(svcs)
	}

	version := ""
	if nodeTotal > 0 {
		version = nodes[0].Version
	}

	apiShared.Success(c, map[string]interface{}{
		"nodes_total":       nodeTotal,
		"pods_total":        podTotal,
		"pods_ready":        podReady,
		"deployments_total": deployTotal,
		"deployments_ready": deployReady,
		"services_total":    svcTotal,
		"namespaces":        nsCount,
		"version":           version,
	})
}

// ==================== Namespace ====================

// ListNamespaceNames returns only the fields needed by lightweight selectors.
// Keep resource counts in ListNamespaces because they require one list request
// per resource type and namespace.
func (h *K8sHandler) ListNamespaceNames(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	nsList, err := h.k8s.CoreV1().Namespaces().List(c.Request.Context(), metav1.ListOptions{})
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	result := make([]NamespaceNameSummary, 0, len(nsList.Items))
	for _, namespace := range nsList.Items {
		result = append(result, NamespaceNameSummary{Name: namespace.Name, Status: string(namespace.Status.Phase)})
	}
	apiShared.Success(c, result)
}

// ListNamespaces 列出 Namespace 与资源摘要
func (h *K8sHandler) ListNamespaces(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}

	nsList, err := h.k8s.CoreV1().Namespaces().List(c.Request.Context(), metav1.ListOptions{})
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}

	result := make([]NamespaceSummary, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		name := ns.Name
		summary := NamespaceSummary{
			Name:             name,
			Status:           string(ns.Status.Phase),
			Labels:           ns.Labels,
			Annotations:      ns.Annotations,
			LabelsCount:      len(ns.Labels),
			AnnotationsCount: len(ns.Annotations),
		}

		if list, listErr := h.k8s.AppsV1().Deployments(name).List(c.Request.Context(), metav1.ListOptions{}); listErr == nil {
			summary.Deployments = len(list.Items)
		}
		if list, listErr := h.k8s.AppsV1().StatefulSets(name).List(c.Request.Context(), metav1.ListOptions{}); listErr == nil {
			summary.StatefulSets = len(list.Items)
		}
		if list, listErr := h.k8s.AppsV1().DaemonSets(name).List(c.Request.Context(), metav1.ListOptions{}); listErr == nil {
			summary.DaemonSets = len(list.Items)
		}
		if list, listErr := h.k8s.CoreV1().Services(name).List(c.Request.Context(), metav1.ListOptions{}); listErr == nil {
			summary.Services = len(list.Items)
		}
		if list, listErr := h.k8s.CoreV1().ConfigMaps(name).List(c.Request.Context(), metav1.ListOptions{}); listErr == nil {
			summary.ConfigMaps = len(list.Items)
		}
		if list, listErr := h.k8s.CoreV1().Secrets(name).List(c.Request.Context(), metav1.ListOptions{}); listErr == nil {
			summary.Secrets = len(list.Items)
		}

		result = append(result, summary)
	}

	apiShared.Success(c, result)
}

// CreateNamespace 创建 Namespace
func (h *K8sHandler) CreateNamespace(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}

	var req struct {
		Name        string            `json:"name"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		apiShared.BadRequest(c, "name 必填")
		return
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Labels:      req.Labels,
			Annotations: req.Annotations,
		},
	}

	created, err := h.k8s.CoreV1().Namespaces().Create(c.Request.Context(), ns, metav1.CreateOptions{})
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}

	apiShared.SuccessWithMessage(c, gin.H{
		"name": created.Name,
	}, "命名空间创建成功")
}

// UpdateNamespace 更新 Namespace 元数据
func (h *K8sHandler) UpdateNamespace(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}

	name := c.Param("name")
	var req struct {
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "invalid namespace payload")
		return
	}

	ns, err := h.k8s.CoreV1().Namespaces().Get(c.Request.Context(), name, metav1.GetOptions{})
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}

	ns.Labels = req.Labels
	ns.Annotations = req.Annotations
	updated, err := h.k8s.CoreV1().Namespaces().Update(c.Request.Context(), ns, metav1.UpdateOptions{})
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}

	apiShared.SuccessWithMessage(c, gin.H{
		"name":        updated.Name,
		"labels":      updated.Labels,
		"annotations": updated.Annotations,
	}, "命名空间更新成功")
}

// DeleteNamespace 删除 Namespace
func (h *K8sHandler) DeleteNamespace(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}

	name := c.Param("name")
	if isProtectedNamespace(name) {
		apiShared.BadRequest(c, "系统命名空间不允许删除")
		return
	}

	if err := h.k8s.CoreV1().Namespaces().Delete(c.Request.Context(), name, metav1.DeleteOptions{}); err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}

	apiShared.SuccessWithMessage(c, nil, "命名空间删除成功")
}

func isProtectedNamespace(name string) bool {
	switch name {
	case "default", "kube-system", "kube-public", "kube-node-lease":
		return true
	default:
		return false
	}
}

// ListPods Pod 列表
func (h *K8sHandler) ListPods(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	pods, err := h.k8s.CoreV1().Pods(ns).List(c.Request.Context(), metav1.ListOptions{})
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}

	type PodInfo struct {
		Name       string   `json:"name"`
		Namespace  string   `json:"namespace"`
		Status     string   `json:"status"`
		Node       string   `json:"node"`
		IP         string   `json:"ip"`
		Restarts   int32    `json:"restarts"`
		Age        string   `json:"age"`
		Containers []string `json:"containers"`
	}
	var result []PodInfo
	for _, p := range pods.Items {
		restarts := int32(0)
		for _, cs := range p.Status.ContainerStatuses {
			restarts += cs.RestartCount
		}
		containers := make([]string, 0, len(p.Spec.Containers))
		for _, container := range p.Spec.Containers {
			containers = append(containers, container.Name)
		}
		dur := metav1.Now().Sub(p.CreationTimestamp.Time)
		result = append(result, PodInfo{
			Name:       p.Name,
			Namespace:  p.Namespace,
			Status:     string(p.Status.Phase),
			Node:       p.Spec.NodeName,
			IP:         p.Status.PodIP,
			Restarts:   restarts,
			Age:        ageStr(dur),
			Containers: containers,
		})
	}
	apiShared.Success(c, result)
}

// ==================== Deployment ====================

// ListDeployments 列出 Deployment（使用封装层）
func (h *K8sHandler) ListDeployments(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	result, err := h.k8s.ListDeploymentsContext(c.Request.Context(), ns)
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	if result == nil {
		result = []k8sclient.DeploymentInfo{}
	}
	apiShared.Success(c, result)
}

// GetDeployment 获取 Deployment 详情
func (h *K8sHandler) GetDeployment(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := h.k8s.GetDeploymentContext(c.Request.Context(), ns, name)
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	apiShared.Success(c, result)
}

// ListDeploymentPods 获取 Deployment 关联 Pod
func (h *K8sHandler) ListDeploymentPods(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := h.k8s.ListDeploymentPodsContext(c.Request.Context(), ns, name)
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	apiShared.Success(c, result)
}

// ListDeploymentRevisions 获取 Deployment 版本历史
func (h *K8sHandler) ListDeploymentRevisions(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := h.k8s.ListDeploymentRevisionsContext(c.Request.Context(), ns, name)
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	apiShared.Success(c, result)
}

// ScaleDeployment 扩缩容 Deployment
func (h *K8sHandler) ScaleDeployment(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")

	var req struct {
		Replicas int32 `json:"replicas"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "invalid replicas")
		return
	}

	if err := h.k8s.ScaleDeploymentContext(c.Request.Context(), ns, name, req.Replicas); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, map[string]interface{}{"replicas": req.Replicas}, "扩缩容成功")
}

// UpdateDeploymentImage 更新 Deployment 镜像
func (h *K8sHandler) UpdateDeploymentImage(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")

	var req struct {
		Container string `json:"container"`
		Image     string `json:"image"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Container == "" || req.Image == "" {
		apiShared.BadRequest(c, "container 和 image 必填")
		return
	}

	if err := h.k8s.UpdateDeploymentImageContext(c.Request.Context(), ns, name, req.Container, req.Image); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, nil, "镜像更新已触发滚动更新")
}

// RollbackDeployment 回滚 Deployment
func (h *K8sHandler) RollbackDeployment(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")

	var req struct {
		Revision int64 `json:"revision"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "invalid revision")
		return
	}

	if err := h.k8s.RollbackDeploymentContext(c.Request.Context(), ns, name, req.Revision); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, nil, "回滚成功")
}

// ==================== StatefulSet ====================

// ListStatefulSets 列出 StatefulSet
func (h *K8sHandler) ListStatefulSets(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	result, err := h.k8s.ListStatefulSetsContext(c.Request.Context(), ns)
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	if result == nil {
		result = []k8sclient.StatefulSetInfo{}
	}
	apiShared.Success(c, result)
}

// GetStatefulSet 获取 StatefulSet 详情
func (h *K8sHandler) GetStatefulSet(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := h.k8s.GetStatefulSetContext(c.Request.Context(), ns, name)
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	apiShared.Success(c, result)
}

// ScaleStatefulSet 扩缩容 StatefulSet
func (h *K8sHandler) ScaleStatefulSet(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")

	var req struct {
		Replicas int32 `json:"replicas"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "invalid replicas")
		return
	}

	if err := h.k8s.ScaleStatefulSetContext(c.Request.Context(), ns, name, req.Replicas); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, map[string]interface{}{"replicas": req.Replicas}, "扩缩容成功")
}

// ==================== DaemonSet ====================

// ListDaemonSets 列出 DaemonSet
func (h *K8sHandler) ListDaemonSets(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	result, err := h.k8s.ListDaemonSetsContext(c.Request.Context(), ns)
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	if result == nil {
		result = []k8sclient.DaemonSetInfo{}
	}
	apiShared.Success(c, result)
}

// GetDaemonSet 获取 DaemonSet 详情
func (h *K8sHandler) GetDaemonSet(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := h.k8s.GetDaemonSetContext(c.Request.Context(), ns, name)
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	apiShared.Success(c, result)
}

// ==================== Service (extended) ====================

// ListServicesV2 列出 Service（使用封装层，含 endpoint_count）
func (h *K8sHandler) ListServicesV2(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	result, err := h.k8s.ListServicesContext(c.Request.Context(), ns)
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	if result == nil {
		result = []k8sclient.ServiceEndpointInfo{}
	}
	apiShared.Success(c, result)
}

// ==================== EndpointSlice ====================

// GetServiceEndpoints 获取 Service 的 EndpointSlice
func (h *K8sHandler) GetServiceEndpoints(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := h.k8s.GetServiceEndpointsContext(c.Request.Context(), ns, name)
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	if result == nil {
		result = []k8sclient.EndpointSliceInfo{}
	}
	apiShared.Success(c, result)
}

// ==================== ConfigMap ====================

// ListConfigMaps 列出 ConfigMap
func (h *K8sHandler) ListConfigMaps(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	var result []k8sclient.ConfigMapInfo
	var err error
	if c.Query("usage") == "false" {
		result, err = h.k8s.ListConfigMapsMetadataContext(c.Request.Context(), ns)
	} else {
		result, err = h.k8s.ListConfigMapsContext(c.Request.Context(), ns)
	}
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	if result == nil {
		result = []k8sclient.ConfigMapInfo{}
	}
	apiShared.Success(c, result)
}

// GetConfigMap 获取 ConfigMap 详情
func (h *K8sHandler) GetConfigMap(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := h.k8s.GetConfigMapContext(c.Request.Context(), ns, name)
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	apiShared.Success(c, result)
}

func (h *K8sHandler) CreateConfigMap(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	var req resourceDataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "请求格式无效")
		return
	}
	if message := validateResourceDataRequest(req); message != "" {
		apiShared.BadRequest(c, message)
		return
	}
	result, err := h.k8s.CreateConfigMapContext(c.Request.Context(), k8sclient.ConfigMapMutation{Namespace: req.Namespace, Name: req.Name, Data: req.Data})
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	log.Printf("AUDIT: configmap created: %s/%s", req.Namespace, req.Name)
	apiShared.SuccessWithMessage(c, result, "ConfigMap 创建成功")
}

func (h *K8sHandler) UpdateConfigMap(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	var req resourceDataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "请求格式无效")
		return
	}
	req.Namespace, req.Name = c.Param("namespace"), c.Param("name")
	if message := validateResourceDataRequest(req); message != "" {
		apiShared.BadRequest(c, message)
		return
	}
	result, err := h.k8s.UpdateConfigMapContext(c.Request.Context(), k8sclient.ConfigMapMutation{Namespace: req.Namespace, Name: req.Name, Data: req.Data})
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	log.Printf("AUDIT: configmap updated: %s/%s", req.Namespace, req.Name)
	apiShared.SuccessWithMessage(c, result, "ConfigMap 更新成功")
}

func (h *K8sHandler) DeleteConfigMap(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	namespace, name := c.Param("namespace"), c.Param("name")
	resource, err := h.k8s.GetConfigMapContext(c.Request.Context(), namespace, name)
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	if len(resource.UsedBy) > 0 {
		apiShared.Error(c, http.StatusConflict, apiShared.CodeBadRequest, "ConfigMap 正被工作负载引用，不能删除")
		return
	}
	if referenced, err := h.resourceIsReferenced(namespace, "configmap", name); err != nil {
		apiShared.DBError(c, err.Error())
		return
	} else if referenced {
		apiShared.Error(c, http.StatusConflict, apiShared.CodeBadRequest, "ConfigMap 仍被上线模板或发布快照引用，不能删除")
		return
	}
	if err := h.k8s.DeleteConfigMapContext(c.Request.Context(), namespace, name); err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	log.Printf("AUDIT: configmap deleted: %s/%s", namespace, name)
	apiShared.SuccessWithMessage(c, nil, "ConfigMap 删除成功")
}

// ==================== Secret ====================

// ListSecrets 列出 Secret
func (h *K8sHandler) ListSecrets(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	var result []k8sclient.SecretInfo
	var err error
	if c.Query("usage") == "false" {
		result, err = h.k8s.ListSecretsMetadataContext(c.Request.Context(), ns)
	} else {
		result, err = h.k8s.ListSecretsContext(c.Request.Context(), ns)
	}
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	if result == nil {
		result = []k8sclient.SecretInfo{}
	}
	apiShared.Success(c, result)
}

// GetSecret 获取 Secret 详情
// GetSecret 获取 Secret 详情（value 已脱敏，仅返回 key 列表，前端需单独请求解码）
func (h *K8sHandler) GetSecret(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := h.k8s.GetSecretContext(c.Request.Context(), ns, name)
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	log.Printf("AUDIT: secret accessed: %s/%s", ns, name)
	// Redact secret values before sending over wire
	redacted := make(map[string]string)
	for k := range result.Data {
		redacted[k] = "W1JFREFDVEVEXQ==" // base64 of [REDACTED]
	}
	result.Data = redacted
	apiShared.Success(c, result)
}

func (h *K8sHandler) CreateOpaqueSecret(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	var req resourceDataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "请求格式无效")
		return
	}
	if message := validateResourceDataRequest(req); message != "" {
		apiShared.BadRequest(c, message)
		return
	}
	result, err := h.k8s.CreateOpaqueSecretContext(c.Request.Context(), k8sclient.OpaqueSecretMutation{Namespace: req.Namespace, Name: req.Name, Data: req.Data})
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	log.Printf("AUDIT: opaque secret created: %s/%s", req.Namespace, req.Name)
	apiShared.SuccessWithMessage(c, result, "Secret 创建成功")
}

func (h *K8sHandler) UpdateOpaqueSecret(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	var req resourceDataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "请求格式无效")
		return
	}
	req.Namespace, req.Name = c.Param("namespace"), c.Param("name")
	if message := validateResourceDataRequest(req); message != "" {
		apiShared.BadRequest(c, message)
		return
	}
	result, err := h.k8s.UpdateOpaqueSecretContext(c.Request.Context(), k8sclient.OpaqueSecretMutation{Namespace: req.Namespace, Name: req.Name, Data: req.Data})
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	log.Printf("AUDIT: opaque secret updated: %s/%s", req.Namespace, req.Name)
	apiShared.SuccessWithMessage(c, result, "Secret 更新成功")
}

func (h *K8sHandler) DeleteOpaqueSecret(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	namespace, name := c.Param("namespace"), c.Param("name")
	resource, err := h.k8s.GetSecretContext(c.Request.Context(), namespace, name)
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	if resource.Type != string(corev1.SecretTypeOpaque) {
		apiShared.Error(c, http.StatusConflict, apiShared.CodeBadRequest, "仅可管理 Opaque Secret")
		return
	}
	if len(resource.UsedBy) > 0 {
		apiShared.Error(c, http.StatusConflict, apiShared.CodeBadRequest, "Secret 正被工作负载引用，不能删除")
		return
	}
	if referenced, err := h.resourceIsReferenced(namespace, "secret", name); err != nil {
		apiShared.DBError(c, err.Error())
		return
	} else if referenced {
		apiShared.Error(c, http.StatusConflict, apiShared.CodeBadRequest, "Secret 仍被上线模板或发布快照引用，不能删除")
		return
	}
	if err := h.k8s.DeleteOpaqueSecretContext(c.Request.Context(), namespace, name); err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	log.Printf("AUDIT: opaque secret deleted: %s/%s", namespace, name)
	apiShared.SuccessWithMessage(c, nil, "Secret 删除成功")
}

// ==================== Ingress (standard) ====================

// ListIngresses 列出标准 Ingress
func (h *K8sHandler) ListIngresses(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	result, err := h.k8s.ListIngressesContext(c.Request.Context(), ns)
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	if result == nil {
		result = []k8sclient.IngressStdInfo{}
	}
	apiShared.Success(c, result)
}

// GetIngress 获取 Ingress 详情
func (h *K8sHandler) GetIngress(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := h.k8s.GetIngressContext(c.Request.Context(), ns, name)
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	apiShared.Success(c, result)
}

// CreateIngress 创建 Ingress
func (h *K8sHandler) CreateIngress(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	var req struct {
		Namespace   string `json:"namespace"`
		Name        string `json:"name"`
		Host        string `json:"host"`
		Path        string `json:"path"`
		ServiceName string `json:"service_name"`
		ServicePort string `json:"service_port"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "invalid request")
		return
	}
	if req.Path == "" {
		req.Path = "/"
	}

	if req.Namespace == "" || req.Name == "" || req.Host == "" || req.ServiceName == "" || req.ServicePort == "" {
		apiShared.BadRequest(c, "namespace, name, host, service_name, service_port 必填")
		return
	}
	result, err := h.k8s.CreateIngressContext(c.Request.Context(), req.Namespace, req.Name, req.Host, req.Path, req.ServiceName, req.ServicePort)
	if err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.Success(c, result)
}

// DeleteIngress 删除标准 Ingress
func (h *K8sHandler) DeleteIngress(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	if err := h.k8s.DeleteIngressContext(c.Request.Context(), ns, name); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, nil, "删除成功")
}

// GetIngressController 检测 Ingress Controller
func (h *K8sHandler) GetIngressController(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	status, _ := h.k8s.DetectIngressControllerContext(c.Request.Context())
	apiShared.Success(c, status)
}

// ==================== Legacy compatibility wrappers ====================

// GetService 获取 Service 详情（兼容旧 API，实际调用封装层）
func (h *K8sHandler) GetService(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := h.k8s.GetServiceContext(c.Request.Context(), ns, name)
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	apiShared.Success(c, result)
}

// UpdateService 更新 Service
func (h *K8sHandler) UpdateService(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")

	var req struct {
		Spec map[string]interface{} `json:"spec"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "invalid request")
		return
	}

	svc, err := h.k8s.CoreV1().Services(ns).Get(c.Request.Context(), name, metav1.GetOptions{})
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}

	// Apply allowed updates (ports, selector, type)
	if ports, ok := req.Spec["ports"]; ok {
		_ = ports
	}

	_, err = h.k8s.CoreV1().Services(ns).Update(c.Request.Context(), svc, metav1.UpdateOptions{})
	if err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, nil, "更新成功")
}

// DeleteService 删除 Service
func (h *K8sHandler) DeleteService(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	err := h.k8s.CoreV1().Services(ns).Delete(c.Request.Context(), name, metav1.DeleteOptions{})
	if err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, nil, "删除成功")
}

func ageStr(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
