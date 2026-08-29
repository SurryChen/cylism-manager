package delivery

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	platformservice "github.com/cylism/cylism-manager/internal/service/platform"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PlatformHandler struct {
	store   *store.Store
	client  *k8sclient.Client
	release *platformservice.ReleaseService
}

func platformK8sUnavailable(c *gin.Context) {
	model.Error(c, http.StatusOK, model.CodeK8sUnavailable, "K8s 集群未连接")
}

type platformDeployRequest struct {
	Image     string `json:"image"`
	CommitSHA string `json:"commit_sha"`
	RunID     string `json:"run_id"`
}

type platformManualReleaseRequest struct {
	Image string `json:"image"`
}

type platformEndpointRequest struct {
	Hostname        string `json:"hostname"`
	CertificateName string `json:"certificate_name"`
	Enabled         bool   `json:"enabled"`
}

type platformEndpointInfo struct {
	Endpoint         model.PlatformEndpoint         `json:"endpoint"`
	URL              string                         `json:"url,omitempty"`
	State            string                         `json:"state"`
	IngressReady     bool                           `json:"ingress_ready"`
	Ingress          *k8sclient.PlatformIngressInfo `json:"ingress,omitempty"`
	Certificate      *k8sclient.CertInfo            `json:"certificate,omitempty"`
	CertificateError string                         `json:"certificate_error,omitempty"`
}

func NewPlatformHandler(s *store.Store, encKey []byte, client *k8sclient.Client) *PlatformHandler {
	return &PlatformHandler{store: s, client: client, release: platformservice.NewReleaseService(s, encKey, client)}
}

// Webhook accepts authenticated platform image deployment requests.
func (h *PlatformHandler) Webhook(c *gin.Context) {
	if h.store == nil {
		model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "部署签名无效或已过期")
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "读取部署请求失败")
		return
	}
	if !h.release.ValidateWebhook(c.GetHeader("X-Cylism-Timestamp"), c.GetHeader("X-Cylism-Nonce"), c.GetHeader("X-Cylism-Signature"), body) {
		model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "部署签名无效或已过期")
		return
	}
	var request platformDeployRequest
	if err := json.Unmarshal(body, &request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "部署请求无效")
		return
	}
	if err := h.release.ValidateImage(request.Image); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.release.ConsumeWebhookNonce(c.GetHeader("X-Cylism-Nonce"), time.Now()); err != nil {
		if errors.Is(err, platformservice.ErrWebhookReplay) {
			model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "部署签名无效或已过期")
		} else {
			model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "清理部署请求失败")
		}
		return
	}
	release, err := h.release.CreateRelease(request.Image, "github", request.CommitSHA, request.RunID)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, model.APIResponse{Code: model.CodeSuccess, Message: "平台发布已接受", Data: release})
	c.Writer.Flush()
	h.release.Apply(release.ID)
}

// ManualUpdate lets an authenticated administrator submit a tagged platform image.
func (h *PlatformHandler) ManualUpdate(c *gin.Context) {
	var request platformManualReleaseRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "平台镜像请求无效")
		return
	}
	if err := h.release.ValidateImage(request.Image); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	release, err := h.release.CreateRelease(request.Image, "manual", "", "")
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, model.APIResponse{Code: model.CodeSuccess, Message: "平台更新已提交", Data: release})
	c.Writer.Flush()
	h.release.Apply(release.ID)
}

func (h *PlatformHandler) Status(c *gin.Context) {
	if h.store == nil || h.client == nil {
		platformK8sUnavailable(c)
		return
	}
	h.release.ReconcileLatest()
	deployment, err := h.client.PlatformDeploymentStatus()
	if err != nil {
		model.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	releases, err := h.store.ListPlatformReleases(20)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "读取平台发布记录失败")
		return
	}
	prefixes := h.release.ImagePrefixes()
	_, configured := h.store.GetSystemConfig(platformservice.WebhookSecretConfigKey)
	model.Success(c, gin.H{"deployment": deployment, "releases": releases, "image_prefix": strings.Join(prefixes, "\n"), "image_prefixes": prefixes, "webhook_configured": configured == nil})
}

func (h *PlatformHandler) EndpointStatus(c *gin.Context) {
	model.Success(c, h.platformEndpointInfo())
}

// UpdateEndpoint stores and reconciles the public HTTPS entry for the Manager
// itself. It intentionally does not create an application or project endpoint.
func (h *PlatformHandler) UpdateEndpoint(c *gin.Context) {
	var request platformEndpointRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "平台入口定义无效")
		return
	}
	if h.store == nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "存储未初始化")
		return
	}
	current, err := h.store.GetPlatformEndpoint()
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "读取平台入口失败")
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		current = &model.PlatformEndpoint{}
	}
	endpoint := &model.PlatformEndpoint{
		Hostname:        strings.ToLower(strings.TrimSpace(request.Hostname)),
		CertificateName: strings.TrimSpace(request.CertificateName),
		Enabled:         request.Enabled,
	}
	if endpoint.Hostname == current.Hostname {
		endpoint.IngressName = current.IngressName
	}
	if !endpoint.Enabled {
		if endpoint.Hostname == "" {
			endpoint.Hostname = current.Hostname
		}
		if endpoint.CertificateName == "" {
			endpoint.CertificateName = current.CertificateName
		}
		if endpoint.TLSSecretName == "" {
			endpoint.TLSSecretName = current.TLSSecretName
		}
		endpoint.IngressName = current.IngressName
	}
	if endpoint.Enabled {
		if !validPlatformHostname(endpoint.Hostname) {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "管理域名必须是合法的精确 DNS 名称，且不支持泛域名")
			return
		}
		if err := h.validatePlatformEndpointPrerequisites(); err != nil {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
			return
		}
		certificate, err := h.platformEndpointCertificate(endpoint)
		if err != nil {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
			return
		}
		endpoint.TLSSecretName = certificate.SecretName
		if domain, lookupErr := h.store.GetManagedDomainByHostname(endpoint.Hostname); lookupErr == nil {
			model.Error(c, http.StatusConflict, model.CodeConflict, fmt.Sprintf("域名 %q 已作为项目受管域名使用", domain.Hostname))
			return
		} else if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "检查域名归属失败")
			return
		}
	}
	if err := h.store.SavePlatformEndpoint(endpoint); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "保存平台入口失败")
		return
	}
	if err := h.reconcilePlatformEndpoint(); err != nil {
		info := h.platformEndpointInfo()
		info.CertificateError = err.Error()
		model.SuccessWithMessage(c, info, "平台入口已保存，但 Kubernetes 同步尚未完成")
		return
	}
	model.SuccessWithMessage(c, h.platformEndpointInfo(), "平台入口已保存，正在同步证书和 Ingress")
}

func (h *PlatformHandler) ReconcileEndpoint(c *gin.Context) {
	if err := h.reconcilePlatformEndpoint(); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	model.SuccessWithMessage(c, h.platformEndpointInfo(), "平台入口已重新同步")
}

// AdoptEndpointIngress explicitly transfers a matching manually created
// platform Ingress to the platform controller.
func (h *PlatformHandler) AdoptEndpointIngress(c *gin.Context) {
	if h.store == nil || h.client == nil {
		platformK8sUnavailable(c)
		return
	}
	endpoint, err := h.store.GetPlatformEndpoint()
	if errors.Is(err, gorm.ErrRecordNotFound) || !endpoint.Enabled {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "请先保存并启用平台 HTTPS 入口")
		return
	}
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "读取平台入口失败")
		return
	}
	certificate, err := h.platformEndpointCertificate(endpoint)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	endpoint.TLSSecretName = certificate.SecretName
	if err := h.store.SavePlatformEndpoint(endpoint); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "保存平台入口失败")
		return
	}
	ingressName, err := h.client.AdoptPlatformIngress(endpoint.Hostname, endpoint.TLSSecretName)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	endpoint.IngressName = ingressName
	if err := h.store.SavePlatformEndpoint(endpoint); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "保存平台入口失败")
		return
	}
	model.SuccessWithMessage(c, h.platformEndpointInfo(), "现有 Ingress 已接管并重新同步")
}

func (h *PlatformHandler) GenerateWebhookSecret(c *gin.Context) {
	if h.store == nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "存储未初始化")
		return
	}
	secret, err := h.release.GenerateWebhookSecret()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "保存部署密钥失败")
		return
	}
	model.Success(c, gin.H{"secret": secret})
}

func (h *PlatformHandler) UpdateImagePrefix(c *gin.Context) {
	var request struct {
		ImagePrefix string `json:"image_prefix"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "平台镜像仓库前缀无效")
		return
	}
	_, err := h.release.SetImagePrefixes(request.ImagePrefix)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "平台镜像仓库前缀无效")
		return
	}
	model.Success(c, nil)
}

func (h *PlatformHandler) Rollback(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "发布记录 ID 无效")
		return
	}
	previous, err := h.store.GetPlatformRelease(uint(id))
	if err != nil || previous.PreviousImage == "" {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "该发布记录不能回滚")
		return
	}
	release, err := h.release.CreateRelease(previous.PreviousImage, "manual_rollback", "", "")
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	model.SuccessWithMessage(c, release, "平台回滚已提交")
	c.Writer.Flush()
	h.release.Apply(release.ID)
}

// Reconcile resumes a pending self-update after this service has restarted.
func (h *PlatformHandler) Reconcile() {
	_ = h.reconcilePlatformEndpoint()
	h.release.ReconcileLatest()
}

func (h *PlatformHandler) reconcilePlatformEndpoint() error {
	if h.store == nil || h.client == nil {
		return nil
	}
	endpoint, err := h.store.GetPlatformEndpoint()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !endpoint.Enabled {
		return h.client.RemovePlatformEndpoint(endpoint.IngressName)
	}
	certificate, err := h.platformEndpointCertificate(endpoint)
	if err != nil {
		return err
	}
	if endpoint.TLSSecretName != certificate.SecretName {
		endpoint.TLSSecretName = certificate.SecretName
		if err := h.store.SavePlatformEndpoint(endpoint); err != nil {
			return err
		}
	}
	return h.client.EnsurePlatformEndpoint(endpoint.Hostname, endpoint.TLSSecretName, endpoint.IngressName)
}

func (h *PlatformHandler) platformEndpointInfo() platformEndpointInfo {
	info := platformEndpointInfo{Endpoint: model.PlatformEndpoint{}, State: "not_configured"}
	if h.store == nil {
		info.CertificateError = "存储未初始化"
		return info
	}
	endpoint, err := h.store.GetPlatformEndpoint()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return info
	}
	if err != nil {
		info.State, info.CertificateError = "unavailable", "读取平台入口失败"
		return info
	}
	info.Endpoint = *endpoint
	if !endpoint.Enabled {
		info.State = "disabled"
		return info
	}
	info.URL = "https://" + endpoint.Hostname
	info.State = "waiting_certificate"
	if h.client == nil {
		info.State, info.CertificateError = "unavailable", "Kubernetes 客户端未初始化"
		return info
	}
	if ingress, ingressErr := h.client.PlatformIngressInfo(endpoint.IngressName); ingressErr != nil {
		info.CertificateError = ingressErr.Error()
	} else {
		info.Ingress = ingress
		info.IngressReady = ingress != nil && ingress.Managed
	}
	certificate, certificateErr := h.client.GetCertificate("default", endpoint.CertificateName)
	if certificateErr != nil {
		info.CertificateError = certificateErr.Error()
		return info
	}
	info.Certificate = certificate
	if certificate.Status == "Ready" && info.IngressReady {
		info.State = "ready"
	} else if certificate.Status == "Ready" {
		info.State = "waiting_ingress"
	} else if certificate.Status == "Failed" {
		info.State = "failed"
	}
	return info
}

func (h *PlatformHandler) platformEndpointCertificate(endpoint *model.PlatformEndpoint) (*k8sclient.CertInfo, error) {
	if h.client == nil {
		return nil, errors.New("Kubernetes 客户端未初始化")
	}
	if strings.TrimSpace(endpoint.CertificateName) == "" {
		return nil, errors.New("请选择 default 命名空间中已就绪的 TLS 证书")
	}
	certificate, err := h.client.GetCertificate("default", endpoint.CertificateName)
	if err != nil {
		return nil, fmt.Errorf("读取 TLS 证书: %w", err)
	}
	if certificate.Status != "Ready" {
		return nil, errors.New("所选 TLS 证书尚未就绪")
	}
	if strings.TrimSpace(certificate.SecretName) == "" {
		return nil, errors.New("所选 TLS 证书未生成 Secret")
	}
	for _, domain := range certificate.Domains {
		if certificateCoversHostname(domain, endpoint.Hostname) {
			return certificate, nil
		}
	}
	return nil, fmt.Errorf("所选 TLS 证书不覆盖域名 %q", endpoint.Hostname)
}

func certificateCoversHostname(certificateDomain, hostname string) bool {
	certificateDomain = strings.ToLower(strings.TrimSpace(certificateDomain))
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if certificateDomain == hostname {
		return true
	}
	if !strings.HasPrefix(certificateDomain, "*.") {
		return false
	}
	suffix := strings.TrimPrefix(certificateDomain, "*")
	if !strings.HasSuffix(hostname, suffix) {
		return false
	}
	prefix := strings.TrimSuffix(hostname, suffix)
	return prefix != "" && !strings.Contains(prefix, ".")
}

func (h *PlatformHandler) validatePlatformEndpointPrerequisites() error {
	if h.client == nil || h.client.Clientset == nil {
		return errors.New("Kubernetes 客户端未初始化")
	}
	status, err := h.client.DetectIngressController()
	if err != nil || status == nil || !status.Running {
		return errors.New("Ingress Controller 未就绪")
	}
	return nil
}

func validPlatformHostname(hostname string) bool {
	if len(hostname) < 3 || len(hostname) > 253 || !strings.Contains(hostname, ".") || strings.HasPrefix(hostname, "*.") {
		return false
	}
	for _, label := range strings.Split(hostname, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-') {
				return false
			}
		}
	}
	return true
}
