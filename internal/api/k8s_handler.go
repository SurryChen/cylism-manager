package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// Dashboard 集群摘要
func (h *K8sHandler) Dashboard(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	nodes, err := K8s.Clientset.CoreV1().Nodes().List(K8s.Ctx(), metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "data": gin.H{}})
		return
	}
	nodeTotal := len(nodes.Items)

	nsList, err := K8s.Clientset.CoreV1().Namespaces().List(K8s.Ctx(), metav1.ListOptions{})
	nsCount := 0
	if err == nil {
		nsCount = len(nsList.Items)
	}

	pods, err := K8s.Clientset.CoreV1().Pods("").List(K8s.Ctx(), metav1.ListOptions{})
	podTotal, podReady := 0, 0
	if err == nil {
		podTotal = len(pods.Items)
		for _, p := range pods.Items {
			if p.Status.Phase == "Running" {
				podReady++
			}
		}
	}

	version := ""
	if nodeTotal > 0 {
		version = nodes.Items[0].Status.NodeInfo.KubeletVersion
	}

	c.JSON(http.StatusOK, gin.H{
		"nodes_total": nodeTotal,
		"pods_total":  podTotal,
		"pods_ready":  podReady,
		"namespaces":  nsCount,
		"version":     version,
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

// ListServices Service 列表
func (h *K8sHandler) ListServices(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	svcs, err := K8s.Clientset.CoreV1().Services(ns).List(K8s.Ctx(), metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "data": []interface{}{}})
		return
	}

	type SvcInfo struct {
		Name      string   `json:"name"`
		Namespace string   `json:"namespace"`
		ClusterIP string   `json:"cluster_ip"`
		Type      string   `json:"type"`
		Ports     []string `json:"ports"`
	}
	var result []SvcInfo
	for _, s := range svcs.Items {
		ports := make([]string, 0, len(s.Spec.Ports))
		for _, p := range s.Spec.Ports {
			ports = append(ports, string(p.Protocol)+":"+p.TargetPort.String())
		}
		result = append(result, SvcInfo{
			Name:      s.Name,
			Namespace: s.Namespace,
			ClusterIP: s.Spec.ClusterIP,
			Type:      string(s.Spec.Type),
			Ports:     ports,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// ListDeployments Deployment 列表
func (h *K8sHandler) ListDeployments(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Query("namespace")
	deploys, err := K8s.Clientset.AppsV1().Deployments(ns).List(K8s.Ctx(), metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "data": []interface{}{}})
		return
	}

	type DeployInfo struct {
		Name       string   `json:"name"`
		Namespace  string   `json:"namespace"`
		Replicas   int32    `json:"replicas"`
		Ready      int32    `json:"ready"`
		Images     []string `json:"images"`
	}
	var result []DeployInfo
	for _, d := range deploys.Items {
		images := make([]string, 0, len(d.Spec.Template.Spec.Containers))
		for _, c := range d.Spec.Template.Spec.Containers {
			images = append(images, c.Image)
		}
		result = append(result, DeployInfo{
			Name:      d.Name,
			Namespace: d.Namespace,
			Replicas:  *d.Spec.Replicas,
			Ready:     d.Status.ReadyReplicas,
			Images:    images,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// GetService Service 详情
func (h *K8sHandler) GetService(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	svc, err := K8s.Clientset.CoreV1().Services(ns).Get(K8s.Ctx(), name, metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	type PortInfo struct {
		Name       string `json:"name"`
		Protocol   string `json:"protocol"`
		Port       int32  `json:"port"`
		TargetPort string `json:"target_port"`
	}
	ports := make([]PortInfo, 0, len(svc.Spec.Ports))
	for _, p := range svc.Spec.Ports {
		ports = append(ports, PortInfo{
			Name:       p.Name,
			Protocol:   string(p.Protocol),
			Port:       p.Port,
			TargetPort: p.TargetPort.String(),
		})
	}

	selector := make(map[string]string)
	for k, v := range svc.Spec.Selector {
		selector[k] = v
	}

	c.JSON(http.StatusOK, gin.H{
		"name":      svc.Name,
		"namespace": svc.Namespace,
		"cluster_ip": svc.Spec.ClusterIP,
		"type":      string(svc.Spec.Type),
		"ports":     ports,
		"selector":  selector,
	})
}

// UpdateService 更新 Service
func (h *K8sHandler) UpdateService(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "更新 Service - 待实现"})
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
