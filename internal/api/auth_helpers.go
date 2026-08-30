package api

import "github.com/gin-gonic/gin"

// getUserID reads the authenticated browser identity from Gin context.
// Authentication middleware owns validation; handlers only need the ID.
func getUserID(c *gin.Context) uint {
	if id, exists := c.Get("user_id"); exists {
		if uid, ok := id.(uint); ok {
			return uid
		}
	}
	return 0
}
