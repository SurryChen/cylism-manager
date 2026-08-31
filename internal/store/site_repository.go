package store

import (
	"errors"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func (s *Store) CreateSite(site *model.Site) error {
	var count int64
	s.db.Model(&model.Site{}).Where("server_id = ? AND domain = ?", site.ServerID, site.Domain).Count(&count)
	if count > 0 {
		return errors.New("domain already exists on this server")
	}
	return s.db.Create(site).Error
}
func (s *Store) GetSite(id uint) (*model.Site, error) {
	var site model.Site
	err := s.db.Preload("Cert").First(&site, id).Error
	return &site, err
}
func (s *Store) ListSites() ([]model.Site, error) {
	var sites []model.Site
	err := s.db.Preload("Cert").Order("created_at desc").Find(&sites).Error
	return sites, err
}
func (s *Store) ListSitesByServer(serverID uint) ([]model.Site, error) {
	var sites []model.Site
	err := s.db.Where("server_id = ?", serverID).Preload("Cert").Order("created_at desc").Find(&sites).Error
	return sites, err
}
func (s *Store) UpdateSite(site *model.Site) error { return s.db.Save(site).Error }
func (s *Store) DeleteSite(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("site_id = ?", id).Delete(&model.Cert{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Site{}, id).Error
	})
}
func (s *Store) CreateCert(cert *model.Cert) error { return s.db.Create(cert).Error }
func (s *Store) GetCert(id uint) (*model.Cert, error) {
	var cert model.Cert
	err := s.db.First(&cert, id).Error
	return &cert, err
}
func (s *Store) GetCertBySite(siteID uint) (*model.Cert, error) {
	var cert model.Cert
	err := s.db.Where("site_id = ?", siteID).First(&cert).Error
	return &cert, err
}
func (s *Store) ListExpiringCerts(daysBefore int) ([]model.Cert, error) {
	var certs []model.Cert
	threshold := time.Now().Add(time.Duration(daysBefore) * 24 * time.Hour)
	err := s.db.Where("status = ? AND valid_to <= ?", "issued", threshold).Find(&certs).Error
	return certs, err
}
func (s *Store) UpdateCert(cert *model.Cert) error { return s.db.Save(cert).Error }
func (s *Store) DeleteCert(id uint) error          { return s.db.Delete(&model.Cert{}, id).Error }
