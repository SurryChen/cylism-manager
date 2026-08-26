package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cylism/cylism-manager/internal/crypto"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	platformWebhookSecretConfigKey = "platform_deploy_webhook_secret"
	platformImagePrefixConfigKey   = "platform_image_prefix"
	platformDefaultImagePrefix     = "crpi-c5u9bb8i5qxw1m72.cn-guangzhou.personal.cr.aliyuncs.com/surrychen/cylism-manager"
	platformWebhookMaxSkew         = 5 * time.Minute
)

var platformImageTagPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

type PlatformHandler struct {
	store    *store.Store
	encKey   []byte
	updateMu sync.Mutex
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
	Hostname  string `json:"hostname"`
	IssuerRef string `json:"issuer_ref"`
	Enabled   bool   `json:"enabled"`
}

type platformEndpointInfo struct {
	Endpoint         model.PlatformEndpoint `json:"endpoint"`
	URL              string                 `json:"url,omitempty"`
	State            string                 `json:"state"`
	IngressReady     bool                   `json:"ingress_ready"`
	Certificate      *k8sclient.CertInfo    `json:"certificate,omitempty"`
	CertificateError string                 `json:"certificate_error,omitempty"`
}

func NewPlatformHandler(s *store.Store, encKey []byte) *PlatformHandler {
	return &PlatformHandler{store: s, encKey: encKey}
}

// Webhook accepts authenticated platform image deployment requests.
func (h *PlatformHandler) Webhook(c *gin.Context) {
	if h.store == nil || !h.validWebhook(c) {
		model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "部署签名无效或已过期")
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "读取部署请求失败")
		return
	}
	if !h.verifyWebhookSignature(c, body) {
		model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "部署签名无效或已过期")
		return
	}
	var request platformDeployRequest
	if err := json.Unmarshal(body, &request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "部署请求无效")
		return
	}
	if err := h.validatePlatformImage(request.Image); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	nonce := c.GetHeader("X-Cylism-Nonce")
	if err := h.store.DeleteExpiredPlatformWebhookNonces(time.Now().UTC()); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "清理部署请求失败")
		return
	}
	if err := h.store.CreatePlatformWebhookNonce(nonce, time.Now().UTC().Add(24*time.Hour)); err != nil {
		model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "部署签名无效或已过期")
		return
	}
	release, err := h.createPlatformRelease(request.Image, "github", request.CommitSHA, request.RunID)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, model.APIResponse{Code: model.CodeSuccess, Message: "平台发布已接受", Data: release})
	c.Writer.Flush()
	h.schedulePlatformRelease(release.ID)
}

// ManualUpdate lets an authenticated administrator submit a tagged platform image.
func (h *PlatformHandler) ManualUpdate(c *gin.Context) {
	var request platformManualReleaseRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "平台镜像请求无效")
		return
	}
	if err := h.validatePlatformImage(request.Image); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	release, err := h.createPlatformRelease(request.Image, "manual", "", "")
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, model.APIResponse{Code: model.CodeSuccess, Message: "平台更新已提交", Data: release})
	c.Writer.Flush()
	h.schedulePlatformRelease(release.ID)
}

func (h *PlatformHandler) Status(c *gin.Context) {
	if h.store == nil || K8s == nil {
		k8sUnavailable(c)
		return
	}
	h.reconcileLatestRelease()
	deployment, err := K8s.PlatformDeploymentStatus()
	if err != nil {
		model.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	releases, err := h.store.ListPlatformReleases(20)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "读取平台发布记录失败")
		return
	}
	prefix, _ := h.store.GetSystemConfig(platformImagePrefixConfigKey)
	if prefix == "" {
		prefix = platformDefaultImagePrefix
	}
	_, configured := h.store.GetSystemConfig(platformWebhookSecretConfigKey)
	model.Success(c, gin.H{"deployment": deployment, "releases": releases, "image_prefix": prefix, "webhook_configured": configured == nil})
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
		IssuerRef:       strings.TrimSpace(request.IssuerRef),
		IssuerKind:      "ClusterIssuer",
		CertificateName: k8sclient.PlatformEndpointCertificateName,
		TLSSecretName:   k8sclient.PlatformEndpointTLSSecretName,
		Enabled:         request.Enabled,
	}
	if !endpoint.Enabled {
		if endpoint.Hostname == "" {
			endpoint.Hostname = current.Hostname
		}
		if endpoint.IssuerRef == "" {
			endpoint.IssuerRef = current.IssuerRef
		}
	}
	if endpoint.Enabled {
		if !validPlatformHostname(endpoint.Hostname) {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "管理域名必须是合法的精确 DNS 名称，且不支持泛域名")
			return
		}
		if err := h.validatePlatformEndpointPrerequisites(endpoint); err != nil {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
			return
		}
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

func (h *PlatformHandler) GenerateWebhookSecret(c *gin.Context) {
	if h.store == nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "存储未初始化")
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "生成部署密钥失败")
		return
	}
	secret := hex.EncodeToString(raw)
	encrypted, err := crypto.Encrypt(h.encKey, secret)
	if err != nil || h.store.SetSystemConfig(platformWebhookSecretConfigKey, encrypted) != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "保存部署密钥失败")
		return
	}
	model.Success(c, gin.H{"secret": secret})
}

func (h *PlatformHandler) UpdateImagePrefix(c *gin.Context) {
	var request struct {
		ImagePrefix string `json:"image_prefix"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.ImagePrefix) == "" || strings.ContainsAny(request.ImagePrefix, "@ \t\n") {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "平台镜像仓库前缀无效")
		return
	}
	if err := h.store.SetSystemConfig(platformImagePrefixConfigKey, strings.TrimSuffix(strings.TrimSpace(request.ImagePrefix), "/")); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "保存平台镜像仓库前缀失败")
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
	release, err := h.createPlatformRelease(previous.PreviousImage, "manual_rollback", "", "")
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	model.SuccessWithMessage(c, release, "平台回滚已提交")
	c.Writer.Flush()
	h.schedulePlatformRelease(release.ID)
}

// Reconcile resumes a pending self-update after this service has restarted.
func (h *PlatformHandler) Reconcile() {
	_ = h.reconcilePlatformEndpoint()
	h.reconcileLatestRelease()
}

func (h *PlatformHandler) reconcilePlatformEndpoint() error {
	if h.store == nil || K8s == nil {
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
		return K8s.RemovePlatformEndpoint()
	}
	return K8s.EnsurePlatformEndpoint(endpoint.Hostname, endpoint.IssuerRef)
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
	info.State = "issuing"
	if K8s == nil {
		info.State, info.CertificateError = "unavailable", "Kubernetes 客户端未初始化"
		return info
	}
	if ready, ingressErr := K8s.PlatformIngressReady(); ingressErr != nil {
		info.CertificateError = ingressErr.Error()
	} else {
		info.IngressReady = ready
	}
	certificate, certificateErr := K8s.GetCertificate("default", endpoint.CertificateName)
	if certificateErr != nil {
		info.CertificateError = certificateErr.Error()
		return info
	}
	info.Certificate = certificate
	if certificate.Status == "Ready" && info.IngressReady {
		info.State = "ready"
	} else if certificate.Status == "Failed" {
		info.State = "failed"
	}
	return info
}

func (h *PlatformHandler) validatePlatformEndpointPrerequisites(endpoint *model.PlatformEndpoint) error {
	if K8s == nil || K8s.Clientset == nil {
		return errors.New("Kubernetes 客户端未初始化")
	}
	status, err := K8s.DetectIngressController()
	if err != nil || status == nil || !status.Running {
		return errors.New("Ingress Controller 未就绪")
	}
	certificateCRD, err := K8s.CheckCRD("certificates.cert-manager.io")
	if err != nil || !certificateCRD {
		return errors.New("cert-manager Certificate CRD 不可用")
	}
	issuers, err := K8s.ListIssuers()
	if err != nil {
		return fmt.Errorf("读取 ClusterIssuer: %w", err)
	}
	for _, issuer := range issuers {
		if issuer.Kind == "ClusterIssuer" && issuer.Name == endpoint.IssuerRef && issuer.Ready {
			return nil
		}
	}
	return fmt.Errorf("ClusterIssuer %q 不存在或尚未就绪", endpoint.IssuerRef)
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

func (h *PlatformHandler) validWebhook(c *gin.Context) bool {
	return len(strings.TrimSpace(c.GetHeader("X-Cylism-Timestamp"))) > 0 && len(strings.TrimSpace(c.GetHeader("X-Cylism-Nonce"))) >= 16 && len(strings.TrimSpace(c.GetHeader("X-Cylism-Nonce"))) <= 256 && len(strings.TrimSpace(c.GetHeader("X-Cylism-Signature"))) == 64
}

func (h *PlatformHandler) verifyWebhookSignature(c *gin.Context, body []byte) bool {
	timestamp, err := strconv.ParseInt(c.GetHeader("X-Cylism-Timestamp"), 10, 64)
	if err != nil || time.Since(time.Unix(timestamp, 0)).Abs() > platformWebhookMaxSkew {
		return false
	}
	encrypted, err := h.store.GetSystemConfig(platformWebhookSecretConfigKey)
	if err != nil {
		return false
	}
	secret, err := crypto.Decrypt(h.encKey, encrypted)
	if err != nil {
		return false
	}
	provided, err := hex.DecodeString(c.GetHeader("X-Cylism-Signature"))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(c.GetHeader("X-Cylism-Timestamp") + "." + c.GetHeader("X-Cylism-Nonce") + "."))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func (h *PlatformHandler) validatePlatformImage(image string) error {
	image = strings.TrimSpace(image)
	prefix, err := h.store.GetSystemConfig(platformImagePrefixConfigKey)
	if err != nil || prefix == "" {
		prefix = platformDefaultImagePrefix
	}
	prefix = strings.TrimSuffix(prefix, "/")
	if !strings.HasPrefix(image, prefix+":") {
		return fmt.Errorf("平台镜像不属于允许的仓库前缀")
	}
	tag := strings.TrimPrefix(image, prefix+":")
	if !platformImageTagPattern.MatchString(tag) {
		return fmt.Errorf("平台镜像 Tag 无效")
	}
	return nil
}

func (h *PlatformHandler) createPlatformRelease(image, source, commitSHA, runID string) (*model.PlatformRelease, error) {
	if K8s == nil {
		return nil, fmt.Errorf("Kubernetes 客户端未初始化")
	}
	status, err := K8s.PlatformDeploymentStatus()
	if err != nil {
		return nil, err
	}
	release := &model.PlatformRelease{Source: source, Image: image, PreviousImage: status.Image, Status: "accepted", CommitSHA: strings.TrimSpace(commitSHA), RunID: strings.TrimSpace(runID)}
	if err := h.store.CreatePlatformRelease(release); err != nil {
		return nil, fmt.Errorf("创建平台发布记录: %w", err)
	}
	return release, nil
}

func (h *PlatformHandler) schedulePlatformRelease(id uint) {
	// Flush the accepted response before applying the Deployment. The update can
	// terminate the Pod that is currently serving the request.
	h.applyPlatformRelease(id)
}

func (h *PlatformHandler) applyPlatformRelease(id uint) {
	h.updateMu.Lock()
	defer h.updateMu.Unlock()

	release, err := h.store.GetPlatformRelease(id)
	if err != nil || (release.Status != "accepted" && release.Status != "applying") {
		return
	}
	now := time.Now().UTC()
	if release.StartedAt == nil {
		release.StartedAt = &now
	}
	release.Status = "applying"
	if err := h.store.UpdatePlatformRelease(release); err != nil {
		return
	}
	if _, err := K8s.UpdatePlatformDeployment(release.Image, release.ID); err != nil {
		release.Status = "failed"
		release.Detail = err.Error()
		release.CompletedAt = &now
		_ = h.store.UpdatePlatformRelease(release)
		return
	}
	release.Status = "waiting_ready"
	_ = h.store.UpdatePlatformRelease(release)
}

func (h *PlatformHandler) reconcileLatestRelease() {
	release, err := h.store.LatestIncompletePlatformRelease()
	if errors.Is(err, gorm.ErrRecordNotFound) || err != nil || K8s == nil {
		return
	}
	status, err := K8s.PlatformDeploymentStatus()
	if err != nil {
		return
	}
	if (release.Status == "accepted" || release.Status == "applying") && (status.ReleaseID != release.ID || status.Image != release.Image) {
		h.applyPlatformRelease(release.ID)
		return
	}
	now := time.Now().UTC()
	if status.ReleaseID == release.ID && status.Image == release.Image && status.ReadyReplicas >= status.DesiredReplicas && status.DesiredReplicas > 0 {
		release.Status = "succeeded"
		release.Detail = "平台工作负载已就绪"
		release.CompletedAt = &now
	} else if status.Failure != "" || now.Sub(release.CreatedAt) > 15*time.Minute {
		release.Status = "failed"
		release.Detail = status.Failure
		if release.Detail == "" {
			release.Detail = "等待平台工作负载就绪超时"
		}
		release.CompletedAt = &now
	} else {
		return
	}
	_ = h.store.UpdatePlatformRelease(release)
}
