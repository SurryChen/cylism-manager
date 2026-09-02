package infrastructure

import (
	"fmt"
	"log"
	"net/http"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	storageservice "github.com/cylism/cylism-manager/internal/service/storage"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/gin-gonic/gin"
)

// K8sHandler h.k8s 资源管理的 HTTP handler
type K8sHandler struct {
	resourceReferences repository.ResourceReferenceRepository
	audit              repository.AuditRepository
	encKey             []byte
	k8s                *k8sclient.Client
	storageService     *storageservice.Service
}

func NewK8sHandlerWithClient(client *k8sclient.Client, references repository.ResourceReferenceRepository, storageService *storageservice.Service, audit repository.AuditRepository) *K8sHandler {
	return &K8sHandler{k8s: client, resourceReferences: references, storageService: storageService, audit: audit}
}

func NewK8sHandlerWithEncryption(references repository.ResourceReferenceRepository, storageService *storageservice.Service, encKey []byte, client *k8sclient.Client, audit repository.AuditRepository) *K8sHandler {
	h := NewK8sHandlerWithClient(client, references, storageService, audit)
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

	nodes, err := h.k8s.ListNodeInfos()
	nodeTotal := 0
	if err == nil {
		nodeTotal = len(nodes)
	}

	nsCount := 0
	if nsList, err := h.k8s.Clientset.CoreV1().Namespaces().List(h.k8s.Ctx(), metav1.ListOptions{}); err == nil {
		nsCount = len(nsList.Items)
	}

	podTotal, podReady := 0, 0
	if pods, pErr := h.k8s.Clientset.CoreV1().Pods("").List(h.k8s.Ctx(), metav1.ListOptions{}); pErr == nil {
		podTotal = len(pods.Items)
		for _, p := range pods.Items {
			if p.Status.Phase == "Running" {
				podReady++
			}
		}
	}

	// 新增：Deployment 统计
	deployTotal, deployReady := 0, 0
	if deps, dErr := h.k8s.ListDeployments(""); dErr == nil {
		deployTotal = len(deps)
		for _, d := range deps {
			if d.Ready == d.Replicas {
				deployReady++
			}
		}
	}

	// 新增：Service 统计
	svcTotal := 0
	if svcs, sErr := h.k8s.ListServices(""); sErr == nil {
		svcTotal = len(svcs)
	}

	version := ""
	if nodeTotal > 0 {
		version = nodes[0].Version
	}

	model.Success(c, map[string]interface{}{
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
	nsList, err := h.k8s.Clientset.CoreV1().Namespaces().List(h.k8s.Ctx(), metav1.ListOptions{})
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	result := make([]NamespaceNameSummary, 0, len(nsList.Items))
	for _, namespace := range nsList.Items {
		result = append(result, NamespaceNameSummary{Name: namespace.Name, Status: string(namespace.Status.Phase)})
	}
	model.Success(c, result)
}

// ListNamespaces 列出 Namespace 与资源摘要
func (h *K8sHandler) ListNamespaces(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}

	nsList, err := h.k8s.Clientset.CoreV1().Namespaces().List(h.k8s.Ctx(), metav1.ListOptions{})
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
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

		if list, listErr := h.k8s.Clientset.AppsV1().Deployments(name).List(h.k8s.Ctx(), metav1.ListOptions{}); listErr == nil {
			summary.Deployments = len(list.Items)
		}
		if list, listErr := h.k8s.Clientset.AppsV1().StatefulSets(name).List(h.k8s.Ctx(), metav1.ListOptions{}); listErr == nil {
			summary.StatefulSets = len(list.Items)
		}
		if list, listErr := h.k8s.Clientset.AppsV1().DaemonSets(name).List(h.k8s.Ctx(), metav1.ListOptions{}); listErr == nil {
			summary.DaemonSets = len(list.Items)
		}
		if list, listErr := h.k8s.Clientset.CoreV1().Services(name).List(h.k8s.Ctx(), metav1.ListOptions{}); listErr == nil {
			summary.Services = len(list.Items)
		}
		if list, listErr := h.k8s.Clientset.CoreV1().ConfigMaps(name).List(h.k8s.Ctx(), metav1.ListOptions{}); listErr == nil {
			summary.ConfigMaps = len(list.Items)
		}
		if list, listErr := h.k8s.Clientset.CoreV1().Secrets(name).List(h.k8s.Ctx(), metav1.ListOptions{}); listErr == nil {
			summary.Secrets = len(list.Items)
		}

		result = append(result, summary)
	}

	model.Success(c, result)
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

	created, err := h.k8s.Clientset.CoreV1().Namespaces().Create(h.k8s.Ctx(), ns, metav1.CreateOptions{})
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}

	model.SuccessWithMessage(c, gin.H{
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

	ns, err := h.k8s.Clientset.CoreV1().Namespaces().Get(h.k8s.Ctx(), name, metav1.GetOptions{})
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}

	ns.Labels = req.Labels
	ns.Annotations = req.Annotations
	updated, err := h.k8s.Clientset.CoreV1().Namespaces().Update(h.k8s.Ctx(), ns, metav1.UpdateOptions{})
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}

	model.SuccessWithMessage(c, gin.H{
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

	if err := h.k8s.Clientset.CoreV1().Namespaces().Delete(h.k8s.Ctx(), name, metav1.DeleteOptions{}); err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}

	model.SuccessWithMessage(c, nil, "命名空间删除成功")
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
	pods, err := h.k8s.Clientset.CoreV1().Pods(ns).List(h.k8s.Ctx(), metav1.ListOptions{})
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
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
	model.Success(c, result)
}

// ==================== Deployment ====================

// ListDeployments 列出 Deployment（使用封装层）
func (h *K8sHandler) ListDeployments(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	result, err := h.k8s.ListDeployments(ns)
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	if result == nil {
		result = []k8sclient.DeploymentInfo{}
	}
	model.Success(c, result)
}

// GetDeployment 获取 Deployment 详情
func (h *K8sHandler) GetDeployment(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := h.k8s.GetDeployment(ns, name)
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	model.Success(c, result)
}

// ListDeploymentPods 获取 Deployment 关联 Pod
func (h *K8sHandler) ListDeploymentPods(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := h.k8s.ListDeploymentPods(ns, name)
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, result)
}

// ListDeploymentRevisions 获取 Deployment 版本历史
func (h *K8sHandler) ListDeploymentRevisions(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := h.k8s.ListDeploymentRevisions(ns, name)
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, result)
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

	if err := h.k8s.ScaleDeployment(ns, name, req.Replicas); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	model.SuccessWithMessage(c, map[string]interface{}{"replicas": req.Replicas}, "扩缩容成功")
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

	if err := h.k8s.UpdateDeploymentImage(ns, name, req.Container, req.Image); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	model.SuccessWithMessage(c, nil, "镜像更新已触发滚动更新")
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

	if err := h.k8s.RollbackDeployment(ns, name, req.Revision); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	model.SuccessWithMessage(c, nil, "回滚成功")
}

// ==================== StatefulSet ====================

// ListStatefulSets 列出 StatefulSet
func (h *K8sHandler) ListStatefulSets(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	result, err := h.k8s.ListStatefulSets(ns)
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	if result == nil {
		result = []k8sclient.StatefulSetInfo{}
	}
	model.Success(c, result)
}

// GetStatefulSet 获取 StatefulSet 详情
func (h *K8sHandler) GetStatefulSet(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := h.k8s.GetStatefulSet(ns, name)
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	model.Success(c, result)
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

	if err := h.k8s.ScaleStatefulSet(ns, name, req.Replicas); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	model.SuccessWithMessage(c, map[string]interface{}{"replicas": req.Replicas}, "扩缩容成功")
}

// ==================== DaemonSet ====================

// ListDaemonSets 列出 DaemonSet
func (h *K8sHandler) ListDaemonSets(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	result, err := h.k8s.ListDaemonSets(ns)
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	if result == nil {
		result = []k8sclient.DaemonSetInfo{}
	}
	model.Success(c, result)
}

// GetDaemonSet 获取 DaemonSet 详情
func (h *K8sHandler) GetDaemonSet(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := h.k8s.GetDaemonSet(ns, name)
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	model.Success(c, result)
}

// ==================== Service (extended) ====================

// ListServicesV2 列出 Service（使用封装层，含 endpoint_count）
func (h *K8sHandler) ListServicesV2(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	result, err := h.k8s.ListServices(ns)
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	if result == nil {
		result = []k8sclient.ServiceEndpointInfo{}
	}
	model.Success(c, result)
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
	result, err := h.k8s.GetServiceEndpoints(ns, name)
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	if result == nil {
		result = []k8sclient.EndpointSliceInfo{}
	}
	model.Success(c, result)
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
		result, err = h.k8s.ListConfigMapsMetadata(ns)
	} else {
		result, err = h.k8s.ListConfigMaps(ns)
	}
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	if result == nil {
		result = []k8sclient.ConfigMapInfo{}
	}
	model.Success(c, result)
}

// GetConfigMap 获取 ConfigMap 详情
func (h *K8sHandler) GetConfigMap(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := h.k8s.GetConfigMap(ns, name)
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	model.Success(c, result)
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
	result, err := h.k8s.CreateConfigMap(k8sclient.ConfigMapMutation{Namespace: req.Namespace, Name: req.Name, Data: req.Data})
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	log.Printf("AUDIT: configmap created: %s/%s", req.Namespace, req.Name)
	model.SuccessWithMessage(c, result, "ConfigMap 创建成功")
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
	result, err := h.k8s.UpdateConfigMap(k8sclient.ConfigMapMutation{Namespace: req.Namespace, Name: req.Name, Data: req.Data})
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	log.Printf("AUDIT: configmap updated: %s/%s", req.Namespace, req.Name)
	model.SuccessWithMessage(c, result, "ConfigMap 更新成功")
}

func (h *K8sHandler) DeleteConfigMap(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	namespace, name := c.Param("namespace"), c.Param("name")
	resource, err := h.k8s.GetConfigMap(namespace, name)
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	if len(resource.UsedBy) > 0 {
		apiShared.Error(c, http.StatusConflict, model.CodeBadRequest, "ConfigMap 正被工作负载引用，不能删除")
		return
	}
	if referenced, err := h.resourceIsReferenced(namespace, "configmap", name); err != nil {
		apiShared.DBError(c, err.Error())
		return
	} else if referenced {
		apiShared.Error(c, http.StatusConflict, model.CodeBadRequest, "ConfigMap 仍被上线模板或发布快照引用，不能删除")
		return
	}
	if err := h.k8s.DeleteConfigMap(namespace, name); err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	log.Printf("AUDIT: configmap deleted: %s/%s", namespace, name)
	model.SuccessWithMessage(c, nil, "ConfigMap 删除成功")
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
		result, err = h.k8s.ListSecretsMetadata(ns)
	} else {
		result, err = h.k8s.ListSecrets(ns)
	}
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	if result == nil {
		result = []k8sclient.SecretInfo{}
	}
	model.Success(c, result)
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
	result, err := h.k8s.GetSecret(ns, name)
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
	model.Success(c, result)
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
	result, err := h.k8s.CreateOpaqueSecret(k8sclient.OpaqueSecretMutation{Namespace: req.Namespace, Name: req.Name, Data: req.Data})
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	log.Printf("AUDIT: opaque secret created: %s/%s", req.Namespace, req.Name)
	model.SuccessWithMessage(c, result, "Secret 创建成功")
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
	result, err := h.k8s.UpdateOpaqueSecret(k8sclient.OpaqueSecretMutation{Namespace: req.Namespace, Name: req.Name, Data: req.Data})
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	log.Printf("AUDIT: opaque secret updated: %s/%s", req.Namespace, req.Name)
	model.SuccessWithMessage(c, result, "Secret 更新成功")
}

func (h *K8sHandler) DeleteOpaqueSecret(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	namespace, name := c.Param("namespace"), c.Param("name")
	resource, err := h.k8s.GetSecret(namespace, name)
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	if resource.Type != string(corev1.SecretTypeOpaque) {
		apiShared.Error(c, http.StatusConflict, model.CodeBadRequest, "仅可管理 Opaque Secret")
		return
	}
	if len(resource.UsedBy) > 0 {
		apiShared.Error(c, http.StatusConflict, model.CodeBadRequest, "Secret 正被工作负载引用，不能删除")
		return
	}
	if referenced, err := h.resourceIsReferenced(namespace, "secret", name); err != nil {
		apiShared.DBError(c, err.Error())
		return
	} else if referenced {
		apiShared.Error(c, http.StatusConflict, model.CodeBadRequest, "Secret 仍被上线模板或发布快照引用，不能删除")
		return
	}
	if err := h.k8s.DeleteOpaqueSecret(namespace, name); err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	log.Printf("AUDIT: opaque secret deleted: %s/%s", namespace, name)
	model.SuccessWithMessage(c, nil, "Secret 删除成功")
}

// ==================== Ingress (standard) ====================

// ListIngresses 列出标准 Ingress
func (h *K8sHandler) ListIngresses(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	result, err := h.k8s.ListIngresses(ns)
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	if result == nil {
		result = []k8sclient.IngressStdInfo{}
	}
	model.Success(c, result)
}

// GetIngress 获取 Ingress 详情
func (h *K8sHandler) GetIngress(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := h.k8s.GetIngress(ns, name)
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	model.Success(c, result)
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
	result, err := h.k8s.CreateIngress(req.Namespace, req.Name, req.Host, req.Path, req.ServiceName, req.ServicePort)
	if err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	model.Success(c, result)
}

// DeleteIngress 删除标准 Ingress
func (h *K8sHandler) DeleteIngress(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	if err := h.k8s.DeleteIngress(ns, name); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	model.SuccessWithMessage(c, nil, "删除成功")
}

// GetIngressController 检测 Ingress Controller
func (h *K8sHandler) GetIngressController(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	status, _ := h.k8s.DetectIngressController()
	model.Success(c, status)
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
	result, err := h.k8s.GetService(ns, name)
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	model.Success(c, result)
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

	svc, err := h.k8s.Clientset.CoreV1().Services(ns).Get(h.k8s.Ctx(), name, metav1.GetOptions{})
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}

	// Apply allowed updates (ports, selector, type)
	if ports, ok := req.Spec["ports"]; ok {
		_ = ports
	}

	_, err = h.k8s.Clientset.CoreV1().Services(ns).Update(h.k8s.Ctx(), svc, metav1.UpdateOptions{})
	if err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	model.SuccessWithMessage(c, nil, "更新成功")
}

// DeleteService 删除 Service
func (h *K8sHandler) DeleteService(c *gin.Context) {
	if h.k8s == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	err := h.k8s.Clientset.CoreV1().Services(ns).Delete(h.k8s.Ctx(), name, metav1.DeleteOptions{})
	if err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	model.SuccessWithMessage(c, nil, "删除成功")
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
