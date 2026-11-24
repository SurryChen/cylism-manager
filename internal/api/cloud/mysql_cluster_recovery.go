package cloud

import (
	"errors"
	"fmt"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	database "github.com/cylism/cylism-manager/internal/service/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mysqlPreflightNode struct {
	Name   string `json:"name"`
	Role   string `json:"role"`
	Ready  bool   `json:"ready"`
	Reason string `json:"reason,omitempty"`
}

func (h *DatabaseHandler) PreflightCluster(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
		return
	}
	var req mysqlClusterTopologyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "部署配置无效")
		return
	}
	if len(req.DatabaseNodes) > 0 && !sameNodePool(item.DatabaseNodes, req.DatabaseNodes) {
		apiShared.ValidationError(c, "数据库候选节点与资源池不一致，请先修改资源池")
		return
	}
	if len(item.DatabaseNodes) == 0 {
		apiShared.ValidationError(c, "请先添加数据库资源池")
		return
	}
	req.DatabaseNodes = item.DatabaseNodes
	candidates, err := databaseCandidates(item.DatabaseNodes, req.CandidateNodes)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	req.RouterNodes = nil
	zeroRouters := 0
	req.RouterInstances = &zeroRouters
	req.StorageClass = item.StorageClass
	quantity, err := resource.ParseQuantity(strings.TrimSpace(req.StorageSize))
	if err != nil || quantity.Sign() <= 0 {
		apiShared.ValidationError(c, "存储配置无效")
		return
	}
	if h.client == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	if len(req.DatabaseNodes) < 1 {
		apiShared.ValidationError(c, "至少选择 1 个数据库候选节点")
		return
	}
	if err := database.ValidateDatabaseInstances(req.DatabaseInstances, req.DatabaseNodes); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if req.RouterInstances != nil && (*req.RouterInstances < 0 || *req.RouterInstances > len(req.RouterNodes)) {
		apiShared.ValidationError(c, "Router 实例数须在 0 和候选节点数之间")
		return
	}
	client, err := h.client(c.Request.Context(), item.ClusterID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	nodes, err := database.DeploymentNodes(c.Request.Context(), client)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	lookup := make(map[string]database.DeploymentNode, len(nodes))
	for _, node := range nodes {
		lookup[node.Name] = node
	}
	result := make([]mysqlPreflightNode, 0, len(candidates)+len(req.RouterNodes))
	allReady := true
	for _, group := range []struct {
		role  string
		names []string
	}{{"database", candidates}, {"router", req.RouterNodes}} {
		for _, name := range group.names {
			node, found := lookup[name]
			check := mysqlPreflightNode{Name: name, Role: group.role, Ready: found}
			if !found {
				check.Reason = "节点不存在"
			} else if group.role == "database" {
				check.Ready, check.Reason = node.DatabaseAllowed, node.BlockedReason
			} else {
				check.Ready, check.Reason = node.RouterAllowed, node.RouterBlockedReason
			}
			if !check.Ready {
				allReady = false
			}
			result = append(result, check)
		}
	}
	storageClass, storageErr := database.ResolveStorageClass(c.Request.Context(), client, strings.TrimSpace(req.StorageClass))
	if storageErr != nil {
		allReady = false
	}
	storageReason := ""
	if storageErr != nil {
		storageReason = storageErr.Error()
	}
	apiShared.Success(c, gin.H{"ready": allReady, "nodes": result, "storage_class": storageClass, "storage_reason": storageReason, "images": "Pod 创建后可查看真实镜像及拉取状态；部署前无法确定 Operator 选择的镜像", "disk_capacity": "Kubernetes 未提供节点可用磁盘容量；请确认目标节点有足够空间"})
}

func (h *DatabaseHandler) RetryClusterPod(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
		return
	}
	if item.Status == "draft" || item.Status == "paused" {
		apiShared.ValidationError(c, "集群尚未运行，无法重建 Pod")
		return
	}
	if h.client == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	client, err := h.client(c.Request.Context(), item.ClusterID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	name := c.Param("podName")
	pod, err := client.Clientset.CoreV1().Pods(item.Namespace).Get(c.Request.Context(), name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		apiShared.NotFound(c, "Pod 不存在")
		return
	}
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := database.ValidateRetryPod(item, pod); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	uid := pod.UID
	if err := client.Clientset.CoreV1().Pods(item.Namespace).Delete(c.Request.Context(), name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil {
		apiShared.ValidationError(c, fmt.Sprintf("请求重建 Pod 失败：%v", err))
		return
	}
	var task model.MySQLClusterTask
	if err := h.store.DB().Where("cluster_id = ? AND kind = ?", item.ID, "deploy").Order("id desc").First(&task).Error; err == nil {
		_ = h.appendMySQLTaskLog(&task, "已请求控制器重建异常 Pod "+name+"；数据卷保留，等待新 Pod 就绪")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.DBError(c, err.Error())
		return
	}
	apiShared.Success(c, gin.H{"pod_name": name})
}

func (h *DatabaseHandler) PullClusterPodImages(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
		return
	}
	if h.registryPull == nil || h.client == nil {
		apiShared.ValidationError(c, "节点镜像拉取能力不可用")
		return
	}
	client, err := h.client(c.Request.Context(), item.ClusterID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	pod, err := client.Clientset.CoreV1().Pods(item.Namespace).Get(c.Request.Context(), c.Param("podName"), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		apiShared.NotFound(c, "Pod 不存在")
		return
	}
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := database.ValidateManagedPod(item, pod); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if pod.Spec.NodeName == "" {
		apiShared.ValidationError(c, "Pod 尚未分配节点")
		return
	}
	servers, err := h.store.ListServers()
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	var server *model.Server
	for i := range servers {
		if servers[i].ClusterID != nil && *servers[i].ClusterID == item.ClusterID && servers[i].K8sNodeName == pod.Spec.NodeName {
			server = &servers[i]
			break
		}
	}
	if server == nil {
		apiShared.ValidationError(c, "未找到与 Pod 节点对应的已纳管服务器，无法从该节点拉取镜像")
		return
	}
	images := database.PendingPodImages(pod)
	if len(images) == 0 {
		apiShared.ValidationError(c, "Pod 没有可拉取的镜像")
		return
	}
	for _, image := range images {
		if err := h.registryPull.Pull(c.Request.Context(), server, image); err != nil {
			apiShared.ValidationError(c, fmt.Sprintf("节点 %s 拉取 %s 失败：%v", pod.Spec.NodeName, image, err))
			return
		}
	}
	var task model.MySQLClusterTask
	if err := h.store.DB().Where("cluster_id = ? AND kind = ?", item.ID, "deploy").Order("id desc").First(&task).Error; err == nil {
		_ = h.appendMySQLTaskLog(&task, fmt.Sprintf("节点 %s 已预拉取 Pod %s 的镜像：%s", pod.Spec.NodeName, pod.Name, strings.Join(images, "、")))
	}
	apiShared.Success(c, gin.H{"node_name": pod.Spec.NodeName, "images": images})
}
