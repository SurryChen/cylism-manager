package api

import (
	"github.com/cylism/cylism-manager/internal/model"
	"net/http"
	"strconv"

	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

// OperationHandler 操作日志 HTTP handler
type OperationHandler struct {
	store *store.Store
}

// NewOperationHandler 创建操作日志 handler
func NewOperationHandler(s *store.Store) *OperationHandler {
	return &OperationHandler{store: s}
}

// ListOperations 查询操作日志
// GET /api/operations?resource_type=server&resource_id=1
func (h *OperationHandler) ListOperations(c *gin.Context) {
	resourceType := c.Query("resource_type")
	resourceIDStr := c.Query("resource_id")

	if resourceType == "" || resourceIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "resource_type 和 resource_id 参数为必填项",
		})
		return
	}

	resourceID, err := strconv.ParseUint(resourceIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "resource_id 必须为整数",
		})
		return
	}

	logs, err := h.store.ListOperationsByResource(resourceType, uint(resourceID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询操作日志失败",
		})
		return
	}

	if logs == nil {
		logs = []model.OperationLog{}
	}

	c.JSON(http.StatusOK, gin.H{
		"operations": logs,
	})
}