package store

import (
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

func (s *Store) CreateTemporaryLoginToken(token *model.TemporaryLoginToken) error {
	return s.db.Create(token).Error
}
func (s *Store) ListTemporaryLoginTokens(userID uint) ([]model.TemporaryLoginToken, error) {
	var tokens []model.TemporaryLoginToken
	err := s.db.Where("created_by = ?", userID).Order("id desc").Find(&tokens).Error
	return tokens, err
}
func (s *Store) FindActiveTemporaryLoginToken(hash string, now time.Time) (*model.TemporaryLoginToken, error) {
	var token model.TemporaryLoginToken
	err := s.db.Where("token_hash = ? AND revoked_at IS NULL AND expires_at > ?", hash, now).First(&token).Error
	return &token, err
}
func (s *Store) MarkTemporaryLoginTokenUsed(hash string, usedAt time.Time) error {
	return s.db.Model(&model.TemporaryLoginToken{}).Where("token_hash = ?", hash).Update("last_used_at", usedAt).Error
}
func (s *Store) RevokeTemporaryLoginToken(userID, tokenID uint, revokedAt time.Time) error {
	return s.db.Model(&model.TemporaryLoginToken{}).Where("id = ? AND created_by = ? AND revoked_at IS NULL", tokenID, userID).Update("revoked_at", revokedAt).Error
}
