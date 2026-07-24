package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CertHandler Certificate 管理的 HTTP handler
type CertHandler struct{}

// NewCertHandler 创建 CertHandler
func NewCertHandler() *CertHandler {
	return &CertHandler{}
}

// ListCerts 列出所有 Certificate
func (h *CertHandler) ListCerts(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	certs, err := K8s.ListCertificates()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "certs": []interface{}{}})
		return
	}
	c.JSON(http.StatusOK, certs)
}

// CreateCert 创建 Certificate
func (h *CertHandler) CreateCert(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "创建证书 - 待实现"})
}

// DeleteCert 删除 Certificate
func (h *CertHandler) DeleteCert(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	if err := K8s.DeleteCertificate(ns, name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}
