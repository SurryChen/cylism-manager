package system

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	alertingservice "github.com/cylism/cylism-manager/internal/service/observability/alerting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *AlertingHandler) WithSuppressionRules(store alertingservice.SuppressionStore) *AlertingHandler {
	h.suppression = store
	return h
}

func (h *AlertingHandler) suppressionScope(c *gin.Context, requireRule bool) (uint, uint, bool) {
	if h.suppression == nil {
		apiShared.DBError(c, "通知屏蔽规则存储不可用")
		return 0, 0, false
	}
	clusterID := h.clusterID
	if raw := c.Param("id"); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || id == 0 {
			apiShared.BadRequest(c, "集群 ID 无效")
			return 0, 0, false
		}
		clusterID = uint(id)
	}
	var ruleID uint
	if requireRule {
		id, err := strconv.ParseUint(c.Param("ruleID"), 10, 32)
		if err != nil || id == 0 {
			apiShared.BadRequest(c, "规则 ID 无效")
			return 0, 0, false
		}
		ruleID = uint(id)
	}
	return clusterID, ruleID, true
}

func (h *AlertingHandler) ListSuppressionRules(c *gin.Context) {
	clusterID, _, ok := h.suppressionScope(c, false)
	if !ok {
		return
	}
	status := c.Query("status")
	if status != "" && status != "active" && status != "disabled" && status != "expired" {
		apiShared.ValidationError(c, "规则状态无效")
		return
	}
	limit, offset := apiShared.LimitOffset(c, 10, 100)
	items, total, err := h.suppression.ListAlertSuppressionRules(c.Request.Context(), model.AlertSuppressionFilter{ClusterID: clusterID, NodeName: strings.TrimSpace(c.Query("node")), Status: status, Now: time.Now().UTC(), Limit: limit, Offset: offset})
	if err != nil {
		apiShared.DBError(c, "读取屏蔽规则失败")
		return
	}
	apiShared.Success(c, gin.H{"items": items, "total": total, "limit": limit, "offset": offset})
}

type suppressionRuleRequest struct {
	Name            string   `json:"name"`
	NodeName        string   `json:"node_name"`
	AlertNames      []string `json:"alert_names"`
	Comment         string   `json:"comment"`
	Enabled         *bool    `json:"enabled"`
	DurationMinutes *int     `json:"duration_minutes"`
}

func (h *AlertingHandler) SaveSuppressionRule(c *gin.Context) {
	update := c.Request.Method == "PUT"
	clusterID, id, ok := h.suppressionScope(c, update)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var req suppressionRuleRequest
	if c.ShouldBindJSON(&req) != nil || req.Enabled == nil || (!update && req.DurationMinutes == nil) || (req.DurationMinutes != nil && (*req.DurationMinutes < 0 || *req.DurationMinutes > 30*24*60)) {
		apiShared.ValidationError(c, "规则参数无效；有效期为 0（长期）或 1 至 43200 分钟")
		return
	}
	rule := model.AlertSuppressionRule{ClusterID: clusterID, Name: req.Name, NodeName: req.NodeName, AlertNames: req.AlertNames, Comment: req.Comment, Enabled: *req.Enabled}
	if err := alertingservice.ValidateSuppressionRule(&rule); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if req.DurationMinutes != nil {
		rule.StartsAt = time.Now().UTC()
		if *req.DurationMinutes > 0 {
			end := rule.StartsAt.Add(time.Duration(*req.DurationMinutes) * time.Minute)
			rule.EndsAt = &end
		}
	}
	var err error
	if update {
		err = h.suppression.UpdateAlertSuppressionRule(c.Request.Context(), clusterID, id, &rule, req.DurationMinutes != nil)
	} else {
		err = h.suppression.CreateAlertSuppressionRule(c.Request.Context(), &rule)
		id = rule.ID
	}
	if suppressionWriteError(c, err) {
		return
	}
	apiShared.Success(c, gin.H{"id": id})
}

func (h *AlertingHandler) SetSuppressionEnabled(c *gin.Context) {
	clusterID, id, ok := h.suppressionScope(c, true)
	if !ok {
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if c.ShouldBindJSON(&req) != nil || req.Enabled == nil {
		apiShared.ValidationError(c, "请指定启用状态")
		return
	}
	if suppressionWriteError(c, h.suppression.SetAlertSuppressionEnabled(c.Request.Context(), clusterID, id, *req.Enabled)) {
		return
	}
	apiShared.Success(c, gin.H{"id": id})
}

func (h *AlertingHandler) DeleteSuppressionRule(c *gin.Context) {
	clusterID, id, ok := h.suppressionScope(c, true)
	if !ok {
		return
	}
	if suppressionWriteError(c, h.suppression.DeleteAlertSuppressionRule(c.Request.Context(), clusterID, id)) {
		return
	}
	apiShared.Success(c, gin.H{"id": id})
}

func suppressionWriteError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.Error(c, http.StatusNotFound, apiShared.CodeNotFound, "屏蔽规则不存在")
	} else {
		apiShared.DBError(c, "保存屏蔽规则失败")
	}
	return true
}
