package platform

import (
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

	"github.com/cylism/cylism-manager/internal/crypto"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
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
	store    *store.Store
	encKey   []byte
	client   *k8sclient.Client
	updateMu sync.Mutex
}

func NewReleaseService(st *store.Store, encKey []byte, client *k8sclient.Client) *ReleaseService {
	return &ReleaseService{store: st, encKey: append([]byte(nil), encKey...), client: client}
}

func (s *ReleaseService) Available() bool {
	return s != nil && s.store != nil && s.client != nil && s.client.Clientset != nil
}

func (s *ReleaseService) ValidateWebhook(timestampHeader, nonce, signature string, body []byte) bool {
	if s == nil || s.store == nil || len(strings.TrimSpace(timestampHeader)) == 0 || len(strings.TrimSpace(nonce)) < 16 || len(strings.TrimSpace(nonce)) > 256 || len(strings.TrimSpace(signature)) != 64 {
		return false
	}
	timestamp, err := strconv.ParseInt(timestampHeader, 10, 64)
	if err != nil || time.Since(time.Unix(timestamp, 0)).Abs() > webhookMaxSkew {
		return false
	}
	encrypted, err := s.store.GetSystemConfig(WebhookSecretConfigKey)
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
	if s == nil || s.store == nil {
		return errors.New("存储未初始化")
	}
	if err := s.store.DeleteExpiredPlatformWebhookNonces(now.UTC()); err != nil {
		return err
	}
	if err := s.store.CreatePlatformWebhookNonce(nonce, now.UTC().Add(24*time.Hour)); err != nil {
		return fmt.Errorf("%w: %v", ErrWebhookReplay, err)
	}
	return nil
}

func (s *ReleaseService) GenerateWebhookSecret() (string, error) {
	if s == nil || s.store == nil {
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
	if err := s.store.SetSystemConfig(WebhookSecretConfigKey, encrypted); err != nil {
		return "", err
	}
	return secret, nil
}

func (s *ReleaseService) ImagePrefixes() []string {
	if s == nil || s.store == nil {
		return []string{DefaultImagePrefix}
	}
	raw, err := s.store.GetSystemConfig(ImagePrefixConfigKey)
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
	if s == nil || s.store == nil {
		return nil, errors.New("存储未初始化")
	}
	prefixes, err := NormalizeImagePrefixes(raw)
	if err != nil {
		return nil, err
	}
	if err := s.store.SetSystemConfig(ImagePrefixConfigKey, strings.Join(prefixes, "\n")); err != nil {
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

func (s *ReleaseService) CreateRelease(image, source, commitSHA, runID string) (*model.PlatformRelease, error) {
	if !s.Available() {
		return nil, errors.New("Kubernetes 客户端未初始化")
	}
	status, err := s.client.PlatformDeploymentStatus()
	if err != nil {
		return nil, err
	}
	release := &model.PlatformRelease{Source: source, Image: image, PreviousImage: status.Image, Status: "accepted", CommitSHA: strings.TrimSpace(commitSHA), RunID: strings.TrimSpace(runID)}
	if err := s.store.CreatePlatformRelease(release); err != nil {
		return nil, fmt.Errorf("创建平台发布记录: %w", err)
	}
	return release, nil
}

func (s *ReleaseService) Submit(image, source, commitSHA, runID string) (*model.PlatformRelease, error) {
	if err := s.ValidateImage(image); err != nil {
		return nil, err
	}
	return s.CreateRelease(image, source, commitSHA, runID)
}

func (s *ReleaseService) Apply(id uint) {
	if !s.Available() {
		return
	}
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	release, err := s.store.GetPlatformRelease(id)
	if err != nil || (release.Status != "accepted" && release.Status != "applying") {
		return
	}
	now := time.Now().UTC()
	if release.StartedAt == nil {
		release.StartedAt = &now
	}
	release.Status = "applying"
	if err := s.store.UpdatePlatformRelease(release); err != nil {
		return
	}
	if _, err := s.client.UpdatePlatformDeployment(release.Image, release.ID); err != nil {
		release.Status, release.Detail, release.CompletedAt = "failed", err.Error(), &now
		_ = s.store.UpdatePlatformRelease(release)
		return
	}
	release.Status = "waiting_ready"
	_ = s.store.UpdatePlatformRelease(release)
}

func (s *ReleaseService) ReconcileLatest() {
	if !s.Available() {
		return
	}
	release, err := s.store.LatestIncompletePlatformRelease()
	if errors.Is(err, gorm.ErrRecordNotFound) || err != nil {
		return
	}
	status, err := s.client.PlatformDeploymentStatus()
	if err != nil {
		return
	}
	if (release.Status == "accepted" || release.Status == "applying") && (status.ReleaseID != release.ID || status.Image != release.Image) {
		s.Apply(release.ID)
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
	_ = s.store.UpdatePlatformRelease(release)
}
