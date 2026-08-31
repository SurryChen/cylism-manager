package store

import (
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func (s *Store) CreateIntegrationSession(session *model.IntegrationSession) error {
	return s.db.Create(session).Error
}

func (s *Store) ExchangeIntegrationSession(handoffHash, sessionHash string, sessionExpiresAt, now time.Time) (*model.IntegrationSession, error) {
	var result model.IntegrationSession
	err := s.db.Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&model.IntegrationSession{}).Where("handoff_code_hash = ? AND handoff_used_at IS NULL AND revoked_at IS NULL AND handoff_expires_at > ?", handoffHash, now).Updates(map[string]interface{}{"handoff_used_at": now, "session_token_hash": sessionHash, "expires_at": sessionExpiresAt})
		if query.Error != nil {
			return query.Error
		}
		if query.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return tx.Where("handoff_code_hash = ?", handoffHash).First(&result).Error
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Store) GetActiveIntegrationSession(tokenHash string, now time.Time) (*model.IntegrationSession, error) {
	var session model.IntegrationSession
	err := s.db.Where("session_token_hash = ? AND revoked_at IS NULL AND expires_at > ?", tokenHash, now).First(&session).Error
	return &session, err
}
