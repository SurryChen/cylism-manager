package store

import (
	"errors"
	"fmt"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Store 数据存储层
type Store struct {
	db *gorm.DB
}

// NamespaceConflictError indicates that another environment already owns a Namespace.
type NamespaceConflictError struct {
	Namespace     string
	EnvironmentID uint
	ProjectID     uint
}

func (e *NamespaceConflictError) Error() string {
	return fmt.Sprintf("命名空间 %q 已被项目 %d 的环境 %d 绑定", e.Namespace, e.ProjectID, e.EnvironmentID)
}

// NamespaceConflict groups legacy duplicate bindings for an operator-directed migration.
type NamespaceConflict struct {
	Namespace    string              `json:"namespace"`
	Environments []model.Environment `json:"environments"`
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
		&model.AssistantProvider{},
		&model.AssistantConversation{},
		&model.AssistantMessage{},
		&model.PlatformRelease{},
		&model.PlatformWebhookNonce{},
		&model.Project{},
		&model.Environment{},
		&model.Application{},
		&model.ApplicationEndpoint{},
		&model.ApplicationDeploymentTemplate{},
		&model.ImageRegistry{},
		&model.NodeRegistryMirror{},
		&model.NodeRegistryMirrorNode{},
		&model.RegistryProxy{},
		&model.ChartRepository{},
		&model.DNSCredential{},
		&model.ManagedDomain{},
		&model.Release{},
		&model.ReleaseOperation{},
		&model.PersistentVolumeMigration{},
		&model.PersistentVolumeBackup{},
		&model.HostDirectoryPVCImport{},
	); err != nil {
		return nil, err
	}
	// Older builds used a one-template-per-application unique index. Drop that
	// database constraint before allowing multiple named templates.
	if db.Migrator().HasIndex(&model.ApplicationDeploymentTemplate{}, "idx_application_deployment_templates_application_id") {
		if err := db.Migrator().DropIndex(&model.ApplicationDeploymentTemplate{}, "idx_application_deployment_templates_application_id"); err != nil {
			return nil, err
		}
	}
	for _, column := range []string{"access_key_id", "access_key_secret"} {
		if db.Migrator().HasColumn(&model.DNSCredential{}, column) {
			if err := db.Migrator().DropColumn(&model.DNSCredential{}, column); err != nil {
				return nil, err
			}
		}
	}
	if err := removeObsoleteApplicationStackSchema(db); err != nil {
		return nil, err
	}

	store := &Store{db: db}
	if err := store.backfillManagedDomainEnvironments(); err != nil {
		return nil, err
	}
	if err := store.backfillApplicationDeploymentTemplates(); err != nil {
		return nil, err
	}
	if err := store.ReconcileEnvironmentNamespaceUniqueness(); err != nil {
		return nil, err
	}
	return store, nil
}

// removeObsoleteApplicationStackSchema permanently retires the removed
// application-stack feature. Stack data was never supported for migration.
func removeObsoleteApplicationStackSchema(db *gorm.DB) error {
	for _, table := range []string{"application_stack_releases", "application_stack_templates"} {
		if db.Migrator().HasTable(table) {
			if err := db.Migrator().DropTable(table); err != nil {
				return err
			}
		}
	}
	if db.Migrator().HasColumn("applications", "stack_template_id") {
		if err := db.Exec("DROP INDEX IF EXISTS idx_applications_stack_template_id").Error; err != nil {
			return err
		}
		return db.Exec("ALTER TABLE applications DROP COLUMN stack_template_id").Error
	}
	return nil
}

func (s *Store) backfillApplicationDeploymentTemplates() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var templates []model.ApplicationDeploymentTemplate
		if err := tx.Where("name = ''").Find(&templates).Error; err != nil {
			return err
		}
		for index := range templates {
			template := &templates[index]
			template.Name = "默认模板"
			template.Enabled = true
			if err := tx.Save(template).Error; err != nil {
				return err
			}
		}
		var applications []model.Application
		if err := tx.Where("default_deployment_template_id IS NULL").Find(&applications).Error; err != nil {
			return err
		}
		for index := range applications {
			var template model.ApplicationDeploymentTemplate
			err := tx.Where("application_id = ?", applications[index].ID).Order("id asc").First(&template).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if err := tx.Model(&applications[index]).Update("default_deployment_template_id", template.ID).Error; err != nil {
				return err
			}
		}
		return nil
	})
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

// UnbindServersFromClusterNode clears the cluster mapping for every server
// associated with a Kubernetes Node that no longer exists in the cluster.
func (s *Store) UnbindServersFromClusterNode(nodeName string) error {
	return s.db.Model(&model.Server{}).
		Where("k8s_node_name = ?", nodeName).
		Updates(map[string]interface{}{
			"cluster_role":  "",
			"k8s_node_name": "",
		}).Error
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

// --- Platform self-update ---

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

func (s *Store) CreatePlatformWebhookNonce(nonce string, expiresAt time.Time) error {
	return s.db.Create(&model.PlatformWebhookNonce{Nonce: nonce, ExpiresAt: expiresAt}).Error
}

func (s *Store) DeleteExpiredPlatformWebhookNonces(now time.Time) error {
	return s.db.Where("expires_at <= ?", now).Delete(&model.PlatformWebhookNonce{}).Error
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
	if environment == nil {
		return errors.New("environment is required")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := ensureNamespaceAvailable(tx, environment.Namespace, 0); err != nil {
			return err
		}
		return tx.Create(environment).Error
	})
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

func (s *Store) GetEnvironmentByID(environmentID uint) (*model.Environment, error) {
	var environment model.Environment
	err := s.db.First(&environment, environmentID).Error
	return &environment, err
}

func (s *Store) UpdateEnvironment(environment *model.Environment) error {
	if environment == nil {
		return errors.New("environment is required")
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := ensureNamespaceAvailable(tx, environment.Namespace, environment.ID); err != nil {
			return err
		}
		return tx.Save(environment).Error
	})
	if err != nil {
		return err
	}
	return s.ReconcileEnvironmentNamespaceUniqueness()
}

func (s *Store) DeleteEnvironment(id uint) error {
	if err := s.db.Delete(&model.Environment{}, id).Error; err != nil {
		return err
	}
	return s.ReconcileEnvironmentNamespaceUniqueness()
}

// EnsureNamespaceAvailable validates an intended namespace binding before Kubernetes is mutated.
func (s *Store) EnsureNamespaceAvailable(namespace string, excludeEnvironmentID uint) error {
	return ensureNamespaceAvailable(s.db, namespace, excludeEnvironmentID)
}

func ensureNamespaceAvailable(db *gorm.DB, namespace string, excludeEnvironmentID uint) error {
	var existing model.Environment
	query := db.Where("namespace = ?", namespace)
	if excludeEnvironmentID != 0 {
		query = query.Where("id <> ?", excludeEnvironmentID)
	}
	if err := query.Order("id asc").First(&existing).Error; err == nil {
		return &NamespaceConflictError{Namespace: namespace, EnvironmentID: existing.ID, ProjectID: existing.ProjectID}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return nil
}

// ListEnvironmentNamespaceConflicts returns duplicate legacy namespace bindings.
func (s *Store) ListEnvironmentNamespaceConflicts() ([]NamespaceConflict, error) {
	type duplicate struct{ Namespace string }
	var duplicates []duplicate
	if err := s.db.Model(&model.Environment{}).
		Select("namespace").
		Where("namespace <> ''").
		Group("namespace").
		Having("COUNT(*) > 1").
		Order("namespace asc").
		Find(&duplicates).Error; err != nil {
		return nil, err
	}
	conflicts := make([]NamespaceConflict, 0, len(duplicates))
	for _, duplicate := range duplicates {
		var environments []model.Environment
		if err := s.db.Where("namespace = ?", duplicate.Namespace).Order("project_id asc, id asc").Find(&environments).Error; err != nil {
			return nil, err
		}
		for index := range environments {
			environments[index].NamespaceConflict = true
		}
		conflicts = append(conflicts, NamespaceConflict{Namespace: duplicate.Namespace, Environments: environments})
	}
	return conflicts, nil
}

func (s *Store) IsEnvironmentNamespaceConflicted(namespace string) (bool, error) {
	var count int64
	if err := s.db.Model(&model.Environment{}).Where("namespace = ?", namespace).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 1, nil
}

// ReconcileEnvironmentNamespaceUniqueness creates the database backstop only after legacy duplicates are resolved.
func (s *Store) ReconcileEnvironmentNamespaceUniqueness() error {
	conflicts, err := s.ListEnvironmentNamespaceConflicts()
	if err != nil || len(conflicts) > 0 {
		return err
	}
	return s.db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_environments_namespace_unique ON environments(namespace)").Error
}

func (s *Store) backfillManagedDomainEnvironments() error {
	var domains []model.ManagedDomain
	if err := s.db.Where("environment_id = 0 AND namespace <> ''").Find(&domains).Error; err != nil {
		return err
	}
	for index := range domains {
		var environments []model.Environment
		if err := s.db.Where("namespace = ?", domains[index].Namespace).Find(&environments).Error; err != nil {
			return err
		}
		if len(environments) == 1 {
			if err := s.db.Model(&domains[index]).Update("environment_id", environments[0].ID).Error; err != nil {
				return err
			}
		}
	}
	return nil
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

func (s *Store) ListApplicationEndpoints(applicationID uint) ([]model.ApplicationEndpoint, error) {
	var endpoints []model.ApplicationEndpoint
	err := s.db.Where("application_id = ?", applicationID).Order("id asc").Find(&endpoints).Error
	return endpoints, err
}

func (s *Store) GetApplicationEndpoint(applicationID, endpointID uint) (*model.ApplicationEndpoint, error) {
	var endpoint model.ApplicationEndpoint
	err := s.db.Where("application_id = ? AND id = ?", applicationID, endpointID).First(&endpoint).Error
	return &endpoint, err
}

func (s *Store) UpdateApplicationEndpoint(endpoint *model.ApplicationEndpoint) error {
	return s.db.Save(endpoint).Error
}

func (s *Store) DeleteApplicationEndpoint(applicationID, endpointID uint) error {
	return s.db.Where("application_id = ? AND id = ?", applicationID, endpointID).Delete(&model.ApplicationEndpoint{}).Error
}

func (s *Store) GetApplicationDeploymentTemplate(applicationID, templateID uint) (*model.ApplicationDeploymentTemplate, error) {
	var template model.ApplicationDeploymentTemplate
	err := s.db.Where("application_id = ? AND id = ?", applicationID, templateID).First(&template).Error
	return &template, err
}

func (s *Store) GetDefaultApplicationDeploymentTemplate(applicationID uint) (*model.ApplicationDeploymentTemplate, error) {
	var application model.Application
	if err := s.db.First(&application, applicationID).Error; err != nil {
		return nil, err
	}
	if application.DefaultDeploymentTemplateID == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return s.GetApplicationDeploymentTemplate(applicationID, *application.DefaultDeploymentTemplateID)
}

func (s *Store) ListApplicationDeploymentTemplates(applicationID uint) ([]model.ApplicationDeploymentTemplate, error) {
	var templates []model.ApplicationDeploymentTemplate
	err := s.db.Where("application_id = ?", applicationID).Order("created_at asc").Find(&templates).Error
	return templates, err
}

func (s *Store) CreateApplicationDeploymentTemplate(template *model.ApplicationDeploymentTemplate, makeDefault bool) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		template.Revision = 1
		if err := tx.Create(template).Error; err != nil {
			return err
		}
		if makeDefault {
			return tx.Model(&model.Application{}).Where("id = ?", template.ApplicationID).Update("default_deployment_template_id", template.ID).Error
		}
		return nil
	})
}

func (s *Store) UpdateApplicationDeploymentTemplate(template *model.ApplicationDeploymentTemplate) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var current model.ApplicationDeploymentTemplate
		if err := tx.Where("id = ? AND application_id = ?", template.ID, template.ApplicationID).First(&current).Error; err != nil {
			return err
		}
		template.CreatedAt = current.CreatedAt
		template.Revision = current.Revision + 1
		return tx.Save(template).Error
	})
}

func (s *Store) SetDefaultApplicationDeploymentTemplate(applicationID, templateID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var template model.ApplicationDeploymentTemplate
		if err := tx.Where("id = ? AND application_id = ? AND enabled = ?", templateID, applicationID, true).First(&template).Error; err != nil {
			return err
		}
		return tx.Model(&model.Application{}).Where("id = ?", applicationID).Update("default_deployment_template_id", templateID).Error
	})
}

func (s *Store) DeleteApplicationDeploymentTemplate(applicationID, templateID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var releases int64
		if err := tx.Model(&model.Release{}).Where("template_id = ?", templateID).Count(&releases).Error; err != nil {
			return err
		}
		if releases > 0 {
			return fmt.Errorf("该模板已有关联发布记录，无法删除")
		}
		if err := tx.Where("id = ? AND application_id = ?", templateID, applicationID).Delete(&model.ApplicationDeploymentTemplate{}).Error; err != nil {
			return err
		}
		return tx.Model(&model.Application{}).Where("id = ? AND default_deployment_template_id = ?", applicationID, templateID).Update("default_deployment_template_id", nil).Error
	})
}

func (s *Store) CountApplicationEndpointsByDomain(domainID uint) (int64, error) {
	var count int64
	err := s.db.Model(&model.ApplicationEndpoint{}).Where("domain_id = ?", domainID).Count(&count).Error
	return count, err
}

func (s *Store) CountApplicationEndpointRoute(domainID uint, path string, exceptEndpointID uint) (int64, error) {
	var count int64
	query := s.db.Model(&model.ApplicationEndpoint{}).Where("domain_id = ? AND path = ?", domainID, path)
	if exceptEndpointID != 0 {
		query = query.Where("id <> ?", exceptEndpointID)
	}
	err := query.Count(&count).Error
	return count, err
}

func (s *Store) GetApplication(id uint) (*model.Application, error) {
	var application model.Application
	err := s.db.Preload("Project.DefaultImageRegistry").Preload("Environment").Preload("Endpoints", func(db *gorm.DB) *gorm.DB { return db.Order("id asc") }).First(&application, id).Error
	return &application, err
}

func (s *Store) GetApplicationByEnvironmentName(environmentID uint, name string) (*model.Application, error) {
	var application model.Application
	err := s.db.Preload("Project.DefaultImageRegistry").Preload("Environment").Where("environment_id = ? AND name = ?", environmentID, name).First(&application).Error
	return &application, err
}

func (s *Store) ListApplications(scope ...uint) ([]model.Application, error) {
	var applications []model.Application
	query := s.db.Preload("Project.DefaultImageRegistry").Preload("Environment").Preload("Endpoints", func(db *gorm.DB) *gorm.DB { return db.Order("id asc") }).Order("created_at desc")
	if len(scope) > 0 && scope[0] != 0 {
		query = query.Where("project_id = ?", scope[0])
	}
	if len(scope) > 1 && scope[1] != 0 {
		query = query.Where("environment_id = ?", scope[1])
	}
	err := query.Find(&applications).Error
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

// --- Node registry mirrors ---

func (s *Store) CreateNodeRegistryMirror(mirror *model.NodeRegistryMirror) error {
	return s.db.Create(mirror).Error
}

func (s *Store) GetNodeRegistryMirror(id uint) (*model.NodeRegistryMirror, error) {
	var mirror model.NodeRegistryMirror
	err := s.db.Preload("NodeStatuses.Server").First(&mirror, id).Error
	if err == nil {
		mirror.CredentialConfigured = mirror.Credential != ""
	}
	return &mirror, err
}

func (s *Store) ListNodeRegistryMirrors() ([]model.NodeRegistryMirror, error) {
	var mirrors []model.NodeRegistryMirror
	err := s.db.Preload("NodeStatuses.Server").Order("created_at desc").Find(&mirrors).Error
	for index := range mirrors {
		mirrors[index].CredentialConfigured = mirrors[index].Credential != ""
	}
	return mirrors, err
}

func (s *Store) UpdateNodeRegistryMirror(mirror *model.NodeRegistryMirror) error {
	return s.db.Save(mirror).Error
}

func (s *Store) UpdateNodeRegistryMirrorVerification(id uint, status, detail string, verifiedAt time.Time) error {
	return s.db.Model(&model.NodeRegistryMirror{}).Where("id = ?", id).Updates(map[string]interface{}{
		"last_verified_at":   verifiedAt,
		"last_verify_status": status,
		"last_verify_error":  detail,
	}).Error
}

func (s *Store) DeleteNodeRegistryMirror(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("mirror_id = ?", id).Delete(&model.NodeRegistryMirrorNode{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.NodeRegistryMirror{}, id).Error
	})
}

func (s *Store) UpsertNodeRegistryMirrorStatus(status *model.NodeRegistryMirrorNode) error {
	return s.db.Where("mirror_id = ? AND server_id = ?", status.MirrorID, status.ServerID).Assign(status).FirstOrCreate(&model.NodeRegistryMirrorNode{}).Error
}

func (s *Store) GetRegistryProxy() (*model.RegistryProxy, error) {
	var proxy model.RegistryProxy
	err := s.db.Order("id asc").First(&proxy).Error
	return &proxy, err
}

func (s *Store) GetRegistryProxyByID(id uint) (*model.RegistryProxy, error) {
	var proxy model.RegistryProxy
	err := s.db.First(&proxy, id).Error
	return &proxy, err
}

func (s *Store) ListRegistryProxies() ([]model.RegistryProxy, error) {
	var proxies []model.RegistryProxy
	err := s.db.Order("created_at asc").Find(&proxies).Error
	return proxies, err
}

func (s *Store) SaveRegistryProxy(proxy *model.RegistryProxy) error {
	return s.db.Save(proxy).Error
}

func (s *Store) CreateChartRepository(repository *model.ChartRepository) error {
	return s.db.Create(repository).Error
}
func (s *Store) ListChartRepositories() ([]model.ChartRepository, error) {
	var repositories []model.ChartRepository
	err := s.db.Order("created_at desc").Find(&repositories).Error
	return repositories, err
}
func (s *Store) GetChartRepository(id uint) (*model.ChartRepository, error) {
	var repository model.ChartRepository
	err := s.db.First(&repository, id).Error
	return &repository, err
}
func (s *Store) UpdateChartRepository(repository *model.ChartRepository) error {
	return s.db.Save(repository).Error
}
func (s *Store) DeleteChartRepository(id uint) error {
	return s.db.Delete(&model.ChartRepository{}, id).Error
}
func (s *Store) GetVerifiedCertManagerChartRepository() (*model.ChartRepository, error) {
	var repository model.ChartRepository
	err := s.db.Where("enabled = ? AND chart_name = ? AND last_verify_status = ?", true, "cert-manager", "succeeded").Order("updated_at desc").First(&repository).Error
	return &repository, err
}

// --- DNS credentials ---

func (s *Store) CreateDNSCredential(credential *model.DNSCredential) error {
	return s.db.Create(credential).Error
}

func (s *Store) ListDNSCredentials() ([]model.DNSCredential, error) {
	var credentials []model.DNSCredential
	err := s.db.Order("created_at desc").Find(&credentials).Error
	for index := range credentials {
		credentials[index].SecretConfigured = credentials[index].EncryptedValues != ""
	}
	return credentials, err
}

func (s *Store) GetDNSCredential(id uint) (*model.DNSCredential, error) {
	var credential model.DNSCredential
	err := s.db.First(&credential, id).Error
	if err == nil {
		credential.SecretConfigured = credential.EncryptedValues != ""
	}
	return &credential, err
}

func (s *Store) UpdateDNSCredential(credential *model.DNSCredential) error {
	return s.db.Save(credential).Error
}

func (s *Store) DeleteDNSCredential(id uint) error {
	return s.db.Delete(&model.DNSCredential{}, id).Error
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
		if err := tx.Model(registry).Association("Projects").Replace(projects); err != nil {
			return err
		}
		defaults := tx.Model(&model.Project{}).Where("default_image_registry_id = ?", registry.ID)
		if registry.Enabled && len(projectIDs) > 0 {
			defaults = defaults.Where("id NOT IN ?", projectIDs)
		}
		return defaults.Update("default_image_registry_id", nil).Error
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
func (s *Store) ListManagedDomains(environmentID ...uint) ([]model.ManagedDomain, error) {
	var domains []model.ManagedDomain
	query := s.db.Order("hostname asc")
	if len(environmentID) > 0 && environmentID[0] != 0 {
		query = query.Where("environment_id = ?", environmentID[0])
	}
	err := query.Find(&domains).Error
	return domains, err
}

func (s *Store) ListUnassignedManagedDomains() ([]model.ManagedDomain, error) {
	var domains []model.ManagedDomain
	err := s.db.Where("environment_id = 0").Order("hostname asc").Find(&domains).Error
	return domains, err
}

func (s *Store) ListClaimableManagedDomains(namespace string) ([]model.ManagedDomain, error) {
	var domains []model.ManagedDomain
	err := s.db.Where("environment_id = 0 AND namespace = ?", namespace).Order("hostname asc").Find(&domains).Error
	return domains, err
}
func (s *Store) GetManagedDomain(id uint) (*model.ManagedDomain, error) {
	var domain model.ManagedDomain
	err := s.db.First(&domain, id).Error
	return &domain, err
}

func (s *Store) GetManagedDomainByHostname(hostname string) (*model.ManagedDomain, error) {
	var domain model.ManagedDomain
	err := s.db.Where("hostname = ?", hostname).First(&domain).Error
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
	return projects, nil
}

func (s *Store) CreateRelease(release *model.Release) error {
	return s.db.Create(release).Error
}

func (s *Store) UpdateApplication(application *model.Application) error {
	return s.db.Save(application).Error
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

func (s *Store) CreatePersistentVolumeMigration(migration *model.PersistentVolumeMigration) error {
	return s.db.Create(migration).Error
}

func (s *Store) GetPersistentVolumeMigration(id uint) (*model.PersistentVolumeMigration, error) {
	var migration model.PersistentVolumeMigration
	err := s.db.First(&migration, id).Error
	return &migration, err
}

func (s *Store) ListPersistentVolumeMigrations(environmentID uint) ([]model.PersistentVolumeMigration, error) {
	var migrations []model.PersistentVolumeMigration
	query := s.db.Order("created_at desc")
	if environmentID != 0 {
		query = query.Where("environment_id = ?", environmentID)
	}
	err := query.Find(&migrations).Error
	return migrations, err
}

func (s *Store) FindActivePVCMigration(environmentID uint, sourcePVCName string) (*model.PersistentVolumeMigration, error) {
	var migration model.PersistentVolumeMigration
	terminal := []string{model.PVCMigrationStatusSucceeded, model.PVCMigrationStatusFailed, model.PVCMigrationStatusRolledBack, model.PVCMigrationStatusCleaned}
	err := s.db.Where("environment_id = ? AND source_pvc_name = ? AND status NOT IN ?", environmentID, sourcePVCName, terminal).Order("created_at desc").First(&migration).Error
	return &migration, err
}

func (s *Store) UpdatePersistentVolumeMigration(migration *model.PersistentVolumeMigration, status, detail string) error {
	migration.Status = status
	migration.Detail = detail
	if model.IsPVCMigrationTerminal(status) {
		now := time.Now()
		migration.CompletedAt = &now
	}
	return s.db.Save(migration).Error
}

func (s *Store) CreatePersistentVolumeBackup(backup *model.PersistentVolumeBackup) error {
	return s.db.Create(backup).Error
}

func (s *Store) GetPersistentVolumeBackup(id uint) (*model.PersistentVolumeBackup, error) {
	var backup model.PersistentVolumeBackup
	err := s.db.First(&backup, id).Error
	return &backup, err
}

func (s *Store) ListPersistentVolumeBackups(environmentID uint, pvcName string) ([]model.PersistentVolumeBackup, error) {
	var backups []model.PersistentVolumeBackup
	err := s.db.Where("environment_id = ? AND pvc_name = ?", environmentID, pvcName).Order("created_at desc").Find(&backups).Error
	return backups, err
}

func (s *Store) UpdatePersistentVolumeBackup(backup *model.PersistentVolumeBackup) error {
	return s.db.Save(backup).Error
}

func (s *Store) CreateHostDirectoryPVCImport(task *model.HostDirectoryPVCImport) error {
	return s.db.Create(task).Error
}

func (s *Store) GetHostDirectoryPVCImport(id uint) (*model.HostDirectoryPVCImport, error) {
	var task model.HostDirectoryPVCImport
	err := s.db.First(&task, id).Error
	return &task, err
}

func (s *Store) ListHostDirectoryPVCImports(environmentID uint, pvcName string) ([]model.HostDirectoryPVCImport, error) {
	var tasks []model.HostDirectoryPVCImport
	err := s.db.Where("environment_id = ? AND pvc_name = ?", environmentID, pvcName).Order("created_at desc").Find(&tasks).Error
	return tasks, err
}

func (s *Store) FindActiveHostDirectoryPVCImport(environmentID uint, pvcName string) (*model.HostDirectoryPVCImport, error) {
	var task model.HostDirectoryPVCImport
	err := s.db.Where("environment_id = ? AND pvc_name = ? AND status NOT IN ?", environmentID, pvcName, []string{model.PVCImportStatusSucceeded, model.PVCImportStatusFailed}).Order("created_at desc").First(&task).Error
	return &task, err
}

func (s *Store) UpdateHostDirectoryPVCImport(task *model.HostDirectoryPVCImport, status, detail string) error {
	task.Status = status
	task.Detail = detail
	if model.IsPVCImportTerminal(status) && task.CompletedAt == nil {
		now := time.Now()
		task.CompletedAt = &now
	}
	return s.db.Save(task).Error
}
