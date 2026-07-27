package api

import (
	"net/http"

	"strings"

	"github.com/cylism/cylism-manager/internal/auth"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

// JWTAuthMiddleware JWT 认证中间件（支持 Header Bearer 和 Query ?token=）
func JWTAuthMiddleware(secret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := ""

		// 优先从 Authorization header 读取
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenStr = parts[1]
			}
		}

		// fallback: query string ?token=（WebSocket 握手用）
		if tokenStr == "" {
			tokenStr = c.Query("token")
		}

		if tokenStr == "" {
			model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "未提供认证 token")
			c.Abort()
			return
		}

		claims, err := auth.ParseToken(secret, tokenStr)
		if err != nil {
			model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "token 无效或已过期")
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Next()
	}
}