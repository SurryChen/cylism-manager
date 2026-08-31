package store

import (
	"errors"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func (s *Store) GetSystemConfig(key string) (string, error) {
	var cfg model.SystemConfig
	err := s.db.Where("key = ?", key).First(&cfg).Error
	if err != nil {
		return "", err
	}
	return cfg.Value, nil
}

func (s *Store) SetSystemConfig(key, value string) error {
	var cfg model.SystemConfig
	err := s.db.Where("key = ?", key).First(&cfg).Error
	if err != nil {
		return s.db.Create(&model.SystemConfig{Key: key, Value: value}).Error
	}
	return s.db.Model(&cfg).Update("value", value).Error
}

func (s *Store) CreatePlatformRelease(release *model.PlatformRelease) error {
	return s.db.Create(release).Error
}

func (s *Store) GetPlatformRelease(id uint) (*model.PlatformRelease, error) {
	var release model.PlatformRelease
	if err := s.db.First(&release, id).Error; err != nil {
		return nil, err
	}
	return &release, nil
}

func (s *Store) ListPlatformReleases(limit int) ([]model.PlatformRelease, error) {
	var releases []model.PlatformRelease
	query := s.db.Order("id desc")
	if limit > 0 {
		query = query.Limit(limit)
	}
	return releases, query.Find(&releases).Error
}

func (s *Store) LatestIncompletePlatformRelease() (*model.PlatformRelease, error) {
	var release model.PlatformRelease
	err := s.db.Where("status IN ?", []string{"accepted", "applying", "waiting_ready"}).Order("id desc").First(&release).Error
	if err != nil {
		return nil, err
	}
	return &release, nil
}

func (s *Store) UpdatePlatformRelease(release *model.PlatformRelease) error {
	return s.db.Save(release).Error
}

func (s *Store) GetPlatformEndpoint() (*model.PlatformEndpoint, error) {
	var endpoint model.PlatformEndpoint
	err := s.db.First(&endpoint, 1).Error
	return &endpoint, err
}

func (s *Store) SavePlatformEndpoint(endpoint *model.PlatformEndpoint) error {
	if endpoint == nil {
		return errors.New("平台入口不能为空")
	}
	endpoint.ID = 1
	var existing model.PlatformEndpoint
	err := s.db.First(&existing, endpoint.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.db.Create(endpoint).Error
	}
	if err != nil {
		return err
	}
	endpoint.CreatedAt = existing.CreatedAt
	return s.db.Save(endpoint).Error
}

func (s *Store) CreatePlatformWebhookNonce(nonce string, expiresAt time.Time) error {
	return s.db.Create(&model.PlatformWebhookNonce{Nonce: nonce, ExpiresAt: expiresAt}).Error
}

func (s *Store) DeleteExpiredPlatformWebhookNonces(now time.Time) error {
	return s.db.Where("expires_at <= ?", now).Delete(&model.PlatformWebhookNonce{}).Error
}
