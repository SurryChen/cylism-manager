package store

import (
	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

// migrateSchema creates and updates the current persistence schema.
func migrateSchema(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.Server{}, &model.Site{}, &model.Cert{}, &model.AuditLog{},
		&model.OperationLog{}, &model.User{}, &model.TemporaryLoginToken{},
		&model.SystemConfig{}, &model.RuntimeInstance{}, &model.AgentCapabilityGrant{},
		&model.AgentOperation{}, &model.AlertEvent{}, &model.AlertAutomationPolicy{},
		&model.PlatformRelease{}, &model.PlatformWebhookNonce{}, &model.PlatformEndpoint{},
		&model.Project{}, &model.Environment{}, &model.Application{},
		&model.IntegrationSession{}, &model.ApplicationEndpoint{},
		&model.ApplicationDeploymentTemplate{}, &model.ApplicationManagedFile{},
		&model.ImageRegistry{}, &model.NodeRegistryMirror{}, &model.NodeRegistryMirrorNode{},
		&model.ManagedOCIRegistry{}, &model.RegistryProxy{}, &model.ClusterDNSPolicy{},
		&model.ChartRepository{}, &model.DNSCredential{}, &model.ManagedDomain{},
		&model.Release{}, &model.ReleaseOperation{}, &model.PersistentVolumeMigration{},
		&model.PersistentVolumeBackup{}, &model.HostDirectoryPVCImport{},
		&model.SystemComponentConfig{},
	)
}
