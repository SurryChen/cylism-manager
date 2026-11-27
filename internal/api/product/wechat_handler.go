package productapi

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/service/wechat"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handler) WithWeChat(service *wechat.Service) *Handler {
	h.wechat = service
	return h
}

func (h *Handler) weChatProduct(c *gin.Context) (uint, bool) {
	product, ok := lifecycleID(c)
	if !ok {
		return 0, false
	}
	if h.wechat == nil {
		shared.NotFound(c, "微信集成不可用")
		return 0, false
	}
	c.Header("Cache-Control", "no-store")
	return product, true
}

func (h *Handler) weChatChannel(c *gin.Context) (uint, uint, bool) {
	product, ok := h.weChatProduct(c)
	if !ok {
		return 0, 0, false
	}
	channel, err := shared.ParsePositiveID(c.Param("channelID"))
	if err != nil {
		shared.BadRequest(c, "发行渠道 ID 无效")
		return 0, 0, false
	}
	return product, channel, true
}

func weChatError(c *gin.Context, err error) {
	var remote wechat.APIError
	switch {
	case errors.Is(err, model.ErrWeChatChannel), errors.Is(err, model.ErrWeChatAppID), errors.Is(err, wechat.ErrAppID),
		errors.Is(err, wechat.ErrAppSecret), errors.Is(err, wechat.ErrScene):
		shared.ValidationError(c, err.Error())
	case errors.Is(err, wechat.ErrAnalyticsNotConfigured), errors.Is(err, wechat.ErrComponentUnavailable),
		errors.Is(err, wechat.ErrTicketUnavailable), errors.Is(err, model.ErrWeChatAuthorization):
		shared.Error(c, http.StatusConflict, shared.CodeConflict, err.Error())
	case errors.Is(err, gorm.ErrRecordNotFound):
		shared.NotFound(c, "微信连接不存在")
	case errors.As(err, &remote):
		shared.Error(c, http.StatusBadGateway, shared.CodeBadRequest, remote.Error())
	default:
		shared.DBError(c, "微信集成操作失败")
	}
}

func (h *Handler) WeChatSettings(c *gin.Context) {
	product, ok := h.weChatProduct(c)
	if !ok {
		return
	}
	result, err := h.wechat.Settings(shared.UserID(c), product)
	if err != nil {
		weChatError(c, err)
		return
	}
	shared.Success(c, result)
}

func (h *Handler) SaveWeChatConnection(c *gin.Context) {
	product, channel, ok := h.weChatChannel(c)
	if !ok {
		return
	}
	var input struct {
		AppID  string `json:"app_id"`
		Secret string `json:"app_secret"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := c.ShouldBindJSON(&input); err != nil {
		shared.BadRequest(c, "微信连接配置无效")
		return
	}
	if err := h.wechat.Connect(shared.UserID(c), product, channel, input.AppID, input.Secret); err != nil {
		weChatError(c, err)
		return
	}
	shared.Success(c, nil)
}

func (h *Handler) DeleteWeChatConnection(c *gin.Context) {
	product, channel, ok := h.weChatChannel(c)
	if !ok {
		return
	}
	if err := h.wechat.Disconnect(shared.UserID(c), product, channel); err != nil {
		weChatError(c, err)
		return
	}
	shared.Success(c, nil)
}

func (h *Handler) AuthorizeWeChatRelease(c *gin.Context) {
	product, channel, ok := h.weChatChannel(c)
	if !ok {
		return
	}
	target, err := h.wechat.StartAuthorization(c.Request.Context(), shared.UserID(c), product, channel)
	if err != nil {
		weChatError(c, err)
		return
	}
	shared.Success(c, gin.H{"url": target})
}

func (h *Handler) SyncWeChatRelease(c *gin.Context) {
	product, channel, ok := h.weChatChannel(c)
	if !ok {
		return
	}
	if err := h.wechat.SyncRelease(c.Request.Context(), shared.UserID(c), product, channel); err != nil {
		weChatError(c, err)
		return
	}
	result, err := h.wechat.Settings(shared.UserID(c), product)
	if err != nil {
		weChatError(c, err)
		return
	}
	shared.Success(c, result)
}

func (h *Handler) WeChatAnalytics(c *gin.Context) {
	product, channel, ok := h.weChatChannel(c)
	if !ok {
		return
	}
	days := 7
	switch c.Query("days") {
	case "", "7":
	case "30":
		days = 30
	default:
		shared.BadRequest(c, "请选择 7 天或 30 天")
		return
	}
	result, err := h.wechat.Analytics(c.Request.Context(), shared.UserID(c), product, channel, days, c.Request.Method == http.MethodPost)
	if err != nil {
		weChatError(c, err)
		return
	}
	shared.Success(c, result)
}

func (h *Handler) WeChatPromotionCode(c *gin.Context) {
	product, channel, ok := h.weChatChannel(c)
	if !ok {
		return
	}
	var input struct {
		Scene string `json:"scene"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	if err := c.ShouldBindJSON(&input); err != nil {
		shared.BadRequest(c, "推广码参数无效")
		return
	}
	image, mime, err := h.wechat.PromotionCode(c.Request.Context(), shared.UserID(c), product, channel, input.Scene)
	if err != nil {
		weChatError(c, err)
		return
	}
	shared.Success(c, gin.H{"image": base64.StdEncoding.EncodeToString(image), "mime": mime})
}

func (h *Handler) WeChatComponentEvent(c *gin.Context) {
	if h.wechat == nil {
		c.Status(http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	err = h.wechat.ReceiveEvent(body, c.Query("timestamp"), c.Query("nonce"), c.Query("msg_signature"))
	switch {
	case err == nil:
		c.String(http.StatusOK, "success")
	case errors.Is(err, wechat.ErrInvalidEvent):
		c.Status(http.StatusForbidden)
	case errors.Is(err, wechat.ErrComponentUnavailable):
		c.Status(http.StatusNotFound)
	default:
		c.Status(http.StatusServiceUnavailable)
	}
}

func (h *Handler) WeChatAuthorizationReturn(c *gin.Context) {
	if h.wechat == nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	item, err := h.wechat.FinishAuthorization(c.Request.Context(), c.Query("state"), c.Query("auth_code"))
	if item.ProductID == 0 {
		c.String(http.StatusBadRequest, "微信授权已失效，请返回项目发行页重新授权。")
		return
	}
	status := "connected"
	if errors.Is(err, wechat.ErrReleaseScope) {
		status = "missing-scope"
	} else if err != nil {
		status = "failed"
	}
	c.Redirect(http.StatusSeeOther, fmt.Sprintf("/#/products/%d?tab=distributions&wechat_authorization=%s&wechat_channel=%d", item.ProductID, status, item.ChannelID))
}
