package platform

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	crypto "github.com/cylism/cylism-manager/internal/security"
	"gorm.io/gorm"
)

const (
	WebhookSecretConfigKey = "platform_deploy_webhook_secret"
	ImagePrefixConfigKey   = "platform_image_prefix"
	DefaultImagePrefix     = "crpi-c5u9bb8i5qxw1m72.cn-guangzhou.personal.cr.aliyuncs.com/surrychen/cylism-manager"
	webhookMaxSkew         = 5 * time.Minute
)

var imageTagPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

var ErrWebhookReplay = errors.New("platform webhook nonce already used")

// ReleaseService owns platform image admission, release persistence and the
// self-update lifecycle. It has no HTTP dependency and can be reused by the
// signed webhook and the management API.
type ReleaseService struct {
	releases repository.PlatformReleaseRepository
	encKey   []byte
	platform PlatformAdapter
	updateMu sync.Mutex
}

// PlatformAdapter is the minimal Kubernetes capability needed by the
// platform release state machine. Endpoint management remains owned by the
// delivery handler and is intentionally outside this interface.
type PlatformAdapter interface {
	PlatformDeploymentStatusContext(context.Context) (*k8sclient.PlatformDeploymentStatus, error)
	UpdatePlatformDeploymentContext(context.Context, string, uint) (string, error)
}

func NewReleaseService(releases repository.PlatformReleaseRepository, encKey []byte, platform PlatformAdapter) *ReleaseService {
	return &ReleaseService{releases: releases, encKey: append([]byte(nil), encKey...), platform: platform}
}

func (s *ReleaseService) Available() bool {
	return s != nil && s.releases != nil && s.platform != nil
}

func (s *ReleaseService) ValidateWebhook(timestampHeader, nonce, signature string, body []byte) bool {
	if s == nil || s.releases == nil || len(strings.TrimSpace(timestampHeader)) == 0 || len(strings.TrimSpace(nonce)) < 16 || len(strings.TrimSpace(nonce)) > 256 || len(strings.TrimSpace(signature)) != 64 {
		return false
	}
	timestamp, err := strconv.ParseInt(timestampHeader, 10, 64)
	if err != nil || time.Since(time.Unix(timestamp, 0)).Abs() > webhookMaxSkew {
		return false
	}
	encrypted, err := s.releases.GetSystemConfig(WebhookSecretConfigKey)
	if err != nil {
		return false
	}
	secret, err := crypto.Decrypt(s.encKey, encrypted)
	if err != nil {
		return false
	}
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestampHeader + "." + nonce + "."))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func (s *ReleaseService) ConsumeWebhookNonce(nonce string, now time.Time) error {
	if s == nil || s.releases == nil {
		return errors.New("存储未初始化")
	}
	if err := s.releases.DeleteExpiredPlatformWebhookNonces(now.UTC()); err != nil {
		return err
	}
	if err := s.releases.CreatePlatformWebhookNonce(nonce, now.UTC().Add(24*time.Hour)); err != nil {
		return fmt.Errorf("%w: %v", ErrWebhookReplay, err)
	}
	return nil
}

func (s *ReleaseService) GenerateWebhookSecret() (string, error) {
	if s == nil || s.releases == nil {
		return "", errors.New("存储未初始化")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(raw)
	encrypted, err := crypto.Encrypt(s.encKey, secret)
	if err != nil {
		return "", err
	}
	if err := s.releases.SetSystemConfig(WebhookSecretConfigKey, encrypted); err != nil {
		return "", err
	}
	return secret, nil
}

func (s *ReleaseService) ImagePrefixes() []string {
	if s == nil || s.releases == nil {
		return []string{DefaultImagePrefix}
	}
	raw, err := s.releases.GetSystemConfig(ImagePrefixConfigKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return []string{DefaultImagePrefix}
	}
	prefixes, err := NormalizeImagePrefixes(raw)
	if err != nil || len(prefixes) == 0 {
		return []string{DefaultImagePrefix}
	}
	return prefixes
}

func (s *ReleaseService) SetImagePrefixes(raw string) ([]string, error) {
	if s == nil || s.releases == nil {
		return nil, errors.New("存储未初始化")
	}
	prefixes, err := NormalizeImagePrefixes(raw)
	if err != nil {
		return nil, err
	}
	if err := s.releases.SetSystemConfig(ImagePrefixConfigKey, strings.Join(prefixes, "\n")); err != nil {
		return nil, err
	}
	return prefixes, nil
}

func (s *ReleaseService) ValidateImage(image string) error {
	image = strings.TrimSpace(image)
	matched := ""
	for _, prefix := range s.ImagePrefixes() {
		if strings.HasPrefix(image, prefix+":") {
			matched = prefix
			break
		}
	}
	if matched == "" {
		return errors.New("平台镜像不属于允许的仓库前缀")
	}
	if !imageTagPattern.MatchString(strings.TrimPrefix(image, matched+":")) {
		return errors.New("平台镜像 Tag 无效")
	}
	return nil
}

func NormalizeImagePrefixes(raw string) ([]string, error) {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' })
	seen := make(map[string]struct{}, len(parts))
	prefixes := make([]string, 0, len(parts))
	for _, part := range parts {
		prefix := strings.TrimSuffix(strings.TrimSpace(part), "/")
		if prefix == "" || strings.ContainsAny(prefix, "@ \t\r\n") {
			return nil, errors.New("invalid image prefix")
		}
		if _, ok := seen[prefix]; ok {
			continue
		}
		seen[prefix] = struct{}{}
		prefixes = append(prefixes, prefix)
	}
	if len(prefixes) == 0 {
		return nil, errors.New("image prefix is required")
	}
	return prefixes, nil
}

func (s *ReleaseService) CreateRelease(ctx context.Context, image, source, commitSHA, runID string) (*model.PlatformRelease, error) {
	if !s.Available() {
		return nil, errors.New("Kubernetes 客户端未初始化")
	}
	status, err := s.platform.PlatformDeploymentStatusContext(ctx)
	if err != nil {
		return nil, err
	}
	release := &model.PlatformRelease{Source: source, Image: image, PreviousImage: status.Image, Status: "accepted", CommitSHA: strings.TrimSpace(commitSHA), RunID: strings.TrimSpace(runID)}
	if err := s.releases.CreatePlatformRelease(release); err != nil {
		return nil, fmt.Errorf("创建平台发布记录: %w", err)
	}
	return release, nil
}

func (s *ReleaseService) Submit(ctx context.Context, image, source, commitSHA, runID string) (*model.PlatformRelease, error) {
	if err := s.ValidateImage(image); err != nil {
		return nil, err
	}
	return s.CreateRelease(ctx, image, source, commitSHA, runID)
}

func (s *ReleaseService) Apply(ctx context.Context, id uint) {
	if !s.Available() {
		return
	}
	if ctx == nil {
		return
	}
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	release, err := s.releases.GetPlatformRelease(id)
	if err != nil || (release.Status != "accepted" && release.Status != "applying") {
		return
	}
	now := time.Now().UTC()
	if release.StartedAt == nil {
		release.StartedAt = &now
	}
	release.Status = "applying"
	if err := s.releases.UpdatePlatformRelease(release); err != nil {
		return
	}
	if _, err := s.platform.UpdatePlatformDeploymentContext(ctx, release.Image, release.ID); err != nil {
		release.Status, release.Detail, release.CompletedAt = "failed", err.Error(), &now
		_ = s.releases.UpdatePlatformRelease(release)
		return
	}
	release.Status = "waiting_ready"
	_ = s.releases.UpdatePlatformRelease(release)
}

func (s *ReleaseService) ReconcileLatest(ctx context.Context) {
	if !s.Available() {
		return
	}
	if ctx == nil {
		return
	}
	release, err := s.releases.LatestIncompletePlatformRelease()
	if errors.Is(err, gorm.ErrRecordNotFound) || err != nil {
		return
	}
	status, err := s.platform.PlatformDeploymentStatusContext(ctx)
	if err != nil {
		return
	}
	if (release.Status == "accepted" || release.Status == "applying") && (status.ReleaseID != release.ID || status.Image != release.Image) {
		s.Apply(ctx, release.ID)
		return
	}
	now := time.Now().UTC()
	if status.ReleaseID == release.ID && status.Image == release.Image && status.ReadyReplicas >= status.DesiredReplicas && status.DesiredReplicas > 0 {
		release.Status, release.Detail, release.CompletedAt = "succeeded", "平台工作负载已就绪", &now
	} else if status.Failure != "" || now.Sub(release.CreatedAt) > 15*time.Minute {
		release.Status, release.Detail, release.CompletedAt = "failed", status.Failure, &now
		if release.Detail == "" {
			release.Detail = "等待平台工作负载就绪超时"
		}
	} else {
		return
	}
	_ = s.releases.UpdatePlatformRelease(release)
}
