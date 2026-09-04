package shared

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ParseID parses a numeric route identifier. It deliberately keeps the
// historical behavior used by application and delivery handlers: zero is
// accepted here and endpoint-specific validation remains with the caller.
func ParseID(value string) (uint, error) {
	id, err := strconv.ParseUint(value, 10, 64)
	return uint(id), err
}

// ParsePositiveID parses a route identifier and rejects zero, which is not a
// valid persisted resource ID for infrastructure endpoints.
func ParsePositiveID(value string) (uint, error) {
	id, err := ParseID(value)
	if err != nil || id == 0 {
		if err != nil {
			return 0, err
		}
		return 0, strconv.ErrSyntax
	}
	return id, nil
}

// K8sUnavailable writes the stable API response used when the Kubernetes
// boundary has not been configured or is unavailable.
func K8sUnavailable(c *gin.Context) {
	Error(c, http.StatusOK, CodeK8sUnavailable, "K8s 集群未连接")
}

// The following helpers keep status/code/message mapping at the API boundary
// consistent while allowing handlers to avoid repeating the same boilerplate.
func BadRequest(c *gin.Context, message string) {
	Error(c, http.StatusBadRequest, CodeBadRequest, message)
}
func ValidationError(c *gin.Context, message string) {
	Error(c, http.StatusBadRequest, CodeValidationFail, message)
}
func Unauthorized(c *gin.Context, message string) {
	Error(c, http.StatusUnauthorized, CodeUnauthorized, message)
}
func NotFound(c *gin.Context, message string) {
	Error(c, http.StatusNotFound, CodeNotFound, message)
}
func Conflict(c *gin.Context, message string) {
	Error(c, http.StatusConflict, CodeConflict, message)
}
func InternalError(c *gin.Context, message string) {
	Error(c, http.StatusInternalServerError, CodeInternalError, message)
}
func DBError(c *gin.Context, message string) {
	Error(c, http.StatusInternalServerError, CodeDBError, message)
}
func ServiceUnavailable(c *gin.Context, code int, message string) {
	Error(c, http.StatusServiceUnavailable, code, message)
}
func K8sAPIError(c *gin.Context, message string) {
	Error(c, http.StatusBadGateway, CodeK8sAPIError, message)
}
func K8sAPIErrorWithData(c *gin.Context, message string, data interface{}) {
	ErrorWithData(c, http.StatusBadGateway, CodeK8sAPIError, message, data)
}

// Pagination normalizes the common page/size query parameters used by list
// endpoints. Invalid or out-of-range values retain the API's historical
// defaults while keeping the upper bound consistent across handlers.
func Pagination(c *gin.Context) (page, size, offset int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ = strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size, (page - 1) * size
}

// LimitOffset normalizes endpoints that expose limit/offset directly.
func LimitOffset(c *gin.Context, defaultLimit, maxLimit int) (limit, offset int) {
	limit, _ = strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(defaultLimit)))
	offset, _ = strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit < 1 {
		limit = defaultLimit
	}
	if maxLimit > 0 && limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
