package store

import (
	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

// cleanupLegacySchema removes schema from features retired by the application.
func cleanupLegacySchema(db *gorm.DB) error {
	if db.Migrator().HasIndex(&model.ApplicationDeploymentTemplate{}, "idx_application_deployment_templates_application_id") {
		if err := db.Migrator().DropIndex(&model.ApplicationDeploymentTemplate{}, "idx_application_deployment_templates_application_id"); err != nil {
			return err
		}
	}
	for _, column := range []string{"access_key_id", "access_key_secret"} {
		if db.Migrator().HasColumn("dns_credentials", column) {
			if err := db.Exec("ALTER TABLE dns_credentials DROP COLUMN " + column).Error; err != nil {
				return err
			}
		}
	}
	if err := removeObsoleteApplicationStackSchema(db); err != nil {
		return err
	}
	if err := removeObsoleteAssistantSchema(db); err != nil {
		return err
	}
	if err := removeObsoleteManagedOCIRegistrySchema(db); err != nil {
		return err
	}
	if db.Migrator().HasTable("managed_documents") {
		return db.Migrator().DropTable("managed_documents")
	}
	return nil
}

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

func removeObsoleteManagedOCIRegistrySchema(db *gorm.DB) error {
	if !db.Migrator().HasColumn(&model.ManagedOCIRegistry{}, "data_path") {
		return nil
	}
	return db.Migrator().DropColumn(&model.ManagedOCIRegistry{}, "data_path")
}
