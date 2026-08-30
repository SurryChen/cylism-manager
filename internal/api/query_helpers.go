package api

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// optionalQueryID parses an optional numeric query parameter shared by
// application endpoints. Empty values intentionally mean "not specified".
func optionalQueryID(c *gin.Context, key string) (uint, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	return uint(id), err
}
