package cloud

import (
	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/gin-gonic/gin"
)

func (h *DatabaseHandler) DatabaseMetadata(c *gin.Context) {
	item, ok := h.get(c)
	if !ok {
		return
	}
	if h.operations == nil {
		apiShared.Success(c, item)
		return
	}
	latest, err := h.operations.Metadata(item.ID)
	if err != nil {
		apiShared.ValidationError(c, "读取实例信息失败")
		return
	}
	apiShared.Success(c, latest)
}
func (h *DatabaseHandler) RefreshDatabaseMetadata(c *gin.Context) {
	item, ok := h.get(c)
	if !ok {
		return
	}
	if h.operations == nil {
		apiShared.ValidationError(c, "数据库查询服务未配置")
		return
	}
	latest, err := h.operations.RefreshMetadata(c.Request.Context(), item.ID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, latest)
}
func (h *DatabaseHandler) RefreshDatabaseReplications(c *gin.Context) {
	item, ok := h.get(c)
	if !ok {
		return
	}
	if h.operations == nil {
		apiShared.ValidationError(c, "数据库查询服务未配置")
		return
	}
	relations, err := h.operations.RefreshReplications(c.Request.Context(), item.ID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, relations)
}
