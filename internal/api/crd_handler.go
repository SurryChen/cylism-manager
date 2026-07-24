package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CRDHandler CRD 依赖检测的 HTTP handler
type CRDHandler struct{}

// NewCRDHandler 创建 CRDHandler
func NewCRDHandler() *CRDHandler {
	return &CRDHandler{}
}

// CheckCRDs 检测必需 CRD 是否安装
func (h *CRDHandler) CheckCRDs(c *gin.Context) {
	// TODO: 调 K8s client 检测 CRD
	c.JSON(http.StatusOK, gin.H{
		"traefik_ok":     false,
		"cert_manager_ok": false,
		"message":        "CRD check - K8s integration pending",
	})
}
