package cloud

import (
	"net/http"

	shared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	cloudservice "github.com/cylism/cylism-manager/internal/service/cloud"
	"github.com/gin-gonic/gin"
)

func (h *Handler) GetBucketPublicAccess(c *gin.Context) {
	id, ok := connectionID(c)
	if !ok {
		return
	}
	config, err := h.service.GetBucketPublicAccess(c.Request.Context(), id, c.Param("container"), c.Query("region"))
	if err != nil {
		providerError(c, err)
		return
	}
	shared.Success(c, config)
}

func (h *Handler) SetBucketPublicAccess(c *gin.Context) {
	id, ok := connectionID(c)
	if !ok {
		return
	}
	var input struct {
		BlockPublicAccess *bool `json:"block_public_access"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := c.ShouldBindJSON(&input); err != nil || input.BlockPublicAccess == nil {
		shared.BadRequest(c, "请选择是否阻止公共访问")
		return
	}
	config := cloudservice.BucketPublicAccess{BlockPublicAccess: *input.BlockPublicAccess}
	bucket := c.Param("container")
	const action = "cloud.object_storage.container.public_access.update"
	summary := "关闭桶级阻止公共访问"
	if config.BlockPublicAccess {
		summary = "开启桶级阻止公共访问"
	}
	if err := h.service.SetBucketPublicAccess(c.Request.Context(), id, bucket, c.Query("region"), config); err != nil {
		h.record(c, action, id, bucket, model.AuditOutcomeFailed, summary+"失败")
		providerError(c, err)
		return
	}
	h.record(c, action, id, bucket, model.AuditOutcomeSucceeded, summary)
	shared.Success(c, config)
}
