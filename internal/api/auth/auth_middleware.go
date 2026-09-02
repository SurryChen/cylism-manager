package authapi

import (
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"

	"github.com/cylism/cylism-manager/internal/auth"
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
			apiShared.Unauthorized(c, "未提供认证 token")
			c.Abort()
			return
		}

		claims, err := auth.ParseToken(secret, tokenStr)
		if err != nil {
			apiShared.Unauthorized(c, "token 无效或已过期")
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Next()
	}
}

// DelegationAuthMiddleware only accepts an explicit integration delegation.
// It intentionally cannot parse ordinary browser JWT claims.
func DelegationAuthMiddleware(secret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" || strings.TrimSpace(parts[1]) == "" {
			apiShared.Unauthorized(c, "未提供委托 token")
			c.Abort()
			return
		}
		claims, err := auth.ParseDelegationToken(secret, parts[1])
		if err != nil {
			apiShared.Unauthorized(c, "委托 token 无效或已过期")
			c.Abort()
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("delegation", claims)
		c.Next()
	}
}
