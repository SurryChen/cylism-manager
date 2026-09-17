package system

import (
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
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
	createdFrom, err := parseAuditDate(c.Query("created_from"), false)
	if err != nil {
		apiShared.BadRequest(c, "created_from 必须是 YYYY-MM-DD")
		return
	}
	createdTo, err := parseAuditDate(c.Query("created_to"), true)
	if err != nil {
		apiShared.BadRequest(c, "created_to 必须是 YYYY-MM-DD")
		return
	}
	filter := model.AuditLogFilter{
		ResourceType: c.Query("resource_type"),
		Action:       c.Query("action"),
		Outcome:      c.Query("outcome"),
		Source:       c.Query("source"),
		ActorType:    c.Query("actor_type"),
		TargetName:   c.Query("target_name"),
		Keyword:      c.Query("keyword"),
		CreatedFrom:  createdFrom,
		CreatedTo:    createdTo,
	}
	limit, offset := apiShared.LimitOffset(c, 20, 100)
	filter.Limit, filter.Offset = limit, offset

	logs, total, err := h.logs.ListAuditLogsFiltered(filter)
	if err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.Success(c, gin.H{
		"data":  apiShared.AuditLogsDTO(logs),
		"total": total,
	})
}

func parseAuditDate(value string, end bool) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, time.Local)
	if err != nil {
		return time.Time{}, err
	}
	if end {
		parsed = parsed.AddDate(0, 0, 1)
	}
	return parsed, nil
}
