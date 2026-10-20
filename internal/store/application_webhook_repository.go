package store

import (
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Store) GetApplicationDeployWebhook(applicationID uint) (*model.ApplicationDeployWebhook, error) {
	var webhook model.ApplicationDeployWebhook
	err := s.db.First(&webhook, "application_id = ?", applicationID).Error
	return &webhook, err
}

func (s *Store) SaveApplicationDeployWebhook(webhook *model.ApplicationDeployWebhook) error {
	return s.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "application_id"}}, DoUpdates: clause.AssignmentColumns([]string{"encrypted_secret", "updated_at"})}).Create(webhook).Error
}

func (s *Store) DeleteApplicationDeployWebhook(applicationID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&model.ApplicationDeployWebhook{}, "application_id = ?", applicationID).Error; err != nil {
			return err
		}
		return tx.Delete(&model.ApplicationDeployWebhookNonce{}, "application_id = ?", applicationID).Error
	})
}

func (s *Store) ConsumeApplicationDeployWebhookNonce(applicationID uint, nonce string, now time.Time) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("expires_at <= ?", now.UTC()).Delete(&model.ApplicationDeployWebhookNonce{}).Error; err != nil {
			return err
		}
		return tx.Create(&model.ApplicationDeployWebhookNonce{ApplicationID: applicationID, Nonce: nonce, ExpiresAt: now.UTC().Add(24 * time.Hour)}).Error
	})
}
