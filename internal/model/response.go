package model

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// APIResponse 统一 API 响应结构体
type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// 错误码定义
const (
	// 成功
	CodeSuccess = 0

	// 客户端错误 4xxxx
	CodeBadRequest      = 40001 // 通用参数错误
	CodeValidationFail  = 40002 // 请求体验证失败
	CodeUnauthorized    = 40101 // 未认证
	CodeTokenExpired    = 40102 // Token 过期
	CodeForbidden       = 40301 // 无权限
	CodeNotFound        = 40401 // 资源不存在
	CodeConflict        = 40901 // 资源冲突（如重复名称）

	// 服务端错误 5xxxx
	CodeInternalError   = 50001 // 通用内部错误
	CodeDBError         = 50002 // 数据库操作失败
	CodeK8sUnavailable  = 50101 // K8s 集群未连接
	CodeK8sAPIError     = 50102 // K8s API 调用失败
)

// Success 统一成功响应
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, APIResponse{
		Code:    CodeSuccess,
		Message: "ok",
		Data:    data,
	})
}

// SuccessWithMessage 带自定义消息的成功响应。
// 适用于需要返回非默认成功消息的场景（如"创建成功"、"更新成功"）。
func SuccessWithMessage(c *gin.Context, data interface{}, msg string) {
	c.JSON(http.StatusOK, APIResponse{
		Code:    CodeSuccess,
		Message: msg,
		Data:    data,
	})
}

// Error 统一错误响应
func Error(c *gin.Context, httpStatus int, code int, msg string) {
	c.JSON(httpStatus, APIResponse{
		Code:    code,
		Message: msg,
		Data:    nil,
	})
}

// ErrorWithData 带附加数据的错误响应。
// 适用于需要在错误中携带附加信息（如校验失败字段列表）的场景。
func ErrorWithData(c *gin.Context, httpStatus int, code int, msg string, data interface{}) {
	c.JSON(httpStatus, APIResponse{
		Code:    code,
		Message: msg,
		Data:    data,
	})
}
