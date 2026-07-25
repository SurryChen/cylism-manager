package api

import (
	"net/http"

	"github.com/cylism/cylism-manager/internal/model"

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
		model.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, certs)
}

// CreateCert 创建 Certificate
func (h *CertHandler) CreateCert(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	model.SuccessWithMessage(c, nil, "创建证书 - 待实现")
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
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
		return
	}
	model.SuccessWithMessage(c, nil, "删除成功")
}
