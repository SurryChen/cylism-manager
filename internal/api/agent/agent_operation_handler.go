package agent

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
		ListAgentOperations(runtimeID uint, limit int, status, sessionID string) ([]model.AgentOperation, error)
		UpdateAgentOperationStatus(operationID, fromStatus, toStatus, errorSummary string, approvedBy *uint, completedAt *time.Time) (bool, error)
		CreateAuditLog(entry *model.AuditLog) error
		ListNodeRegistryMirrors() ([]model.NodeRegistryMirror, error)
		ListRegistryProxies() ([]model.RegistryProxy, error)
		ListServers() ([]model.Server, error)
		GetAlertEvent(uint) (*model.AlertEvent, error)
		UpdateAlertEvent(*model.AlertEvent) error
	}
	client                     *k8s.Client
	registryPullExecutor       AgentRegistryPullExecutor
	maintenanceCleanupExecutor AgentMaintenanceCleanupExecutor
}

func NewAgentOperationHandler(store interface {
	GetRuntime(id uint) (*model.RuntimeInstance, error)
	ListAgentCapabilityGrants(runtimeID uint) ([]model.AgentCapabilityGrant, error)
	ReplaceAgentCapabilityGrants(runtimeID uint, grants []model.AgentCapabilityGrant) error
	GetAgentOperation(operationID string) (*model.AgentOperation, error)
	ListAgentOperations(runtimeID uint, limit int, status, sessionID string) ([]model.AgentOperation, error)
	UpdateAgentOperationStatus(operationID, fromStatus, toStatus, errorSummary string, approvedBy *uint, completedAt *time.Time) (bool, error)
	CreateAuditLog(entry *model.AuditLog) error
	ListNodeRegistryMirrors() ([]model.NodeRegistryMirror, error)
	ListRegistryProxies() ([]model.RegistryProxy, error)
	ListServers() ([]model.Server, error)
	GetAlertEvent(uint) (*model.AlertEvent, error)
	UpdateAlertEvent(*model.AlertEvent) error
}, client *k8s.Client) *AgentOperationHandler {
	return &AgentOperationHandler{store: store, client: client}
}

func (h *AgentOperationHandler) WithRegistryPullExecutor(executor AgentRegistryPullExecutor) *AgentOperationHandler {
	h.registryPullExecutor = executor
	return h
}

func (h *AgentOperationHandler) WithMaintenanceCleanupExecutor(executor AgentMaintenanceCleanupExecutor) *AgentOperationHandler {
	h.maintenanceCleanupExecutor = executor
	return h
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
	status := strings.TrimSpace(c.Query("status"))
	if status != "" && !validAgentOperationStatus(status) {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "Agent 操作状态无效")
		return
	}
	sessionID := strings.TrimSpace(c.Query("session_id"))
	if len(sessionID) > 128 || strings.HasPrefix(sessionID, "api:") {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "会话 ID 无效")
		return
	}
	operations, err := h.store.ListAgentOperations(runtimeID, 50, status, sessionID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取 Agent 操作失败")
		return
	}
	items := make([]agentOperationSummary, 0, len(operations))
	for _, operation := range operations {
		items = append(items, toAgentOperationSummary(operation))
	}
	model.Success(c, items)
}

// agentOperationSummary intentionally omits raw parameters and their hash.
// Those values are immutable execution inputs, not browser display data.
type agentOperationSummary struct {
	OperationID   string     `json:"operation_id"`
	Capability    string     `json:"capability"`
	ChatSessionID string     `json:"chat_session_id,omitempty"`
	Status        string     `json:"status"`
	Summary       string     `json:"summary"`
	ErrorSummary  string     `json:"error_summary,omitempty"`
	ApprovedBy    *uint      `json:"approved_by,omitempty"`
	ApprovedAt    *time.Time `json:"approved_at,omitempty"`
	ExpiresAt     time.Time  `json:"expires_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

const agentOperationErrorSummaryLimit = 512

// agentOperationErrorSummary keeps terminal diagnostics useful without turning
// the approval history into a channel for operation inputs or credentials.
func agentOperationErrorSummary(prefix string, err error) string {
	if err == nil {
		return prefix
	}
	detail := strings.TrimSpace(redactAgentText(err.Error()))
	if detail == "" {
		return prefix
	}
	return truncateAgentText(prefix+": "+detail, agentOperationErrorSummaryLimit-3)
}

func maintenanceCleanupFailureSummary(output string, executeErr error) string {
	detail := strings.TrimSpace(output)
	if detail == "" {
		return agentOperationErrorSummary("执行固定清理配方失败", executeErr)
	}
	return agentOperationErrorSummary("执行固定清理配方失败", fmt.Errorf("%s: %w", detail, executeErr))
}

func toAgentOperationSummary(operation model.AgentOperation) agentOperationSummary {
	return agentOperationSummary{
		OperationID: operation.OperationID, Capability: operation.Capability,
		ChatSessionID: operation.ChatSessionID, Status: operation.Status,
		Summary: operation.Summary, ErrorSummary: operation.ErrorSummary,
		ApprovedBy: operation.ApprovedBy, ApprovedAt: operation.ApprovedAt,
		ExpiresAt: operation.ExpiresAt, CompletedAt: operation.CompletedAt,
		CreatedAt: operation.CreatedAt,
	}
}

func validAgentOperationStatus(status string) bool {
	switch status {
	case model.AgentOperationPendingApproval, model.AgentOperationApproved, model.AgentOperationRejected, model.AgentOperationSucceeded, model.AgentOperationFailed, model.AgentOperationStale, model.AgentOperationExpired:
		return true
	default:
		return false
	}
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
	if operation.Capability == model.AgentCapabilityRegistryPullCheck {
		h.resolveRegistryPullCheck(c, operation, userID)
		return
	}
	if operation.Capability == model.AgentCapabilityMaintenanceCleanup {
		h.resolveMaintenanceCleanup(c, operation, userID)
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
		_, _ = h.store.UpdateAgentOperationStatus(operationID, model.AgentOperationApproved, model.AgentOperationFailed, agentOperationErrorSummary("执行 Deployment 扩缩容失败", err), nil, &now)
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "执行 Deployment 扩缩容失败")
		return
	}
	now := time.Now()
	_, _ = h.store.UpdateAgentOperationStatus(operationID, model.AgentOperationApproved, model.AgentOperationSucceeded, "", nil, &now)
	h.audit(operation.RuntimeID, userID, "agent.operation_executed", map[string]any{"operation_id": operation.OperationID})
	model.SuccessWithMessage(c, gin.H{"operation_id": operation.OperationID, "status": model.AgentOperationSucceeded}, "Agent 操作已执行")
}

func (h *AgentOperationHandler) resolveMaintenanceCleanup(c *gin.Context, operation *model.AgentOperation, userID uint) {
	if h.maintenanceCleanupExecutor == nil {
		model.Error(c, http.StatusServiceUnavailable, model.CodeInternalError, "固定清理执行器不可用")
		return
	}
	var parameters maintenanceCleanupParameters
	if json.Unmarshal([]byte(operation.Parameters), &parameters) != nil || !validMaintenanceRecipe(parameters.Recipe) || parameters.AlertID == 0 || !validAgentResourceName(parameters.Node) {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "Agent 操作参数无效")
		return
	}
	event, err := h.store.GetAlertEvent(parameters.AlertID)
	if err != nil || event.NodeName != parameters.Node || event.Status == model.AlertEventResolved {
		h.markOperationStale(c, operation, "告警已恢复或目标节点已变化，需要重新诊断")
		return
	}
	servers, err := h.store.ListServers()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取节点映射失败")
		return
	}
	server, found := agentServerForNode(parameters.Node, servers)
	if !found {
		h.markOperationStale(c, operation, "目标节点不再由平台管理，需要重新诊断")
		return
	}
	changed, err := h.store.UpdateAgentOperationStatus(operation.OperationID, model.AgentOperationPendingApproval, model.AgentOperationApproved, "", &userID, nil)
	if err != nil || !changed {
		model.Error(c, http.StatusConflict, model.CodeConflict, "Agent 操作状态已变化")
		return
	}
	event.Status, event.DiagnosticSummary = model.AlertEventRemediating, "管理员已批准，正在执行固定清理配方"
	_ = h.store.UpdateAlertEvent(event)
	output, executeErr := h.maintenanceCleanupExecutor(server, parameters.Recipe)
	now := time.Now()
	if executeErr != nil {
		message := maintenanceCleanupFailureSummary(output, executeErr)
		_, _ = h.store.UpdateAgentOperationStatus(operation.OperationID, model.AgentOperationApproved, model.AgentOperationFailed, message, nil, &now)
		event.Status, event.LastError = model.AlertEventFailed, message
		_ = h.store.UpdateAlertEvent(event)
		model.Error(c, http.StatusBadGateway, model.CodeInternalError, "执行固定清理配方失败")
		return
	}
	_, _ = h.store.UpdateAgentOperationStatus(operation.OperationID, model.AgentOperationApproved, model.AgentOperationSucceeded, "", nil, &now)
	event.Status, event.DiagnosticSummary, event.LastError = model.AlertEventFiring, maintenanceCompletionSummary(parameters.Recipe, output), ""
	_ = h.store.UpdateAlertEvent(event)
	h.audit(operation.RuntimeID, userID, "agent.maintenance_cleanup_executed", map[string]any{"operation_id": operation.OperationID, "recipe": parameters.Recipe, "alert_id": parameters.AlertID})
	model.SuccessWithMessage(c, gin.H{"operation_id": operation.OperationID, "status": model.AgentOperationSucceeded}, "固定清理配方已执行，请根据后续告警与指标确认恢复")
}

func (h *AgentOperationHandler) resolveRegistryPullCheck(c *gin.Context, operation *model.AgentOperation, userID uint) {
	if h.registryPullExecutor == nil {
		model.Error(c, http.StatusServiceUnavailable, model.CodeInternalError, "镜像拉取检测执行器不可用")
		return
	}
	var parameters registryNodeRequest
	if json.Unmarshal([]byte(operation.Parameters), &parameters) != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "Agent 操作参数无效")
		return
	}
	mirrors, err := h.store.ListNodeRegistryMirrors()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取镜像源配置失败")
		return
	}
	proxies, err := h.store.ListRegistryProxies()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取镜像代理配置失败")
		return
	}
	config, found := agentResolveRegistry(parameters.Registry, mirrors, proxies)
	if !found || config.Mirror == nil || config.VerificationImage == "" || config.VerificationImage != parameters.VerificationImage {
		h.markOperationStale(c, operation, "镜像源配置或验证镜像已变化，需要重新发起审批")
		return
	}
	servers, err := h.store.ListServers()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取节点映射失败")
		return
	}
	server, found := agentServerForNode(parameters.Node, servers)
	if !found {
		h.markOperationStale(c, operation, "目标节点不再由平台管理，需要重新发起审批")
		return
	}
	changed, err := h.store.UpdateAgentOperationStatus(operation.OperationID, model.AgentOperationPendingApproval, model.AgentOperationApproved, "", &userID, nil)
	if err != nil || !changed {
		model.Error(c, http.StatusConflict, model.CodeConflict, "Agent 操作状态已变化")
		return
	}
	if err := h.registryPullExecutor(server, config.VerificationImage); err != nil {
		now := time.Now()
		_, _ = h.store.UpdateAgentOperationStatus(operation.OperationID, model.AgentOperationApproved, model.AgentOperationFailed, agentOperationErrorSummary("节点验证镜像拉取失败", err), nil, &now)
		model.Error(c, http.StatusBadGateway, model.CodeInternalError, "节点验证镜像拉取失败")
		return
	}
	now := time.Now()
	_, _ = h.store.UpdateAgentOperationStatus(operation.OperationID, model.AgentOperationApproved, model.AgentOperationSucceeded, "", nil, &now)
	h.audit(operation.RuntimeID, userID, "agent.operation_executed", map[string]any{"operation_id": operation.OperationID})
	model.SuccessWithMessage(c, gin.H{"operation_id": operation.OperationID, "status": model.AgentOperationSucceeded}, "节点验证镜像已拉取")
}

func (h *AgentOperationHandler) markOperationStale(c *gin.Context, operation *model.AgentOperation, message string) {
	_, _ = h.store.UpdateAgentOperationStatus(operation.OperationID, model.AgentOperationPendingApproval, model.AgentOperationStale, message, nil, nil)
	model.Error(c, http.StatusConflict, model.CodeConflict, message)
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
