package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentOperationHandler is the browser-only approval boundary. Runtime tokens
// cannot approve operations; only this JWT-protected handler may execute them.
type AgentOperationHandler struct {
	store interface {
		GetRuntime(id uint) (*model.RuntimeInstance, error)
		ListAgentCapabilityGrants(runtimeID uint) ([]model.AgentCapabilityGrant, error)
		ReplaceAgentCapabilityGrants(runtimeID uint, grants []model.AgentCapabilityGrant) error
		GetAgentOperation(operationID string) (*model.AgentOperation, error)
		ListAgentOperations(runtimeID uint, limit int) ([]model.AgentOperation, error)
		UpdateAgentOperationStatus(operationID, fromStatus, toStatus, errorSummary string, approvedBy *uint, completedAt *time.Time) (bool, error)
		CreateAuditLog(entry *model.AuditLog) error
	}
	client *k8s.Client
}

func NewAgentOperationHandler(store interface {
	GetRuntime(id uint) (*model.RuntimeInstance, error)
	ListAgentCapabilityGrants(runtimeID uint) ([]model.AgentCapabilityGrant, error)
	ReplaceAgentCapabilityGrants(runtimeID uint, grants []model.AgentCapabilityGrant) error
	GetAgentOperation(operationID string) (*model.AgentOperation, error)
	ListAgentOperations(runtimeID uint, limit int) ([]model.AgentOperation, error)
	UpdateAgentOperationStatus(operationID, fromStatus, toStatus, errorSummary string, approvedBy *uint, completedAt *time.Time) (bool, error)
	CreateAuditLog(entry *model.AuditLog) error
}, client *k8s.Client) *AgentOperationHandler {
	return &AgentOperationHandler{store: store, client: client}
}

func (h *AgentOperationHandler) ListGrants(c *gin.Context) {
	runtimeID, ok := parseRuntimeID(c)
	if !ok || !h.runtimeExists(c, runtimeID) {
		return
	}
	grants, err := h.store.ListAgentCapabilityGrants(runtimeID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取 Agent 能力授权失败")
		return
	}
	model.Success(c, grants)
}

func (h *AgentOperationHandler) ReplaceGrants(c *gin.Context) {
	runtimeID, ok := parseRuntimeID(c)
	if !ok || !h.runtimeExists(c, runtimeID) {
		return
	}
	var request struct {
		Grants []model.AgentCapabilityGrant `json:"grants"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "授权参数无效")
		return
	}
	for index := range request.Grants {
		request.Grants[index].RuntimeID = runtimeID
	}
	if err := h.store.ReplaceAgentCapabilityGrants(runtimeID, request.Grants); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "授权范围或能力无效")
		return
	}
	h.audit(runtimeID, getUserID(c), "agent.grants_replaced", map[string]any{"grant_count": len(request.Grants)})
	model.SuccessWithMessage(c, request.Grants, "Agent 能力授权已更新")
}

func (h *AgentOperationHandler) ListOperations(c *gin.Context) {
	runtimeID, ok := parseRuntimeID(c)
	if !ok || !h.runtimeExists(c, runtimeID) {
		return
	}
	operations, err := h.store.ListAgentOperations(runtimeID, 50)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取 Agent 操作失败")
		return
	}
	model.Success(c, operations)
}

func (h *AgentOperationHandler) Approve(c *gin.Context) { h.resolve(c, true) }
func (h *AgentOperationHandler) Reject(c *gin.Context)  { h.resolve(c, false) }

func (h *AgentOperationHandler) resolve(c *gin.Context, approve bool) {
	operationID := c.Param("operationID")
	operation, err := h.store.GetAgentOperation(operationID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "Agent 操作不存在")
		return
	}
	if operation.Status != model.AgentOperationPendingApproval {
		model.Error(c, http.StatusConflict, model.CodeConflict, "Agent 操作不再等待审批")
		return
	}
	if time.Now().After(operation.ExpiresAt) {
		_, _ = h.store.UpdateAgentOperationStatus(operationID, model.AgentOperationPendingApproval, model.AgentOperationExpired, "审批已过期", nil, nil)
		model.Error(c, http.StatusConflict, model.CodeConflict, "Agent 操作审批已过期")
		return
	}
	userID := getUserID(c)
	if !approve {
		changed, err := h.store.UpdateAgentOperationStatus(operationID, model.AgentOperationPendingApproval, model.AgentOperationRejected, "已被管理员拒绝", &userID, nil)
		if err != nil || !changed {
			model.Error(c, http.StatusConflict, model.CodeConflict, "Agent 操作状态已变化")
			return
		}
		h.audit(operation.RuntimeID, userID, "agent.operation_rejected", map[string]any{"operation_id": operation.OperationID})
		model.SuccessWithMessage(c, gin.H{"operation_id": operation.OperationID, "status": model.AgentOperationRejected}, "Agent 操作已拒绝")
		return
	}
	if operation.Capability != model.AgentCapabilityDeploymentScale || h.client == nil || h.client.Clientset == nil {
		model.Error(c, http.StatusServiceUnavailable, model.CodeK8sUnavailable, "Agent 操作执行器不可用")
		return
	}
	var parameters deploymentScaleRequest
	if json.Unmarshal([]byte(operation.Parameters), &parameters) != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "Agent 操作参数无效")
		return
	}
	deployment, err := h.client.Clientset.AppsV1().Deployments(parameters.Namespace).Get(c.Request.Context(), parameters.Name, metav1.GetOptions{})
	if err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "读取目标 Deployment 失败")
		return
	}
	if deployment.ResourceVersion != operation.ResourceVersion {
		_, _ = h.store.UpdateAgentOperationStatus(operationID, model.AgentOperationPendingApproval, model.AgentOperationStale, "目标资源已变化，需要重新发起审批", nil, nil)
		model.Error(c, http.StatusConflict, model.CodeConflict, "目标资源已变化，需要重新发起审批")
		return
	}
	changed, err := h.store.UpdateAgentOperationStatus(operationID, model.AgentOperationPendingApproval, model.AgentOperationApproved, "", &userID, nil)
	if err != nil || !changed {
		model.Error(c, http.StatusConflict, model.CodeConflict, "Agent 操作状态已变化")
		return
	}
	deployment.Spec.Replicas = &parameters.Replicas
	if _, err := h.client.Clientset.AppsV1().Deployments(parameters.Namespace).Update(c.Request.Context(), deployment, metav1.UpdateOptions{}); err != nil {
		now := time.Now()
		_, _ = h.store.UpdateAgentOperationStatus(operationID, model.AgentOperationApproved, model.AgentOperationFailed, "执行 Deployment 扩缩容失败", nil, &now)
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "执行 Deployment 扩缩容失败")
		return
	}
	now := time.Now()
	_, _ = h.store.UpdateAgentOperationStatus(operationID, model.AgentOperationApproved, model.AgentOperationSucceeded, "", nil, &now)
	h.audit(operation.RuntimeID, userID, "agent.operation_executed", map[string]any{"operation_id": operation.OperationID})
	model.SuccessWithMessage(c, gin.H{"operation_id": operation.OperationID, "status": model.AgentOperationSucceeded}, "Agent 操作已执行")
}

func (h *AgentOperationHandler) runtimeExists(c *gin.Context, runtimeID uint) bool {
	if _, err := h.store.GetRuntime(runtimeID); err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "Runtime 不存在")
		return false
	}
	return true
}

func (h *AgentOperationHandler) audit(runtimeID, userID uint, action string, detail map[string]any) {
	encoded, err := json.Marshal(detail)
	if err == nil {
		_ = h.store.CreateAuditLog(&model.AuditLog{Action: action, ResourceType: "agent_runtime", ResourceID: runtimeID, UserID: userID, Detail: string(encoded), CreatedAt: time.Now()})
	}
}

func parseRuntimeID(c *gin.Context) (uint, bool) {
	value := strings.TrimSpace(c.Param("id"))
	var runtimeID uint
	if _, err := fmt.Sscan(value, &runtimeID); err != nil || runtimeID == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "Runtime ID 无效")
		return 0, false
	}
	return runtimeID, true
}
