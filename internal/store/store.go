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
		&model.AuditLog{},
		&model.OperationLog{},
		&model.User{},
		&model.SystemConfig{},
		&model.Project{},
		&model.Environment{},
		&model.Application{},
		&model.ApplicationEndpoint{},
		&model.ImageRegistry{},
		&model.ManagedDomain{},
		&model.Release{},
		&model.ReleaseOperation{},
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
	TotalServers  int64 `json:"total_servers"`
	TotalSites    int64 `json:"total_sites"`
	ExpiringCerts int64 `json:"expiring_certs"`
}

func (s *Store) GetDashboardStats(daysBefore int) (*DashboardStats, error) {
	stats := &DashboardStats{}

	s.db.Model(&model.Server{}).Count(&stats.TotalServers)
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

// --- OperationLog ---

// CreateOperationLog 创建操作日志
func (s *Store) CreateOperationLog(log *model.OperationLog) error {
	return s.db.Create(log).Error
}

// UpdateOperationLog 更新操作日志状态
func (s *Store) UpdateOperationLog(log *model.OperationLog) error {
	return s.db.Save(log).Error
}

// ListOperationsByResource 按资源类型和 ID 查询操作日志
func (s *Store) ListOperationsByResource(resourceType string, resourceID uint) ([]model.OperationLog, error) {
	var logs []model.OperationLog
	err := s.db.Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).
		Order("created_at desc").
		Find(&logs).Error
	return logs, err
}

// DeleteExpiredOperationLogs 删除过期操作日志
func (s *Store) DeleteExpiredOperationLogs(retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}
	threshold := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	return s.db.Where("created_at < ?", threshold).Delete(&model.OperationLog{}).Error
}

// --- SystemConfig ---

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

// --- Application release center ---

func (s *Store) CreateProject(project *model.Project) error {
	return s.db.Create(project).Error
}

func (s *Store) ListProjects() ([]model.Project, error) {
	var projects []model.Project
	err := s.db.Preload("Environments").Preload("DefaultImageRegistry").Order("created_at desc").Find(&projects).Error
	return projects, err
}

func (s *Store) GetProject(id uint) (*model.Project, error) {
	var project model.Project
	err := s.db.Preload("Environments").Preload("DefaultImageRegistry").First(&project, id).Error
	return &project, err
}

func (s *Store) UpdateProject(project *model.Project) error {
	return s.db.Save(project).Error
}

func (s *Store) DeleteProject(id uint) error {
	return s.db.Delete(&model.Project{}, id).Error
}

func (s *Store) CountProjectEnvironments(projectID uint) (int64, error) {
	var count int64
	err := s.db.Model(&model.Environment{}).Where("project_id = ?", projectID).Count(&count).Error
	return count, err
}

func (s *Store) CountProjectApplications(projectID uint) (int64, error) {
	var count int64
	err := s.db.Model(&model.Application{}).Where("project_id = ?", projectID).Count(&count).Error
	return count, err
}

func (s *Store) CreateEnvironment(environment *model.Environment) error {
	return s.db.Create(environment).Error
}

func (s *Store) ListEnvironments(projectID uint) ([]model.Environment, error) {
	var environments []model.Environment
	err := s.db.Where("project_id = ?", projectID).Order("created_at asc").Find(&environments).Error
	return environments, err
}

func (s *Store) GetEnvironment(projectID, environmentID uint) (*model.Environment, error) {
	var environment model.Environment
	err := s.db.Where("project_id = ?", projectID).First(&environment, environmentID).Error
	return &environment, err
}

func (s *Store) UpdateEnvironment(environment *model.Environment) error {
	return s.db.Save(environment).Error
}

func (s *Store) DeleteEnvironment(id uint) error {
	return s.db.Delete(&model.Environment{}, id).Error
}

func (s *Store) CountEnvironmentApplications(environmentID uint) (int64, error) {
	var count int64
	err := s.db.Model(&model.Application{}).Where("environment_id = ?", environmentID).Count(&count).Error
	return count, err
}

func (s *Store) CreateApplication(application *model.Application) error {
	return s.db.Create(application).Error
}

func (s *Store) CreateApplicationEndpoint(endpoint *model.ApplicationEndpoint) error {
	return s.db.Create(endpoint).Error
}

func (s *Store) GetApplication(id uint) (*model.Application, error) {
	var application model.Application
	err := s.db.Preload("Project.DefaultImageRegistry").Preload("Environment").Preload("Endpoints").First(&application, id).Error
	return &application, err
}

func (s *Store) ListApplications() ([]model.Application, error) {
	var applications []model.Application
	err := s.db.Preload("Project.DefaultImageRegistry").Preload("Environment").Preload("Endpoints").Order("created_at desc").Find(&applications).Error
	return applications, err
}

func (s *Store) CreateImageRegistry(registry *model.ImageRegistry, projectIDs []uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		projects, err := imageRegistryProjects(tx, projectIDs)
		if err != nil {
			return err
		}
		if err := tx.Create(registry).Error; err != nil {
			return err
		}
		return tx.Model(registry).Association("Projects").Replace(projects)
	})
}

func (s *Store) ListImageRegistries(projectID uint) ([]model.ImageRegistry, error) {
	var registries []model.ImageRegistry
	query := s.db.Preload("Projects").Order("created_at desc")
	if projectID != 0 {
		query = query.Joins("JOIN image_registry_projects ON image_registry_projects.image_registry_id = image_registries.id").Where("image_registry_projects.project_id = ?", projectID)
	}
	if err := query.Find(&registries).Error; err != nil {
		return nil, err
	}
	for index := range registries {
		registries[index].CredentialConfigured = registries[index].Credential != ""
	}
	return registries, nil
}

func (s *Store) GetImageRegistry(id uint) (*model.ImageRegistry, error) {
	var registry model.ImageRegistry
	err := s.db.Preload("Projects").First(&registry, id).Error
	if err == nil {
		registry.CredentialConfigured = registry.Credential != ""
	}
	return &registry, err
}

func (s *Store) GetImageRegistryForProject(id, projectID uint) (*model.ImageRegistry, error) {
	var registry model.ImageRegistry
	err := s.db.Preload("Projects").Joins("JOIN image_registry_projects ON image_registry_projects.image_registry_id = image_registries.id").Where("image_registries.id = ? AND image_registry_projects.project_id = ?", id, projectID).First(&registry).Error
	if err == nil {
		registry.CredentialConfigured = registry.Credential != ""
	}
	return &registry, err
}

func (s *Store) UpdateImageRegistry(registry *model.ImageRegistry, projectIDs []uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		projects, err := imageRegistryProjects(tx, projectIDs)
		if err != nil {
			return err
		}
		if err := tx.Omit("Projects").Save(registry).Error; err != nil {
			return err
		}
		return tx.Model(registry).Association("Projects").Replace(projects)
	})
}

func (s *Store) UpdateImageRegistryVerification(id uint, status, detail string, verifiedAt time.Time) error {
	return s.db.Model(&model.ImageRegistry{}).Where("id = ?", id).Updates(map[string]interface{}{
		"last_verified_at":   verifiedAt,
		"last_verify_status": status,
		"last_verify_error":  detail,
	}).Error
}

func (s *Store) CountImageRegistryReleases(id uint) (int64, error) {
	var count int64
	err := s.db.Model(&model.Release{}).Where("image_registry_id = ?", id).Count(&count).Error
	return count, err
}

func (s *Store) DeleteImageRegistry(id uint) error {
	return s.db.Delete(&model.ImageRegistry{}, id).Error
}

func (s *Store) CreateManagedDomain(domain *model.ManagedDomain) error {
	return s.db.Create(domain).Error
}
func (s *Store) ListManagedDomains() ([]model.ManagedDomain, error) {
	var domains []model.ManagedDomain
	err := s.db.Order("hostname asc").Find(&domains).Error
	return domains, err
}
func (s *Store) GetManagedDomain(id uint) (*model.ManagedDomain, error) {
	var domain model.ManagedDomain
	err := s.db.First(&domain, id).Error
	return &domain, err
}
func (s *Store) UpdateManagedDomain(domain *model.ManagedDomain) error {
	return s.db.Save(domain).Error
}
func (s *Store) DeleteManagedDomain(id uint) error {
	return s.db.Delete(&model.ManagedDomain{}, id).Error
}

func imageRegistryProjects(tx *gorm.DB, projectIDs []uint) ([]model.Project, error) {
	projects := make([]model.Project, 0, len(projectIDs))
	seen := make(map[uint]struct{}, len(projectIDs))
	for _, projectID := range projectIDs {
		if projectID == 0 {
			return nil, errors.New("项目 ID 无效")
		}
		if _, exists := seen[projectID]; exists {
			continue
		}
		seen[projectID] = struct{}{}
		var project model.Project
		if err := tx.First(&project, projectID).Error; err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	if len(projects) == 0 {
		return nil, errors.New("至少授权一个项目")
	}
	return projects, nil
}

func (s *Store) CreateRelease(release *model.Release) error {
	return s.db.Create(release).Error
}

func (s *Store) GetRelease(id uint) (*model.Release, error) {
	var release model.Release
	err := s.db.Preload("Operations").First(&release, id).Error
	return &release, err
}

func (s *Store) ListReleases(applicationID uint) ([]model.Release, error) {
	var releases []model.Release
	err := s.db.Where("application_id = ?", applicationID).Order("sequence desc").Find(&releases).Error
	return releases, err
}

func (s *Store) UpdateRelease(release *model.Release) error {
	return s.db.Save(release).Error
}

func (s *Store) GetLatestSuccessfulRelease(applicationID uint) (*model.Release, error) {
	var release model.Release
	err := s.db.Where("application_id = ? AND status = ?", applicationID, model.ReleaseStatusSucceeded).
		Order("sequence desc").First(&release).Error
	return &release, err
}

func (s *Store) CreateReleaseOperation(operation *model.ReleaseOperation) error {
	return s.db.Create(operation).Error
}

func (s *Store) UpdateReleaseOperation(operation *model.ReleaseOperation) error {
	return s.db.Save(operation).Error
}

func (s *Store) ListReleaseOperations(releaseID uint) ([]model.ReleaseOperation, error) {
	var operations []model.ReleaseOperation
	err := s.db.Where("release_id = ?", releaseID).Order("created_at asc").Find(&operations).Error
	return operations, err
}
