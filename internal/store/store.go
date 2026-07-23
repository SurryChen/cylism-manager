package store

import (
	"errors"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Store 数据存储层
type Store struct {
	db *gorm.DB
}

// New 创建新的 Store 实例，自动迁移所有模型
func New(dsn string) (*Store, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(
		&model.Server{},
		&model.Site{},
		&model.Cert{},
		&model.AuditLog{},model.AuditLog{},
		&model.User{},
	); err != nil {
		return nil, err
	}

	return &Store{db: db}, nil
}

// DB 返回底层 GORM DB 实例（供中间件等使用）
func (s *Store) DB() *gorm.DB {
	return s.db
}

// --- Server ---

func (s *Store) CreateServer(server *model.Server) error {
	return s.db.Create(server).Error
}

func (s *Store) GetServer(id uint) (*model.Server, error) {
	var server model.Server
	err := s.db.First(&server, id).Error
	if err != nil {
		return nil, err
	}
	return &server, nil
}

func (s *Store) GetServerByHost(host string) (*model.Server, error) {
	var server model.Server
	err := s.db.Where("host = ?", host).First(&server).Error
	return &server, err
}

func (s *Store) ListServers() ([]model.Server, error) {
	var servers []model.Server
	err := s.db.Order("created_at desc").Find(&servers).Error
	return servers, err
}

func (s *Store) UpdateServer(server *model.Server) error {
	return s.db.Save(server).Error
}

func (s *Store) DeleteServer(id uint) error {
	return s.db.Delete(&model.Server{}, id).Error
}

// --- Site ---

func (s *Store) CreateSite(site *model.Site) error {
	// 检查同一服务器内域名唯一性
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

func (s *Store) UpdateSite(site *model.Site) error {
	return s.db.Save(site).Error
}

func (s *Store) DeleteSite(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 先删除关联证书
		if err := tx.Where("site_id = ?", id).Delete(&model.Cert{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Site{}, id).Error
	})
}

// --- Cert ---

func (s *Store) CreateCert(cert *model.Cert) error {
	return s.db.Create(cert).Error
}

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

func (s *Store) UpdateCert(cert *model.Cert) error {
	return s.db.Save(cert).Error
}

func (s *Store) DeleteCert(id uint) error {
	return s.db.Delete(&model.Cert{}, id).Error
}

// --- AuditLog ---

func (s *Store) CreateAuditLog(entry *model.AuditLog) error {
	return s.db.Create(entry).Error
}

func (s *Store) ListAuditLogs(resourceType, action string, limit, offset int) ([]model.AuditLog, int64, error) {
	var logs []model.AuditLog
	var total int64

	query := s.db.Model(&model.AuditLog{})
	if resourceType != "" {
		query = query.Where("resource_type = ?", resourceType)
	}
	if action != "" {
		query = query.Where("action = ?", action)
	}
	query.Count(&total)

	err := query.Order("created_at desc").Limit(limit).Offset(offset).Find(&logs).Error
	return logs, total, err
}

// --- Dashboard Stats ---

// DashboardStats 仪表盘统计数据
type DashboardStats struct {
	TotalServers    int64 `json:"total_servers"`
	OnlineServers   int64 `json:"online_servers"`
	TotalSites      int64 `json:"total_sites"`
	ExpiringCerts   int64 `json:"expiring_certs"`
}

func (s *Store) GetDashboardStats(daysBefore int) (*DashboardStats, error) {
	stats := &DashboardStats{}

	s.db.Model(&model.Server{}).Count(&stats.TotalServers)
	s.db.Model(&model.Server{}).Where("status = ?", "online").Count(&stats.OnlineServers)
	s.db.Model(&model.Site{}).Count(&stats.TotalSites)

	threshold := time.Now().Add(time.Duration(daysBefore) * 24 * time.Hour)
	s.db.Model(&model.Cert{}).Where("status = ? AND valid_to <= ?", "issued", threshold).Count(&stats.ExpiringCerts)

	return stats, nil
}



// --- User ---

func (s *Store) CreateUser(user *model.User) error {
	return s.db.Create(user).Error
}

func (s *Store) GetUserByUsername(username string) (*model.User, error) {
	var user model.User
	err := s.db.Where("username = ?", username).First(&user).Error
	return &user, err
}

func (s *Store) CountUsers() (int64, error) {
	var count int64
	err := s.db.Model(&model.User{}).Count(&count).Error
	return count, err
}
