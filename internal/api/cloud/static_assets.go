package cloud

import (
	"net/http"

	shared "github.com/cylism/cylism-manager/internal/api/shared"
	cloudservice "github.com/cylism/cylism-manager/internal/service/cloud"
	"github.com/gin-gonic/gin"
)

func (h *Handler) WithStaticAssets(service *cloudservice.StaticAssetService) *Handler {
	h.staticAssets = service
	return h
}

func (h *Handler) StaticAssetSettings(c *gin.Context) {
	settings, err := h.staticAssets.Settings()
	if err != nil {
		shared.DBError(c, "读取静态资源配置失败")
		return
	}
	shared.Success(c, settings)
}

func (h *Handler) SaveStaticAssetSettings(c *gin.Context) {
	var input cloudservice.StaticAssetSettings
	if !bindStaticAssetRequest(c, &input) {
		return
	}
	settings, err := h.staticAssets.SaveSettings(c.Request.Context(), input)
	if err != nil {
		shared.ValidationError(c, err.Error())
		return
	}
	shared.Success(c, settings)
}

func (h *Handler) SignStaticAsset(c *gin.Context) {
	var input struct {
		ContentType string `json:"content_type"`
		Size        int64  `json:"size"`
	}
	if !bindStaticAssetRequest(c, &input) {
		return
	}
	upload, err := h.staticAssets.Issue(shared.UserID(c), input.ContentType, input.Size)
	if err != nil {
		shared.ValidationError(c, err.Error())
		return
	}
	c.Header("Cache-Control", "no-store")
	shared.Success(c, upload)
}

func (h *Handler) PublishStaticAsset(c *gin.Context) {
	var input struct {
		Ticket string `json:"ticket"`
	}
	if !bindStaticAssetRequest(c, &input) {
		return
	}
	publicURL, err := h.staticAssets.Publish(c.Request.Context(), shared.UserID(c), input.Ticket)
	if err != nil {
		shared.ValidationError(c, err.Error())
		return
	}
	shared.Success(c, gin.H{"url": publicURL})
}

func bindStaticAssetRequest(c *gin.Context, input any) bool {
	if shared.UserID(c) == 0 {
		shared.Unauthorized(c, "请先登录")
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := c.ShouldBindJSON(input); err != nil {
		shared.BadRequest(c, "静态资源请求无效")
		return false
	}
	return true
}
