package cloud

import (
	"context"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/gin-gonic/gin"
)

func (h *DatabaseHandler) UpdateDatabaseResources(c *gin.Context) {
	item, ok := h.get(c)
	if !ok {
		return
	}
	var req struct {
		CPU    string `json:"cpu_limit"`
		Memory string `json:"memory_limit"`
	}
	if c.ShouldBindJSON(&req) != nil {
		apiShared.BadRequest(c, "资源配置无效")
		return
	}
	if h.operations == nil {
		apiShared.ValidationError(c, "数据库后台任务服务未配置")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	task, err := h.operations.QueueResize(ctx, item.ID, req.CPU, req.Memory)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, task)
}
