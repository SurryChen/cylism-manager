package shared

import (
	auditservice "github.com/cylism/cylism-manager/internal/service/audit"
	"github.com/gin-gonic/gin"
)

// UserID returns the authenticated user ID attached by the auth middleware.
// Missing or malformed context values intentionally resolve to zero so callers
// preserve the existing unauthenticated behavior.
func UserID(c *gin.Context) uint {
	if id, exists := c.Get("user_id"); exists {
		switch userID := id.(type) {
		case uint:
			return userID
		case uint8:
			return uint(userID)
		case uint16:
			return uint(userID)
		case uint32:
			return uint(userID)
		case uint64:
			return uint(userID)
		case int:
			if userID > 0 {
				return uint(userID)
			}
		case int32:
			if userID > 0 {
				return uint(userID)
			}
		case int64:
			if userID > 0 {
				return uint(userID)
			}
		}
	}
	return 0
}

// Username returns the authenticated username attached by the auth
// middleware, or an empty string when the context is unauthenticated.
func Username(c *gin.Context) string {
	return c.GetString("username")
}

// ActorFromContext converts the authenticated Gin identity into the narrow
// audit service identity contract.
func ActorFromContext(c *gin.Context) auditservice.Actor {
	return auditservice.Actor{Type: "user", ID: UserID(c), Name: Username(c)}
}

// OptionalID parses an optional numeric query/route value. Empty values mean
// "not specified"; non-empty values must be positive identifiers.
func OptionalID(value string) (uint, error) {
	if value == "" {
		return 0, nil
	}
	return ParsePositiveID(value)
}
