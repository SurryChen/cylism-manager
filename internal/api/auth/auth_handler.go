package authapi

import (
	apiShared "github.com/cylism/cylism-manager/internal/api/shared"

	"github.com/cylism/cylism-manager/internal/model"
	"time"

	"github.com/cylism/cylism-manager/internal/auth"
	"github.com/cylism/cylism-manager/internal/repository"
	authservice "github.com/cylism/cylism-manager/internal/service/auth"
	"github.com/gin-gonic/gin"
)

// AuthConfig configures browser authentication token issuance.
type AuthConfig struct {
	JWTSecret       []byte
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	AdminUser       string
	AdminPassword   string
	PlatformURL     string
}

type AuthHandler struct {
	users           repository.UserRepository
	jwtSecret       []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	temporaryTokens *authservice.TemporaryTokenService
}

// NewAuthHandlerWithTemporaryService uses a service composed by Bootstrap.
func NewAuthHandlerWithTemporaryService(users repository.UserRepository, jwtSecret []byte, accessTTL, refreshTTL time.Duration, temporaryTokens *authservice.TemporaryTokenService) *AuthHandler {
	return &AuthHandler{users: users, jwtSecret: jwtSecret, accessTokenTTL: accessTTL, refreshTokenTTL: refreshTTL, temporaryTokens: temporaryTokens}
}

// NewAuthHandlerWithDependencies constructs an AuthHandler from dependencies
// composed by Bootstrap. It does not create services.
func NewAuthHandlerWithDependencies(users repository.UserRepository, config *AuthConfig, temporaryTokens *authservice.TemporaryTokenService) *AuthHandler {
	if config == nil {
		return &AuthHandler{users: users, temporaryTokens: temporaryTokens}
	}
	return NewAuthHandlerWithTemporaryService(users, config.JWTSecret, config.AccessTokenTTL, config.RefreshTokenTTL, temporaryTokens)
}

type temporaryLoginReq struct {
	Token string `json:"token" binding:"required"`
}

// TemporaryLogin exchanges a managed temporary secret for a normal browser
// session. The secret itself is never returned or stored in the session.
func (h *AuthHandler) TemporaryLogin(c *gin.Context) {
	if h.temporaryTokens == nil {
		apiShared.Unauthorized(c, "临时登录秘钥不可用")
		return
	}
	var req temporaryLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "请提供临时登录秘钥")
		return
	}
	user, err := h.temporaryTokens.Redeem(req.Token)
	if err != nil {
		apiShared.Unauthorized(c, "临时登录秘钥无效或已过期")
		return
	}
	accessToken, refreshToken, err := authservice.GenerateSessionTokens(h.jwtSecret, user, h.accessTokenTTL, h.refreshTokenTTL)
	if err != nil {
		apiShared.InternalError(c, "生成 token 失败")
		return
	}
	model.Success(c, gin.H{"access_token": accessToken, "refresh_token": refreshToken, "user": gin.H{"id": user.ID, "username": user.Username}})
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
	model.Success(c, items)
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
	model.SuccessWithMessage(c, item, "临时登录秘钥已生成，请立即复制保存")
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
	model.SuccessWithMessage(c, gin.H{"id": id}, "临时登录秘钥已撤销")
}

type loginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login 登录 POST /api/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "请提供用户名和密码")
		return
	}

	user, err := h.users.GetUserByUsername(req.Username)
	if err != nil || !auth.CheckPassword(req.Password, user.PasswordHash) {
		apiShared.Unauthorized(c, "用户名或密码错误")
		return
	}

	accessToken, err := auth.GenerateAccessToken(h.jwtSecret, user.ID, user.Username, h.accessTokenTTL)
	if err != nil {
		apiShared.InternalError(c, "生成 token 失败")
		return
	}
	refreshToken, err := auth.GenerateRefreshToken(h.jwtSecret, user.ID, h.refreshTokenTTL)
	if err != nil {
		apiShared.InternalError(c, "生成 token 失败")
		return
	}

	model.Success(c, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
		},
	})
}

type refreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Refresh 刷新 token POST /api/auth/refresh
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "请提供 refresh_token")
		return
	}

	// 验证 refresh token
	claims, err := auth.ParseToken(h.jwtSecret, req.RefreshToken)
	if err != nil {
		apiShared.Unauthorized(c, "refresh token 无效或已过期")
		return
	}

	accessToken, err := auth.GenerateAccessToken(h.jwtSecret, claims.UserID, claims.Username, h.accessTokenTTL)
	if err != nil {
		apiShared.InternalError(c, "生成 token 失败")
		return
	}
	refreshToken, err := auth.GenerateRefreshToken(h.jwtSecret, claims.UserID, h.refreshTokenTTL)
	if err != nil {
		apiShared.InternalError(c, "生成 token 失败")
		return
	}

	model.Success(c, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

// Me 获取当前用户信息 GET /api/auth/me
func (h *AuthHandler) Me(c *gin.Context) {
	userID := apiShared.UserID(c)
	if userID == 0 {
		apiShared.Unauthorized(c, "未认证")
		return
	}
	model.Success(c, gin.H{
		"id":       userID,
		"username": apiShared.Username(c),
	})
}
