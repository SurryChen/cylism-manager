package store

import (
	"errors"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrAccessCAExists = errors.New("access CA already exists")
var ErrAccessCertificateInUse = errors.New("access certificate is in use")

func (s *Store) GetAccessCA() (*model.AccessCA, error) {
	var ca model.AccessCA
	if err := s.db.First(&ca, 1).Error; err != nil {
		return nil, err
	}
	return &ca, nil
}

func (s *Store) CreateAccessCA(ca *model.AccessCA) error {
	ca.ID = 1
	result := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(ca)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrAccessCAExists
	}
	return nil
}

func (s *Store) ListAccessCertificates() ([]model.AccessCertificate, error) {
	var items []model.AccessCertificate
	err := s.db.Order("created_at desc, id desc").Find(&items).Error
	return items, err
}

func (s *Store) GetAccessCertificate(id uint) (*model.AccessCertificate, error) {
	var item model.AccessCertificate
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Store) CreateAccessCertificate(item *model.AccessCertificate) error {
	return s.db.Create(item).Error
}

func (s *Store) MarkAccessCertificateInUse(id uint, inUse bool) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var item model.AccessCertificate
		if err := tx.First(&item, id).Error; err != nil {
			return err
		}
		if inUse {
			if err := tx.Model(&model.AccessCertificate{}).Where("cluster_id = ? AND kind = ? AND id <> ?", item.ClusterID, item.Kind, id).Update("in_use", false).Error; err != nil {
				return err
			}
		}
		return tx.Model(&model.AccessCertificate{}).Where("id = ?", id).Update("in_use", inUse).Error
	})
}

func (s *Store) MarkClusterAccessCertificatesUnused(clusterID uint, kind string) error {
	return s.db.Model(&model.AccessCertificate{}).Where("cluster_id = ? AND kind = ?", clusterID, kind).Update("in_use", false).Error
}

func (s *Store) DeleteAccessCertificate(id uint) error {
	result := s.db.Where("id = ? AND in_use = ?", id, false).Delete(&model.AccessCertificate{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrAccessCertificateInUse
	}
	return nil
}

func (s *Store) RevokeAccessCertificate(id uint, at time.Time) error {
	return s.db.Model(&model.AccessCertificate{}).Where("id = ? AND revoked_at IS NULL", id).Update("revoked_at", at).Error
}
