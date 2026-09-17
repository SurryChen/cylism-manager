package authapi

import (
	apiShared "github.com/cylism/cylism-manager/internal/api/shared"

	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	auditservice "github.com/cylism/cylism-manager/internal/service/audit"
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
	audit           auditservice.Repository
}

func (h *AuthHandler) WithAudit(repository auditservice.Repository) *AuthHandler {
	h.audit = repository
	return h
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
	if err != nil || !authservice.CheckPassword(req.Password, user.PasswordHash) {
		h.record(c, "auth.login", 0, req.Username, model.AuditOutcomeDenied, "登录失败", nil)
		apiShared.Unauthorized(c, "用户名或密码错误")
		return
	}

	accessToken, err := authservice.GenerateAccessToken(h.jwtSecret, user.ID, user.Username, h.accessTokenTTL)
	if err != nil {
		h.record(c, "auth.login", user.ID, user.Username, model.AuditOutcomeFailed, "签发登录令牌失败", nil)
		apiShared.InternalError(c, "生成 token 失败")
		return
	}
	refreshToken, err := authservice.GenerateRefreshToken(h.jwtSecret, user.ID, h.refreshTokenTTL)
	if err != nil {
		h.record(c, "auth.login", user.ID, user.Username, model.AuditOutcomeFailed, "签发刷新令牌失败", nil)
		apiShared.InternalError(c, "生成 token 失败")
		return
	}
	h.record(c, "auth.login", user.ID, user.Username, model.AuditOutcomeSucceeded, "用户登录", nil)

	apiShared.Success(c, gin.H{
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
	claims, err := authservice.ParseToken(h.jwtSecret, req.RefreshToken)
	if err != nil {
		h.record(c, "auth.token.refresh", 0, "", model.AuditOutcomeDenied, "刷新令牌被拒绝", nil)
		apiShared.Unauthorized(c, "refresh token 无效或已过期")
		return
	}

	accessToken, err := authservice.GenerateAccessToken(h.jwtSecret, claims.UserID, claims.Username, h.accessTokenTTL)
	if err != nil {
		h.record(c, "auth.token.refresh", claims.UserID, claims.Username, model.AuditOutcomeFailed, "签发访问令牌失败", nil)
		apiShared.InternalError(c, "生成 token 失败")
		return
	}
	refreshToken, err := authservice.GenerateRefreshToken(h.jwtSecret, claims.UserID, h.refreshTokenTTL)
	if err != nil {
		h.record(c, "auth.token.refresh", claims.UserID, claims.Username, model.AuditOutcomeFailed, "签发刷新令牌失败", nil)
		apiShared.InternalError(c, "生成 token 失败")
		return
	}
	h.record(c, "auth.token.refresh", claims.UserID, claims.Username, model.AuditOutcomeSucceeded, "刷新登录令牌", nil)

	apiShared.Success(c, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

func (h *AuthHandler) record(c *gin.Context, action string, userID uint, username, outcome, summary string, metadata map[string]any) {
	if h == nil || h.audit == nil {
		return
	}
	_ = auditservice.NewService(h.audit).Record(auditservice.AuditEventInput{
		Action: action, ResourceType: "identity", ResourceID: userID, TargetName: username,
		Actor: auditservice.Actor{Type: model.AuditActorUser, ID: userID, Name: username}, Source: model.AuditSourceAPI,
		Outcome: outcome, Summary: summary, RequestID: apiShared.RequestID(c), Metadata: metadata,
	})
}

// Me 获取当前用户信息 GET /api/auth/me
func (h *AuthHandler) Me(c *gin.Context) {
	userID := apiShared.UserID(c)
	if userID == 0 {
		apiShared.Unauthorized(c, "未认证")
		return
	}
	apiShared.Success(c, gin.H{
		"id":       userID,
		"username": apiShared.Username(c),
	})
}
