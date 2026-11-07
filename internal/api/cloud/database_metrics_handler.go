package cloud

import (
	"strconv"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	cloudservice "github.com/cylism/cylism-manager/internal/service/cloud"
	"github.com/gin-gonic/gin"
)

func (h *DatabaseHandler) WithCloudMetrics(service *cloudservice.Service) *DatabaseHandler {
	h.cloudMetrics = service
	return h
}

func (h *DatabaseHandler) ConfigureDatabaseMonitor(c *gin.Context) {
	item, ok := h.get(c)
	if !ok {
		return
	}
	var req struct {
		ConnectionID uint   `json:"connection_id"`
		Region       string `json:"region"`
		InstanceID   string `json:"instance_id"`
	}
	if c.ShouldBindJSON(&req) != nil || item.Mode != "external" || h.cloudMetrics == nil {
		apiShared.ValidationError(c, "仅已接入的外部实例支持云监控配置")
		return
	}
	req.Region, req.InstanceID = strings.TrimSpace(req.Region), strings.TrimSpace(req.InstanceID)
	if req.ConnectionID != 0 {
		if err := h.cloudMetrics.ValidateDatabaseMonitor(req.ConnectionID, req.Region, req.InstanceID); err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
	} else {
		req.Region, req.InstanceID = "", ""
	}
	item.CloudConnectionID, item.CloudRegion, item.CloudInstanceID = req.ConnectionID, req.Region, req.InstanceID
	if err := h.store.DB().Save(item).Error; err != nil {
		apiShared.DBError(c, "保存监控配置失败")
		return
	}
	apiShared.Success(c, item)
}

func (h *DatabaseHandler) DatabaseMetrics(c *gin.Context) {
	item, ok := h.get(c)
	if !ok {
		return
	}
	if h.cloudMetrics == nil || item.CloudConnectionID == 0 {
		apiShared.ValidationError(c, "请先关联云账号、地域和 RDS 实例 ID")
		return
	}
	hours := 1
	if raw := c.Query("hours"); raw != "" {
		var err error
		hours, err = strconv.Atoi(raw)
		if err != nil {
			apiShared.BadRequest(c, "时间范围无效")
			return
		}
	}
	result, err := h.cloudMetrics.DatabaseMetrics(c.Request.Context(), item.CloudConnectionID, item.CloudRegion, item.CloudInstanceID, hours)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, result)
}
