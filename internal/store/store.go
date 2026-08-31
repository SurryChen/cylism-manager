package store

import (
	"errors"
	"fmt"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/runtime"
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
		&model.RuntimeInstance{},
		&model.AgentCapabilityGrant{},
		&model.AgentOperation{},
		&model.AlertEvent{},
		&model.AlertAutomationPolicy{},
		&model.PlatformRelease{},
		&model.PlatformWebhookNonce{},
		&model.PlatformEndpoint{},
		&model.Project{},
		&model.Environment{},
		&model.Application{},
		&model.IntegrationSession{},
		&model.ApplicationEndpoint{},
		&model.ApplicationDeploymentTemplate{},
		&model.ApplicationManagedFile{},
		&model.ImageRegistry{},
		&model.NodeRegistryMirror{},
		&model.NodeRegistryMirrorNode{},
		&model.ManagedOCIRegistry{},
		&model.RegistryProxy{},
		&model.ClusterDNSPolicy{},
		&model.ChartRepository{},
		&model.DNSCredential{},
		&model.ManagedDomain{},
		&model.Release{},
		&model.ReleaseOperation{},
		&model.PersistentVolumeMigration{},
		&model.PersistentVolumeBackup{},
		&model.HostDirectoryPVCImport{},
		&model.SystemComponentConfig{},
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
	if err := removeObsoleteAssistantSchema(db); err != nil {
		return nil, err
	}
	if err := removeObsoleteManagedOCIRegistrySchema(db); err != nil {
		return nil, err
	}
	if db.Migrator().HasTable("managed_documents") {
		if err := db.Migrator().DropTable("managed_documents"); err != nil {
			return nil, err
		}
	}

	store := &Store{db: db}
	if err := store.backfillManagedDomainEnvironments(); err != nil {
		return nil, err
	}
	if err := store.backfillApplicationDeploymentTemplates(); err != nil {
		return nil, err
	}
	if err := store.backfillRuntimeVersions(); err != nil {
		return nil, err
	}
	if err := store.ReconcileEnvironmentNamespaceUniqueness(); err != nil {
		return nil, err
	}
	return store, nil
}

// backfillRuntimeVersions derives runtime versions from image tags for records
// created before image-tag version detection. Idempotent: only empty versions
// are touched, so reruns are safe.
func (s *Store) backfillRuntimeVersions() error {
	var runtimes []model.RuntimeInstance
	if err := s.db.Where("runtime_version = ?", "").Find(&runtimes).Error; err != nil {
		return fmt.Errorf("查询待回填的 Runtime 版本: %w", err)
	}
	for index := range runtimes {
		version := runtime.ImageVersion(runtimes[index].Image)
		if version == "" {
			continue
		}
		if err := s.db.Model(&runtimes[index]).Update("runtime_version", version).Error; err != nil {
			return fmt.Errorf("回填 Runtime %s 版本: %w", runtimes[index].Name, err)
		}
	}
	return nil
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

// removeObsoleteAssistantSchema permanently retires the former PydanticAI
// Runtime data. New agent runtimes own their sessions and memory themselves.
func removeObsoleteAssistantSchema(db *gorm.DB) error {
	if db.Migrator().HasTable("system_configs") {
		if err := db.Where("key = ?", "assistant_default_provider_id").Delete(&model.SystemConfig{}).Error; err != nil {
			return err
		}
	}
	for _, table := range []string{"assistant_messages", "assistant_conversations", "assistant_runtime_migrations", "assistant_providers"} {
		if db.Migrator().HasTable(table) {
			if err := db.Migrator().DropTable(table); err != nil {
				return err
			}
		}
	}
	return nil
}

// removeObsoleteManagedOCIRegistrySchema retires the hostPath-only column
// left by releases before Registry storage moved to PVCs. AutoMigrate adds
// fields but does not remove obsolete NOT NULL columns, which blocks inserts.
func removeObsoleteManagedOCIRegistrySchema(db *gorm.DB) error {
	if !db.Migrator().HasColumn(&model.ManagedOCIRegistry{}, "data_path") {
		return nil
	}
	return db.Migrator().DropColumn(&model.ManagedOCIRegistry{}, "data_path")
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
