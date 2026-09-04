package system

import (
	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	"github.com/gin-gonic/gin"
)

// OperationHandler 操作日志 HTTP handler
type OperationHandler struct {
	store repository.OperationLogRepository
}

// NewOperationHandler 创建操作日志 handler
func NewOperationHandler(s repository.OperationLogRepository) *OperationHandler {
	return &OperationHandler{store: s}
}

// ListOperations 查询操作日志 GET /api/operations?resource_type=server&resource_id=1
func (h *OperationHandler) ListOperations(c *gin.Context) {
	resourceType := c.Query("resource_type")
	resourceIDStr := c.Query("resource_id")

	if resourceType == "" || resourceIDStr == "" {
		apiShared.BadRequest(c, "resource_type 和 resource_id 参数为必填项")
		return
	}

	resourceID, err := apiShared.ParsePositiveID(resourceIDStr)
	if err != nil {
		apiShared.BadRequest(c, "resource_id 必须为整数")
		return
	}

	logs, err := h.store.ListOperationsByResource(resourceType, resourceID)
	if err != nil {
		apiShared.InternalError(c, "查询操作日志失败")
		return
	}

	if logs == nil {
		logs = []model.OperationLog{}
	}

	apiShared.Success(c, gin.H{"operations": apiShared.OperationLogsDTO(logs)})
}
