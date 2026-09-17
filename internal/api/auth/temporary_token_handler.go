package authapi

import (
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	auditservice "github.com/cylism/cylism-manager/internal/service/audit"
	authservice "github.com/cylism/cylism-manager/internal/service/auth"
	"github.com/gin-gonic/gin"
)

type temporaryLoginReq struct {
	Token string `json:"token" binding:"required"`
}

// TemporaryLogin exchanges a managed temporary secret for a normal browser
// session. The secret itself is never returned or stored in the session.
func (h *AuthHandler) TemporaryLogin(c *gin.Context) {
	if h.temporaryTokens == nil {
		h.recordTemporaryLogin(c, 0, "", model.AuditOutcomeFailed, "临时登录凭据服务不可用")
		apiShared.Unauthorized(c, "临时登录秘钥不可用")
		return
	}
	var req temporaryLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		h.recordTemporaryLogin(c, 0, "", model.AuditOutcomeDenied, "临时登录凭据格式无效")
		apiShared.BadRequest(c, "请提供临时登录秘钥")
		return
	}
	user, err := h.temporaryTokens.Redeem(req.Token)
	if err != nil {
		h.recordTemporaryLogin(c, 0, "", model.AuditOutcomeDenied, "临时登录凭据被拒绝")
		apiShared.Unauthorized(c, "临时登录秘钥无效或已过期")
		return
	}
	accessToken, refreshToken, err := authservice.GenerateSessionTokens(h.jwtSecret, user, h.accessTokenTTL, h.refreshTokenTTL)
	if err != nil {
		h.recordTemporaryLogin(c, user.ID, user.Username, model.AuditOutcomeFailed, "签发临时登录会话失败")
		apiShared.InternalError(c, "生成 token 失败")
		return
	}
	h.recordTemporaryLogin(c, user.ID, user.Username, model.AuditOutcomeSucceeded, "使用临时登录凭据创建会话")
	apiShared.Success(c, gin.H{"access_token": accessToken, "refresh_token": refreshToken, "user": gin.H{"id": user.ID, "username": user.Username}})
}

type createTemporaryTokenReq struct {
	Label string `json:"label"`
	TTL   int64  `json:"ttl_seconds"`
}

func (h *AuthHandler) ListTemporaryTokens(c *gin.Context) {
	if h.temporaryTokens == nil {
		apiShared.InternalError(c, "临时登录秘钥不可用")
		return
	}
	items, err := h.temporaryTokens.List(apiShared.UserID(c))
	if err != nil {
		apiShared.InternalError(c, "读取临时登录秘钥失败")
		return
	}
	apiShared.Success(c, items)
}

func (h *AuthHandler) CreateTemporaryToken(c *gin.Context) {
	if h.temporaryTokens == nil {
		apiShared.InternalError(c, "临时登录秘钥不可用")
		return
	}
	var req createTemporaryTokenReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "临时登录秘钥配置无效")
		return
	}
	item, err := h.temporaryTokens.Create(apiShared.UserID(c), req.Label, time.Duration(req.TTL)*time.Second)
	if err != nil {
		apiShared.InternalError(c, "生成临时登录秘钥失败")
		return
	}
	h.recordTemporaryToken(c, "auth.temporary_token.create", item.ID, item.Label, model.AuditOutcomeSucceeded, "创建临时登录凭据")
	apiShared.SuccessWithMessage(c, item, "临时登录秘钥已生成，请立即复制保存")
}

func (h *AuthHandler) RevokeTemporaryToken(c *gin.Context) {
	if h.temporaryTokens == nil {
		apiShared.InternalError(c, "临时登录秘钥不可用")
		return
	}
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "秘钥 ID 无效")
		return
	}
	if err := h.temporaryTokens.Revoke(apiShared.UserID(c), id); err != nil {
		apiShared.InternalError(c, "撤销临时登录秘钥失败")
		return
	}
	h.recordTemporaryToken(c, "auth.temporary_token.revoke", id, "临时登录凭据", model.AuditOutcomeSucceeded, "撤销临时登录凭据")
	apiShared.SuccessWithMessage(c, gin.H{"id": id}, "临时登录秘钥已撤销")
}

func (h *AuthHandler) recordTemporaryLogin(c *gin.Context, userID uint, username, outcome, summary string) {
	h.record(c, "auth.temporary_login", userID, username, outcome, summary, nil)
}

func (h *AuthHandler) recordTemporaryToken(c *gin.Context, action string, tokenID uint, targetName, outcome, summary string) {
	if h == nil || h.audit == nil {
		return
	}
	_ = auditservice.NewService(h.audit).Record(auditservice.AuditEventInput{
		Action: action, ResourceType: "temporary_token", ResourceID: tokenID, TargetName: targetName,
		Actor: apiShared.ActorFromContext(c), Source: model.AuditSourceAPI, Outcome: outcome, Summary: summary,
		RequestID: apiShared.RequestID(c), Metadata: map[string]any{},
	})
}
