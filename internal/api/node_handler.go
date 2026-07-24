package api

import (
	"net/http"
	"strconv"

	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

// NodeHandler K8s 节点管理的 HTTP handler
type NodeHandler struct {
	store *store.Store
}

// NewNodeHandler 创建 NodeHandler
func NewNodeHandler(s *store.Store) *NodeHandler {
	return &NodeHandler{store: s}
}

// ListNode 列出所有集群节点
func (h *NodeHandler) ListNode(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	nodes, err := K8s.ListNodeInfos()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "nodes": []interface{}{}})
		return
	}
	c.JSON(http.StatusOK, nodes)
}

// AddNode 将已注册的服务器加入 K3s 集群
func (h *NodeHandler) AddNode(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	server, err := h.store.GetServer(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "加入集群 - SSH 集成待实现", "server": server.Name})
}

// DrainNode 驱逐节点
func (h *NodeHandler) DrainNode(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	id := c.Param("id")
	if err := K8s.DrainNode(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "驱逐成功"})
}

// RemoveNode 从集群移除节点
func (h *NodeHandler) RemoveNode(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	id := c.Param("id")
	if err := K8s.DeleteNode(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "移除成功"})
}
