package shared

import "github.com/gin-gonic/gin"

// UserID returns the authenticated user ID attached by the auth middleware.
// Missing or malformed context values intentionally resolve to zero so callers
// preserve the existing unauthenticated behavior.
func UserID(c *gin.Context) uint {
	if id, exists := c.Get("user_id"); exists {
		if userID, ok := id.(uint); ok {
			return userID
		}
	}
	return 0
}
