package store

import (
	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func (s *Store) GetActiveClusterDNSPolicy() (*model.ClusterDNSPolicy, error) {
	var policy model.ClusterDNSPolicy
	err := s.db.Where("active = ?", true).Order("revision desc").First(&policy).Error
	return &policy, err
}

func (s *Store) ListClusterDNSPolicies(limit int) ([]model.ClusterDNSPolicy, error) {
	if limit < 1 || limit > 50 {
		limit = 20
	}
	var policies []model.ClusterDNSPolicy
	err := s.db.Order("revision desc").Limit(limit).Find(&policies).Error
	return policies, err
}

func (s *Store) CreateClusterDNSPolicy(policy *model.ClusterDNSPolicy) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var maxRevision uint
		if err := tx.Model(&model.ClusterDNSPolicy{}).Select("COALESCE(MAX(revision), 0)").Scan(&maxRevision).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.ClusterDNSPolicy{}).Where("active = ?", true).Update("active", false).Error; err != nil {
			return err
		}
		policy.Revision, policy.Active = maxRevision+1, true
		return tx.Create(policy).Error
	})
}
