package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	platformservice "github.com/cylism/cylism-manager/internal/service/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PlatformHandler struct {
	store   repository.PlatformEndpointRepository
	client  platformKubernetes
	release *platformservice.ReleaseService
}

// PlatformKubernetesAdapter is the narrow Kubernetes port required by the
// platform delivery handler. Bootstrap creates the concrete implementation.
type PlatformKubernetesAdapter interface {
	platformservice.PlatformAdapter
	AdoptPlatformIngressContext(context.Context, string, string) (string, error)
	RemovePlatformEndpointContext(context.Context, string) error
	EnsurePlatformEndpointContext(context.Context, string, string, string) error
	PlatformIngressInfoContext(context.Context, string) (*k8sclient.PlatformIngressInfo, error)
	GetCertificateContext(context.Context, string, string) (*k8sclient.CertInfo, error)
	DetectIngressControllerContext(context.Context) (*k8sclient.IngressControllerStatus, error)
	KubernetesAvailable() bool
}

type platformKubernetes = PlatformKubernetesAdapter

// NewPlatformKubernetesAdapter narrows a concrete client before it reaches
// the HTTP handler.
func NewPlatformKubernetesAdapter(client *k8sclient.Client) PlatformKubernetesAdapter {
	if client == nil {
		return nil
	}
	return client
}

func platformK8sUnavailable(c *gin.Context) {
	apiShared.K8sUnavailable(c)
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

func NewPlatformHandler(s repository.PlatformEndpointRepository, encKey []byte, client platformKubernetes) *PlatformHandler {
	return NewPlatformHandlerWithService(s, encKey, client, platformservice.NewReleaseService(s, encKey, client))
}

func NewPlatformHandlerWithService(s repository.PlatformEndpointRepository, encKey []byte, client platformKubernetes, release *platformservice.ReleaseService) *PlatformHandler {
	if release == nil {
		release = platformservice.NewReleaseService(s, encKey, client)
	}
	return &PlatformHandler{store: s, client: client, release: release}
}

// Webhook accepts authenticated platform image deployment requests.
func (h *PlatformHandler) Webhook(c *gin.Context) {
	if h.store == nil {
		apiShared.Unauthorized(c, "部署签名无效或已过期")
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
	if err != nil {
		apiShared.BadRequest(c, "读取部署请求失败")
		return
	}
	if !h.release.ValidateWebhook(c.GetHeader("X-Cylism-Timestamp"), c.GetHeader("X-Cylism-Nonce"), c.GetHeader("X-Cylism-Signature"), body) {
		apiShared.Unauthorized(c, "部署签名无效或已过期")
		return
	}
	var request platformDeployRequest
	if err := json.Unmarshal(body, &request); err != nil {
		apiShared.BadRequest(c, "部署请求无效")
		return
	}
	if err := h.release.ValidateImage(request.Image); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.release.ConsumeWebhookNonce(c.GetHeader("X-Cylism-Nonce"), time.Now()); err != nil {
		if errors.Is(err, platformservice.ErrWebhookReplay) {
			apiShared.Unauthorized(c, "部署签名无效或已过期")
		} else {
			apiShared.InternalError(c, "清理部署请求失败")
		}
		return
	}
	release, err := h.release.CreateRelease(c.Request.Context(), request.Image, "github", request.CommitSHA, request.RunID)
	if err != nil {
		apiShared.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, model.APIResponse{Code: model.CodeSuccess, Message: "平台发布已接受", Data: release})
	c.Writer.Flush()
	h.release.Apply(context.WithoutCancel(c.Request.Context()), release.ID)
}

// ManualUpdate lets an authenticated administrator submit a tagged platform image.
func (h *PlatformHandler) ManualUpdate(c *gin.Context) {
	var request platformManualReleaseRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "平台镜像请求无效")
		return
	}
	if err := h.release.ValidateImage(request.Image); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	release, err := h.release.CreateRelease(c.Request.Context(), request.Image, "manual", "", "")
	if err != nil {
		apiShared.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, model.APIResponse{Code: model.CodeSuccess, Message: "平台更新已提交", Data: release})
	c.Writer.Flush()
	h.release.Apply(context.WithoutCancel(c.Request.Context()), release.ID)
}

func (h *PlatformHandler) Status(c *gin.Context) {
	if h.store == nil || h.client == nil || !h.client.KubernetesAvailable() {
		platformK8sUnavailable(c)
		return
	}
	h.release.ReconcileLatest(c.Request.Context())
	deployment, err := h.client.PlatformDeploymentStatusContext(c.Request.Context())
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	releases, err := h.store.ListPlatformReleases(20)
	if err != nil {
		apiShared.InternalError(c, "读取平台发布记录失败")
		return
	}
	prefixes := h.release.ImagePrefixes()
	_, configured := h.store.GetSystemConfig(platformservice.WebhookSecretConfigKey)
	model.Success(c, gin.H{"deployment": deployment, "releases": releases, "image_prefix": strings.Join(prefixes, "\n"), "image_prefixes": prefixes, "webhook_configured": configured == nil})
}

func (h *PlatformHandler) EndpointStatus(c *gin.Context) {
	model.Success(c, h.platformEndpointInfo(c.Request.Context()))
}

// UpdateEndpoint stores and reconciles the public HTTPS entry for the Manager
// itself. It intentionally does not create an application or project endpoint.
func (h *PlatformHandler) UpdateEndpoint(c *gin.Context) {
	var request platformEndpointRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "平台入口定义无效")
		return
	}
	if h.store == nil {
		apiShared.InternalError(c, "存储未初始化")
		return
	}
	current, err := h.store.GetPlatformEndpoint()
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.InternalError(c, "读取平台入口失败")
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
			apiShared.ValidationError(c, "管理域名必须是合法的精确 DNS 名称，且不支持泛域名")
			return
		}
		if err := h.validatePlatformEndpointPrerequisites(c.Request.Context()); err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
		certificate, err := h.platformEndpointCertificate(c.Request.Context(), endpoint)
		if err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
		endpoint.TLSSecretName = certificate.SecretName
		if domain, lookupErr := h.store.GetManagedDomainByHostname(endpoint.Hostname); lookupErr == nil {
			apiShared.Conflict(c, fmt.Sprintf("域名 %q 已作为项目受管域名使用", domain.Hostname))
			return
		} else if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			apiShared.InternalError(c, "检查域名归属失败")
			return
		}
	}
	if err := h.store.SavePlatformEndpoint(endpoint); err != nil {
		apiShared.InternalError(c, "保存平台入口失败")
		return
	}
	if err := h.reconcilePlatformEndpoint(c.Request.Context()); err != nil {
		info := h.platformEndpointInfo(c.Request.Context())
		info.CertificateError = err.Error()
		model.SuccessWithMessage(c, info, "平台入口已保存，但 Kubernetes 同步尚未完成")
		return
	}
	model.SuccessWithMessage(c, h.platformEndpointInfo(c.Request.Context()), "平台入口已保存，正在同步证书和 Ingress")
}

func (h *PlatformHandler) ReconcileEndpoint(c *gin.Context) {
	if err := h.reconcilePlatformEndpoint(c.Request.Context()); err != nil {
		apiShared.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	model.SuccessWithMessage(c, h.platformEndpointInfo(c.Request.Context()), "平台入口已重新同步")
}

// AdoptEndpointIngress explicitly transfers a matching manually created
// platform Ingress to the platform controller.
func (h *PlatformHandler) AdoptEndpointIngress(c *gin.Context) {
	if h.store == nil || h.client == nil || !h.client.KubernetesAvailable() {
		platformK8sUnavailable(c)
		return
	}
	endpoint, err := h.store.GetPlatformEndpoint()
	if errors.Is(err, gorm.ErrRecordNotFound) || !endpoint.Enabled {
		apiShared.ValidationError(c, "请先保存并启用平台 HTTPS 入口")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "读取平台入口失败")
		return
	}
	certificate, err := h.platformEndpointCertificate(c.Request.Context(), endpoint)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	endpoint.TLSSecretName = certificate.SecretName
	if err := h.store.SavePlatformEndpoint(endpoint); err != nil {
		apiShared.InternalError(c, "保存平台入口失败")
		return
	}
	ingressName, err := h.client.AdoptPlatformIngressContext(c.Request.Context(), endpoint.Hostname, endpoint.TLSSecretName)
	if err != nil {
		apiShared.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	endpoint.IngressName = ingressName
	if err := h.store.SavePlatformEndpoint(endpoint); err != nil {
		apiShared.InternalError(c, "保存平台入口失败")
		return
	}
	model.SuccessWithMessage(c, h.platformEndpointInfo(c.Request.Context()), "现有 Ingress 已接管并重新同步")
}

func (h *PlatformHandler) GenerateWebhookSecret(c *gin.Context) {
	if h.store == nil {
		apiShared.InternalError(c, "存储未初始化")
		return
	}
	secret, err := h.release.GenerateWebhookSecret()
	if err != nil {
		apiShared.InternalError(c, "保存部署密钥失败")
		return
	}
	model.Success(c, gin.H{"secret": secret})
}

func (h *PlatformHandler) UpdateImagePrefix(c *gin.Context) {
	var request struct {
		ImagePrefix string `json:"image_prefix"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "平台镜像仓库前缀无效")
		return
	}
	_, err := h.release.SetImagePrefixes(request.ImagePrefix)
	if err != nil {
		apiShared.BadRequest(c, "平台镜像仓库前缀无效")
		return
	}
	model.Success(c, nil)
}

func (h *PlatformHandler) Rollback(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "发布记录 ID 无效")
		return
	}
	previous, err := h.store.GetPlatformRelease(id)
	if err != nil || previous.PreviousImage == "" {
		apiShared.BadRequest(c, "该发布记录不能回滚")
		return
	}
	release, err := h.release.CreateRelease(c.Request.Context(), previous.PreviousImage, "manual_rollback", "", "")
	if err != nil {
		apiShared.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	model.SuccessWithMessage(c, release, "平台回滚已提交")
	c.Writer.Flush()
	h.release.Apply(context.WithoutCancel(c.Request.Context()), release.ID)
}

// Reconcile resumes a pending self-update after this service has restarted.
func (h *PlatformHandler) reconcilePlatformEndpoint(ctx context.Context) error {
	if h.store == nil || h.client == nil || !h.client.KubernetesAvailable() {
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
		return h.client.RemovePlatformEndpointContext(ctx, endpoint.IngressName)
	}
	certificate, err := h.platformEndpointCertificate(ctx, endpoint)
	if err != nil {
		return err
	}
	if endpoint.TLSSecretName != certificate.SecretName {
		endpoint.TLSSecretName = certificate.SecretName
		if err := h.store.SavePlatformEndpoint(endpoint); err != nil {
			return err
		}
	}
	return h.client.EnsurePlatformEndpointContext(ctx, endpoint.Hostname, endpoint.TLSSecretName, endpoint.IngressName)
}

func (h *PlatformHandler) platformEndpointInfo(ctx context.Context) platformEndpointInfo {
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
	if h.client == nil || !h.client.KubernetesAvailable() {
		info.State, info.CertificateError = "unavailable", "Kubernetes 客户端未初始化"
		return info
	}
	if ingress, ingressErr := h.client.PlatformIngressInfoContext(ctx, endpoint.IngressName); ingressErr != nil {
		info.CertificateError = ingressErr.Error()
	} else {
		info.Ingress = ingress
		info.IngressReady = ingress != nil && ingress.Managed
	}
	certificate, certificateErr := h.client.GetCertificateContext(ctx, "default", endpoint.CertificateName)
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

func (h *PlatformHandler) platformEndpointCertificate(ctx context.Context, endpoint *model.PlatformEndpoint) (*k8sclient.CertInfo, error) {
	if h.client == nil || !h.client.KubernetesAvailable() {
		return nil, errors.New("Kubernetes 客户端未初始化")
	}
	if strings.TrimSpace(endpoint.CertificateName) == "" {
		return nil, errors.New("请选择 default 命名空间中已就绪的 TLS 证书")
	}
	certificate, err := h.client.GetCertificateContext(ctx, "default", endpoint.CertificateName)
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

func (h *PlatformHandler) validatePlatformEndpointPrerequisites(ctx context.Context) error {
	if h.client == nil || !h.client.KubernetesAvailable() {
		return errors.New("Kubernetes 客户端未初始化")
	}
	status, err := h.client.DetectIngressControllerContext(ctx)
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
