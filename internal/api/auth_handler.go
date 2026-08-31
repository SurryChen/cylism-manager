package api

import (
	"net/http"

	"github.com/cylism/cylism-manager/internal/model"
	"time"

	"github.com/cylism/cylism-manager/internal/auth"
	"github.com/cylism/cylism-manager/internal/repository"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	users           repository.UserRepository
	jwtSecret       []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

func NewAuthHandler(users repository.UserRepository, jwtSecret []byte, accessTTL, refreshTTL time.Duration) *AuthHandler {
	return &AuthHandler{
		users:           users,
		jwtSecret:       jwtSecret,
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
	}
}

type loginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login 登录 POST /api/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "请提供用户名和密码")
		return
	}

	user, err := h.users.GetUserByUsername(req.Username)
	if err != nil || !auth.CheckPassword(req.Password, user.PasswordHash) {
		model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "用户名或密码错误")
		return
	}

	accessToken, err := auth.GenerateAccessToken(h.jwtSecret, user.ID, user.Username, h.accessTokenTTL)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "生成 token 失败")
		return
	}
	refreshToken, err := auth.GenerateRefreshToken(h.jwtSecret, user.ID, h.refreshTokenTTL)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "生成 token 失败")
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
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "请提供 refresh_token")
		return
	}

	// 验证 refresh token
	claims, err := auth.ParseToken(h.jwtSecret, req.RefreshToken)
	if err != nil {
		model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "refresh token 无效或已过期")
		return
	}

	accessToken, err := auth.GenerateAccessToken(h.jwtSecret, claims.UserID, claims.Username, h.accessTokenTTL)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "生成 token 失败")
		return
	}
	refreshToken, err := auth.GenerateRefreshToken(h.jwtSecret, claims.UserID, h.refreshTokenTTL)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "生成 token 失败")
		return
	}

	model.Success(c, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

// Me 获取当前用户信息 GET /api/auth/me
func (h *AuthHandler) Me(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "未认证")
		return
	}
	username, _ := c.Get("username")
	model.Success(c, gin.H{
		"id":       userID,
		"username": username,
	})
}
