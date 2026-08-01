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

var digestImagePattern = regexp.MustCompile(`^[^\s@]+@sha256:[a-f0-9]{64}$`)

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

func NewPlatformHandler(s *store.Store, encKey []byte) *PlatformHandler {
	return &PlatformHandler{store: s, encKey: encKey}
}

// Webhook accepts only authenticated immutable image deployment requests.
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

// ManualUpdate lets an authenticated administrator submit an immutable platform image.
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
	h.reconcileLatestRelease()
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
	if !digestImagePattern.MatchString(image) {
		return fmt.Errorf("平台镜像必须使用 sha256 digest")
	}
	prefix, err := h.store.GetSystemConfig(platformImagePrefixConfigKey)
	if err != nil || prefix == "" {
		prefix = platformDefaultImagePrefix
	}
	if !strings.HasPrefix(image, strings.TrimSuffix(prefix, "/")+"@") {
		return fmt.Errorf("平台镜像不属于允许的仓库前缀")
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
