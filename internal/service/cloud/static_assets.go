package cloud

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	security "github.com/cylism/cylism-manager/internal/security"
	"gorm.io/gorm"
)

const (
	staticAssetConfigKey = "cloud.static_assets"
	staticAssetTicketTTL = 5 * time.Minute
	MaxStaticImageSize   = 2 << 20
	MaxStaticImageSide   = 1024
)

type StaticAssetSettings struct {
	ConnectionID uint   `json:"connection_id"`
	Bucket       string `json:"bucket"`
	Region       string `json:"region"`
}

type StaticAssetConfigStore interface {
	GetSystemConfig(string) (string, error)
	SetSystemConfig(string, string) error
}

type StaticAssetProvider interface {
	SignStaticAssetUpload(bucket, key, contentType string, size int64, ttl time.Duration) (string, error)
	PublishStaticImage(ctx context.Context, bucket, source, destination, contentType string, size int64) (string, error)
}

type StaticAssetService struct {
	cloud  *Service
	config StaticAssetConfigStore
	key    []byte
}

func NewStaticAssetService(cloud *Service, config StaticAssetConfigStore, key []byte) *StaticAssetService {
	return &StaticAssetService{cloud: cloud, config: config, key: key}
}

func (s *StaticAssetService) Settings() (StaticAssetSettings, error) {
	var settings StaticAssetSettings
	raw, err := s.config.GetSystemConfig(staticAssetConfigKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	return settings, json.Unmarshal([]byte(raw), &settings)
}

func (s *StaticAssetService) SaveSettings(ctx context.Context, input StaticAssetSettings) (StaticAssetSettings, error) {
	input.Bucket = strings.TrimSpace(input.Bucket)
	if input.ConnectionID == 0 || input.Bucket == "" {
		return StaticAssetSettings{}, errors.New("请选择静态资源云连接和存储桶")
	}
	containers, err := s.cloud.ListContainers(ctx, input.ConnectionID)
	if err != nil {
		return StaticAssetSettings{}, errors.New("读取存储桶失败，请检查云连接及权限")
	}
	for _, bucket := range containers {
		if bucket.Name == input.Bucket {
			input.Region = bucket.Region
			if _, err := s.provider(input); err != nil {
				return StaticAssetSettings{}, err
			}
			raw, err := json.Marshal(input)
			if err != nil {
				return StaticAssetSettings{}, err
			}
			return input, s.config.SetSystemConfig(staticAssetConfigKey, string(raw))
		}
	}
	return StaticAssetSettings{}, errors.New("存储桶不属于所选云连接")
}

type StaticAssetUpload struct {
	URL     string            `json:"upload_url"`
	Headers map[string]string `json:"headers"`
	Ticket  string            `json:"ticket"`
}

type staticAssetTicket struct {
	Purpose     string
	OwnerID     uint
	Settings    StaticAssetSettings
	ID          string
	ContentType string
	Size        int64
	Expires     time.Time
}

func (t staticAssetTicket) keys() (string, string) {
	ext := ".png"
	if t.ContentType == "image/jpeg" {
		ext = ".jpg"
	}
	return "pending/" + t.ID + ext, "images/" + t.ID + ext
}

var staticAssetID = regexp.MustCompile(`^[a-f0-9]{32}$`)

func (s *StaticAssetService) Issue(ownerID uint, contentType string, size int64) (StaticAssetUpload, error) {
	if size <= 0 || size > MaxStaticImageSize || (contentType != "image/png" && contentType != "image/jpeg") {
		return StaticAssetUpload{}, errors.New("请选择 2 MiB 以内的 PNG 或 JPEG 图片")
	}
	settings, err := s.Settings()
	if err != nil {
		return StaticAssetUpload{}, errors.New("读取静态资源配置失败")
	}
	provider, err := s.provider(settings)
	if err != nil {
		return StaticAssetUpload{}, err
	}
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return StaticAssetUpload{}, errors.New("生成上传凭证失败")
	}
	ticket := staticAssetTicket{
		Purpose: staticAssetConfigKey, OwnerID: ownerID, Settings: settings,
		ID: hex.EncodeToString(entropy[:]), ContentType: contentType, Size: size,
		Expires: time.Now().Add(2 * staticAssetTicketTTL),
	}
	pending, _ := ticket.keys()
	uploadURL, err := provider.SignStaticAssetUpload(settings.Bucket, pending, contentType, size, staticAssetTicketTTL)
	if err != nil {
		return StaticAssetUpload{}, errors.New("生成 OSS 上传地址失败")
	}
	raw, err := json.Marshal(ticket)
	if err != nil {
		return StaticAssetUpload{}, err
	}
	ciphertext, err := security.Encrypt(s.key, string(raw))
	if err != nil {
		return StaticAssetUpload{}, errors.New("生成上传凭证失败")
	}
	return StaticAssetUpload{URL: uploadURL, Ticket: ciphertext, Headers: map[string]string{
		"Content-Type": contentType, "x-oss-object-acl": "private", "x-oss-forbid-overwrite": "true",
	}}, nil
}

func (s *StaticAssetService) Publish(ctx context.Context, ownerID uint, ciphertext string) (string, error) {
	raw, err := security.Decrypt(s.key, ciphertext)
	var ticket staticAssetTicket
	if err != nil || json.Unmarshal([]byte(raw), &ticket) != nil || ticket.Purpose != staticAssetConfigKey ||
		ticket.OwnerID != ownerID || !time.Now().Before(ticket.Expires) || !staticAssetID.MatchString(ticket.ID) {
		return "", errors.New("上传凭证无效或已过期，请重新上传")
	}
	settings, err := s.Settings()
	if err != nil || ticket.Settings != settings {
		return "", errors.New("静态资源配置已变更，请重新上传")
	}
	provider, err := s.provider(settings)
	if err != nil {
		return "", err
	}
	pending, published := ticket.keys()
	return provider.PublishStaticImage(ctx, settings.Bucket, pending, published, ticket.ContentType, ticket.Size)
}

func (s *StaticAssetService) provider(settings StaticAssetSettings) (StaticAssetProvider, error) {
	if settings.ConnectionID == 0 || settings.Bucket == "" {
		return nil, errors.New("请先在对象存储中配置静态资源桶")
	}
	_, storage, err := s.cloud.objectStorageProvider(settings.ConnectionID)
	if err != nil {
		return nil, errors.New("静态资源云连接不可用")
	}
	provider, ok := storage.(StaticAssetProvider)
	if !ok {
		return nil, errors.New("当前云连接不支持静态资源直传")
	}
	if err := setStorageRegion(storage, settings.Region); err != nil {
		return nil, fmt.Errorf("静态资源存储地域无效")
	}
	return provider, nil
}
