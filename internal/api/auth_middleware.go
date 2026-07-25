package api

import (
	"net/http"

	"strings"

	"github.com/cylism/cylism-manager/internal/auth"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

// JWTAuthMiddleware JWT 认证中间件
func JWTAuthMiddleware(secret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "未提供认证 token")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "认证格式错误")
			c.Abort()
			return
		}

		claims, err := auth.ParseToken(secret, parts[1])
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
