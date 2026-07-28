package api

import (
	"net/http"
	"strings"

	"github.com/cylism/cylism-manager/internal/k8s"
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
	var request k8s.CreateCertificateRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Name == "" || request.Namespace == "" || request.IssuerRef == "" || len(request.Domains) == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "证书名称、命名空间、域名和 Issuer 必填")
		return
	}
	if request.IssuerKind != "" && request.IssuerKind != "Issuer" && request.IssuerKind != "ClusterIssuer" {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "Issuer 类型必须为 Issuer 或 ClusterIssuer")
		return
	}
	for _, domain := range request.Domains {
		if strings.TrimSpace(domain) == "" {
			model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "域名不能为空")
			return
		}
	}
	certificate, err := K8s.CreateCertificate(request)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, certificate)
}

func (h *CertHandler) ListIssuers(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	issuers, err := K8s.ListIssuers()
	if err != nil {
		model.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, issuers)
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
