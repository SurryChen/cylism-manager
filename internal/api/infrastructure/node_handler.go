package infrastructure

import (
	"errors"
	"net/http"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/service/cluster"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NodeHandler maps node resource requests onto Cluster Service operations.
type NodeHandler struct {
	cluster *cluster.Service
}

func NewNodeHandler(service *cluster.Service) *NodeHandler {
	return &NodeHandler{cluster: service}
}

func (h *NodeHandler) ListNode(c *gin.Context) {
	nodes, err := h.cluster.ListNodesContext(c.Request.Context())
	if err != nil {
		writeClusterError(c, err, http.StatusOK, model.CodeK8sAPIError)
		return
	}
	model.Success(c, nodes)
}

func (h *NodeHandler) GetLabels(c *gin.Context) {
	labels, err := h.cluster.GetNodeLabelsContext(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeClusterError(c, err, http.StatusNotFound, model.CodeNotFound)
		return
	}
	model.Success(c, labels)
}

type updateNodeLabelsRequest struct {
	Set    map[string]string `json:"set"`
	Remove []string          `json:"remove"`
}

func (h *NodeHandler) UpdateLabels(c *gin.Context) {
	var request updateNodeLabelsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "节点标签请求无效")
		return
	}
	labels, err := h.cluster.UpdateNodeLabelsContext(c.Request.Context(), c.Param("id"), request.Set, request.Remove)
	if err != nil {
		writeClusterError(c, err, http.StatusBadRequest, model.CodeValidationFail)
		return
	}
	model.Success(c, labels)
}

func (h *NodeHandler) DrainPlan(c *gin.Context) {
	plan, err := h.cluster.GetDrainPlanContext(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeClusterError(c, err, http.StatusInternalServerError, model.CodeK8sAPIError)
		return
	}
	model.Success(c, plan)
}

type drainNodeRequest struct {
	DeleteEmptyDirData bool `json:"delete_empty_dir_data"`
}

func (h *NodeHandler) DrainNode(c *gin.Context) {
	var request drainNodeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "驱逐选项无效")
		return
	}
	result, err := h.cluster.DrainNodeContext(c.Request.Context(), c.Param("id"), k8s.DrainOptions{DeleteEmptyDirData: request.DeleteEmptyDirData})
	if err != nil {
		apiShared.ErrorWithData(c, http.StatusConflict, model.CodeConflict, err.Error(), result)
		return
	}
	message := "驱逐请求已提交"
	if len(result.Pending) > 0 {
		message = "部分 Pod 暂未迁移，请查看逐 Pod 原因后重试"
	}
	model.SuccessWithMessage(c, result, message)
}

type forceDrainNodeRequest struct {
	DeleteEmptyDirData bool   `json:"delete_empty_dir_data"`
	AcknowledgeRisk    bool   `json:"acknowledge_risk"`
	ConfirmNodeName    string `json:"confirm_node_name"`
}

func (h *NodeHandler) ForceDrainNode(c *gin.Context) {
	var request forceDrainNodeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "强制驱逐选项无效")
		return
	}
	result, err := h.cluster.ForceDrainNodeContext(c.Request.Context(), c.Param("id"), k8s.ForceDrainOptions{
		DeleteEmptyDirData: request.DeleteEmptyDirData,
		AcknowledgeRisk:    request.AcknowledgeRisk,
		ConfirmNodeName:    strings.TrimSpace(request.ConfirmNodeName),
	})
	if err != nil {
		if strings.Contains(err.Error(), "请确认风险") {
			apiShared.ValidationError(c, err.Error())
			return
		}
		apiShared.ErrorWithData(c, http.StatusConflict, model.CodeConflict, err.Error(), result)
		return
	}
	model.SuccessWithMessage(c, result, "故障节点强制驱逐请求已提交")
}

func (h *NodeHandler) RejoinNode(c *gin.Context) {
	info, err := h.cluster.RejoinNodeContext(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeClusterError(c, err, http.StatusConflict, model.CodeConflict)
		return
	}
	model.SuccessWithMessage(c, info, "节点已重新加入调度")
}

func (h *NodeHandler) RemovalCheck(c *gin.Context) {
	check, err := h.cluster.GetRemovalCheckContext(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeClusterError(c, err, http.StatusInternalServerError, model.CodeK8sAPIError)
		return
	}
	model.Success(c, check)
}

func (h *NodeHandler) RemoveNode(c *gin.Context) {
	if err := h.cluster.RemoveNodeContext(c.Request.Context(), c.Param("id")); err != nil {
		if errors.Is(err, cluster.ErrNodeBindingCleanup) {
			apiShared.InternalError(c, err.Error())
			return
		}
		apiShared.Conflict(c, err.Error())
		return
	}
	model.SuccessWithMessage(c, nil, "移除成功")
}

func (h *NodeHandler) AddNode(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "invalid server id")
		return
	}
	server, err := h.cluster.GetServer(id)
	if err != nil {
		apiShared.NotFound(c, "server not found")
		return
	}
	model.SuccessWithMessage(c, gin.H{"server": server.Name}, "加入集群 - SSH 集成待实现")
}

func (h *NodeHandler) PreImport(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "invalid server id")
		return
	}
	result, err := h.cluster.PreImportServerContext(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apiShared.NotFound(c, "server not found")
			return
		}
		code := model.CodeInternalError
		if strings.Contains(err.Error(), "集群") || strings.Contains(err.Error(), "节点") {
			code = model.CodeK8sAPIError
		}
		apiShared.Error(c, http.StatusOK, code, err.Error())
		return
	}
	model.Success(c, result)
}

func (h *NodeHandler) ConfirmImport(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "invalid server id")
		return
	}
	var request struct {
		Hostname string `json:"hostname" binding:"required"`
		Role     string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, err.Error())
		return
	}
	result, err := h.cluster.ConfirmImport(id, request.Hostname, request.Role)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apiShared.NotFound(c, "server not found")
			return
		}
		apiShared.InternalError(c, err.Error())
		return
	}
	model.SuccessWithMessage(c, gin.H{"server_name": result.ServerName, "node_name": result.NodeName, "role": result.Role}, "导入成功")
}

func writeClusterError(c *gin.Context, err error, status, code int) {
	if strings.Contains(err.Error(), "Kubernetes 集群未连接") {
		apiShared.K8sUnavailable(c)
		return
	}
	apiShared.Error(c, status, code, err.Error())
}
