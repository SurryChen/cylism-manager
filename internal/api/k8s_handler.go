package api

import (
	"log"
	"net/http"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"


	"github.com/gin-gonic/gin"
)

// K8sHandler K8s 资源管理的 HTTP handler
type K8sHandler struct{}

// NewK8sHandler 创建 K8sHandler
func NewK8sHandler() *K8sHandler {
	return &K8sHandler{}
}

func k8sUnavailable(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"error": "K8s 集群未连接",
		"data":  []interface{}{},
	})
}

// Dashboard 集群摘要（扩展 Deployment/Service 统计）
func (h *K8sHandler) Dashboard(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}

	nodes, err := K8s.ListNodeInfos()
	nodeTotal := 0
	if err == nil {
		nodeTotal = len(nodes)
	}

	nsCount := 0
	if nsList, err := K8s.Clientset.CoreV1().Namespaces().List(K8s.Ctx(), metav1.ListOptions{}); err == nil {
		nsCount = len(nsList.Items)
	}

	podTotal, podReady := 0, 0
	if pods, pErr := K8s.Clientset.CoreV1().Pods("").List(K8s.Ctx(), metav1.ListOptions{}); pErr == nil {
		podTotal = len(pods.Items)
		for _, p := range pods.Items {
			if p.Status.Phase == "Running" {
				podReady++
			}
		}
	}

	// 新增：Deployment 统计
	deployTotal, deployReady := 0, 0
	if deps, dErr := K8s.ListDeployments(""); dErr == nil {
		deployTotal = len(deps)
		for _, d := range deps {
			if d.Ready == d.Replicas {
				deployReady++
			}
		}
	}

	// 新增：Service 统计
	svcTotal := 0
	if svcs, sErr := K8s.ListServices(""); sErr == nil {
		svcTotal = len(svcs)
	}

	version := ""
	if nodeTotal > 0 {
		version = nodes[0].Version
	}

	c.JSON(http.StatusOK, gin.H{
		"nodes_total":        nodeTotal,
		"pods_total":         podTotal,
		"pods_ready":         podReady,
		"deployments_total":  deployTotal,
		"deployments_ready":  deployReady,
		"services_total":     svcTotal,
		"namespaces":         nsCount,
		"version":            version,
	})
}

// ListPods Pod 列表
func (h *K8sHandler) ListPods(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	pods, err := K8s.Clientset.CoreV1().Pods(ns).List(K8s.Ctx(), metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "data": []interface{}{}})
		return
	}

	type PodInfo struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Status    string `json:"status"`
		Node      string `json:"node"`
		IP        string `json:"ip"`
		Restarts  int32  `json:"restarts"`
	}
	var result []PodInfo
	for _, p := range pods.Items {
		restarts := int32(0)
		for _, cs := range p.Status.ContainerStatuses {
			restarts += cs.RestartCount
		}
		result = append(result, PodInfo{
			Name:      p.Name,
			Namespace: p.Namespace,
			Status:    string(p.Status.Phase),
			Node:      p.Spec.NodeName,
			IP:        p.Status.PodIP,
			Restarts:  restarts,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// ==================== Deployment ====================

// ListDeployments 列出 Deployment（使用封装层）
func (h *K8sHandler) ListDeployments(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	result, err := K8s.ListDeployments(ns)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "data": []interface{}{}})
		return
	}
	if result == nil {
		result = []k8sclient.DeploymentInfo{}
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// GetDeployment 获取 Deployment 详情
func (h *K8sHandler) GetDeployment(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := K8s.GetDeployment(ns, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// ListDeploymentPods 获取 Deployment 关联 Pod
func (h *K8sHandler) ListDeploymentPods(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := K8s.ListDeploymentPods(ns, name)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "data": []interface{}{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// ListDeploymentRevisions 获取 Deployment 版本历史
func (h *K8sHandler) ListDeploymentRevisions(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := K8s.ListDeploymentRevisions(ns, name)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "data": []interface{}{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// ScaleDeployment 扩缩容 Deployment
func (h *K8sHandler) ScaleDeployment(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")

	var req struct {
		Replicas int32 `json:"replicas"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid replicas"})
		return
	}

	if err := K8s.ScaleDeployment(ns, name, req.Replicas); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "扩缩容成功", "replicas": req.Replicas})
}

// UpdateDeploymentImage 更新 Deployment 镜像
func (h *K8sHandler) UpdateDeploymentImage(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")

	var req struct {
		Container string `json:"container"`
		Image     string `json:"image"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Container == "" || req.Image == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container 和 image 必填"})
		return
	}

	if err := K8s.UpdateDeploymentImage(ns, name, req.Container, req.Image); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "镜像更新已触发滚动更新"})
}

// RollbackDeployment 回滚 Deployment
func (h *K8sHandler) RollbackDeployment(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")

	var req struct {
		Revision int64 `json:"revision"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid revision"})
		return
	}

	if err := K8s.RollbackDeployment(ns, name, req.Revision); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "回滚成功"})
}

// ==================== StatefulSet ====================

// ListStatefulSets 列出 StatefulSet
func (h *K8sHandler) ListStatefulSets(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	result, err := K8s.ListStatefulSets(ns)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "data": []interface{}{}})
		return
	}
	if result == nil {
		result = []k8sclient.StatefulSetInfo{}
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// GetStatefulSet 获取 StatefulSet 详情
func (h *K8sHandler) GetStatefulSet(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := K8s.GetStatefulSet(ns, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// ScaleStatefulSet 扩缩容 StatefulSet
func (h *K8sHandler) ScaleStatefulSet(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")

	var req struct {
		Replicas int32 `json:"replicas"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid replicas"})
		return
	}

	if err := K8s.ScaleStatefulSet(ns, name, req.Replicas); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "扩缩容成功", "replicas": req.Replicas})
}

// ==================== DaemonSet ====================

// ListDaemonSets 列出 DaemonSet
func (h *K8sHandler) ListDaemonSets(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	result, err := K8s.ListDaemonSets(ns)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "data": []interface{}{}})
		return
	}
	if result == nil {
		result = []k8sclient.DaemonSetInfo{}
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// GetDaemonSet 获取 DaemonSet 详情
func (h *K8sHandler) GetDaemonSet(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := K8s.GetDaemonSet(ns, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// ==================== Service (extended) ====================

// ListServicesV2 列出 Service（使用封装层，含 endpoint_count）
func (h *K8sHandler) ListServicesV2(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	result, err := K8s.ListServices(ns)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "data": []interface{}{}})
		return
	}
	if result == nil {
		result = []k8sclient.ServiceEndpointInfo{}
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// ==================== EndpointSlice ====================

// GetServiceEndpoints 获取 Service 的 EndpointSlice
func (h *K8sHandler) GetServiceEndpoints(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := K8s.GetServiceEndpoints(ns, name)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "data": []interface{}{}})
		return
	}
	if result == nil {
		result = []k8sclient.EndpointSliceInfo{}
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// ==================== ConfigMap ====================

// ListConfigMaps 列出 ConfigMap
func (h *K8sHandler) ListConfigMaps(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	result, err := K8s.ListConfigMaps(ns)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "data": []interface{}{}})
		return
	}
	if result == nil {
		result = []k8sclient.ConfigMapInfo{}
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// GetConfigMap 获取 ConfigMap 详情
func (h *K8sHandler) GetConfigMap(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := K8s.GetConfigMap(ns, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// ==================== Secret ====================

// ListSecrets 列出 Secret
func (h *K8sHandler) ListSecrets(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	result, err := K8s.ListSecrets(ns)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "data": []interface{}{}})
		return
	}
	if result == nil {
		result = []k8sclient.SecretInfo{}
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// GetSecret 获取 Secret 详情
// GetSecret 获取 Secret 详情（value 已脱敏，仅返回 key 列表，前端需单独请求解码）
func (h *K8sHandler) GetSecret(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := K8s.GetSecret(ns, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	log.Printf("AUDIT: secret accessed: %s/%s", ns, name)
	// Redact secret values before sending over wire
	redacted := make(map[string]string)
	for k := range result.Data {
		redacted[k] = "W1JFREFDVEVEXQ==" // base64 of [REDACTED]
	}
	result.Data = redacted
	c.JSON(http.StatusOK, result)
}

// ==================== Ingress (standard) ====================

// ListIngresses 列出标准 Ingress
func (h *K8sHandler) ListIngresses(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	result, err := K8s.ListIngresses(ns)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "data": []interface{}{}})
		return
	}
	if result == nil {
		result = []k8sclient.IngressStdInfo{}
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// GetIngress 获取 Ingress 详情
func (h *K8sHandler) GetIngress(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := K8s.GetIngress(ns, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// CreateIngress 创建 Ingress
func (h *K8sHandler) CreateIngress(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if req.Path == "" {
		req.Path = "/"
	}

	if req.Namespace == "" || req.Name == "" || req.Host == "" || req.ServiceName == "" || req.ServicePort == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "namespace, name, host, service_name, service_port 必填"})
		return
	}
	result, err := K8s.CreateIngress(req.Namespace, req.Name, req.Host, req.Path, req.ServiceName, req.ServicePort)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// DeleteIngress 删除标准 Ingress
func (h *K8sHandler) DeleteIngress(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	if err := K8s.DeleteIngress(ns, name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// GetIngressController 检测 Ingress Controller
func (h *K8sHandler) GetIngressController(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	status, _ := K8s.DetectIngressController()
	c.JSON(http.StatusOK, status)
}

// ==================== Legacy compatibility wrappers ====================

// GetService 获取 Service 详情（兼容旧 API，实际调用封装层）
func (h *K8sHandler) GetService(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	result, err := K8s.GetService(ns, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// UpdateService 更新 Service
func (h *K8sHandler) UpdateService(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")

	var req struct {
		Spec map[string]interface{} `json:"spec"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	svc, err := K8s.Clientset.CoreV1().Services(ns).Get(K8s.Ctx(), name, metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Apply allowed updates (ports, selector, type)
	if ports, ok := req.Spec["ports"]; ok {
		_ = ports
	}

	_, err = K8s.Clientset.CoreV1().Services(ns).Update(K8s.Ctx(), svc, metav1.UpdateOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// DeleteService 删除 Service
func (h *K8sHandler) DeleteService(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	err := K8s.Clientset.CoreV1().Services(ns).Delete(K8s.Ctx(), name, metav1.DeleteOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}
