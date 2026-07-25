package api

import (
	"net/http"

	"github.com/cylism/cylism-manager/internal/model"
	"strconv"

	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

type AuditHandler struct {
	store *store.Store
}

func NewAuditHandler(s *store.Store) *AuditHandler {
	return &AuditHandler{store: s}
}

// List 查询审计日志 GET /api/audit-logs
func (h *AuditHandler) List(c *gin.Context) {
	resourceType := c.Query("resource_type")
	action := c.Query("action")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	logs, total, err := h.store.ListAuditLogs(resourceType, action, limit, offset)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
		return
	}
	model.Success(c, gin.H{
		"data":  logs,
		"total": total,
	})
}
