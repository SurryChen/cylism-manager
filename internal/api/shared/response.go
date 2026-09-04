package shared

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// APIResponse is the stable envelope returned by HTTP endpoints.
// It belongs to the API layer rather than the persistence/domain model.
type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// API error codes shared by all HTTP handlers.
const (
	CodeSuccess = 0

	CodeBadRequest     = 40001
	CodeValidationFail = 40002
	CodeUnauthorized   = 40101
	CodeTokenExpired   = 40102
	CodeForbidden      = 40301
	CodeNotFound       = 40401
	CodeConflict       = 40901

	CodeInternalError  = 50001
	CodeDBError        = 50002
	CodeK8sUnavailable = 50101
	CodeK8sAPIError    = 50102
)

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, APIResponse{Code: CodeSuccess, Message: "ok", Data: data})
}

func SuccessWithMessage(c *gin.Context, data interface{}, message string) {
	c.JSON(http.StatusOK, APIResponse{Code: CodeSuccess, Message: message, Data: data})
}

func Error(c *gin.Context, status, code int, message string) {
	c.JSON(status, APIResponse{Code: code, Message: message, Data: nil})
}

func ErrorWithData(c *gin.Context, status, code int, message string, data interface{}) {
	c.JSON(status, APIResponse{Code: code, Message: message, Data: data})
}
