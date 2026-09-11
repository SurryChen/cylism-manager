package system

import (
	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/repository"
	"github.com/gin-gonic/gin"
)

type AuditHandler struct {
	logs repository.AuditRepository
}

func NewAuditHandler(logs repository.AuditRepository) *AuditHandler {
	return &AuditHandler{logs: logs}
}

// List 查询审计日志 GET /api/audit-logs
func (h *AuditHandler) List(c *gin.Context) {
	resourceType := c.Query("resource_type")
	action := c.Query("action")
	keyword := c.Query("keyword")
	limit, offset := apiShared.LimitOffset(c, 20, 100)

	logs, total, err := h.logs.ListAuditLogs(resourceType, action, keyword, limit, offset)
	if err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.Success(c, gin.H{
		"data":  apiShared.AuditLogsDTO(logs),
		"total": total,
	})
}
