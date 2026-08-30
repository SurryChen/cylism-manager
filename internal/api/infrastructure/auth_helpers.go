package infrastructure

import "github.com/gin-gonic/gin"

// getUserID returns the authenticated user ID attached by the auth middleware.
// Infrastructure handlers use zero when the context has no authenticated user.
func getUserID(c *gin.Context) uint {
	if id, exists := c.Get("user_id"); exists {
		if userID, ok := id.(uint); ok {
			return userID
		}
	}
	return 0
}
