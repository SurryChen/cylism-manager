package store

import (
	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

// migrateSchema creates and updates the current persistence schema.
func migrateSchema(db *gorm.DB) error {
	legacyCatalog := db.Migrator().HasTable(&model.ApplicationCatalogItem{}) && !db.Migrator().HasColumn(&model.ApplicationCatalogItem{}, "enabled")
	if err := db.AutoMigrate(
		&model.Cluster{}, &model.ClusterDeployment{}, &model.ClusterDeploymentLog{}, &model.CrossClusterService{}, &model.AccessCA{}, &model.AccessCertificate{}, &model.Server{}, &model.Site{}, &model.Cert{}, &model.AuditLog{},
		&model.OperationLog{}, &model.User{}, &model.TemporaryLoginToken{},
		&model.SystemConfig{}, &model.RuntimeInstance{}, &model.AgentCapabilityGrant{},
		&model.AgentOperation{}, &model.AlertEvent{}, &model.AlertAutomationPolicy{},
		&model.PlatformRelease{}, &model.PlatformWebhookNonce{}, &model.PlatformEndpoint{},
		&model.Project{}, &model.Environment{}, &model.Application{},
		&model.IntegrationSession{}, &model.ApplicationEndpoint{}, &model.ApplicationDeployWebhook{}, &model.ApplicationDeployWebhookNonce{},
		&model.ApplicationDeploymentTemplate{}, &model.ApplicationCatalogItem{}, &model.ApplicationCatalogInstallation{}, &model.ApplicationCatalogInstallationComponent{}, &model.ApplicationManagedFile{},
		&model.ImageRegistry{}, &model.NodeRegistryMirror{}, &model.NodeRegistryMirrorNode{},
		&model.ManagedOCIRegistry{}, &model.RegistryProxy{}, &model.RegistryProxyRelay{}, &model.RegistryProxyNodeRoute{}, &model.ClusterDNSPolicy{},
		&model.ChartRepository{}, &model.DNSCredential{}, &model.ManagedDomain{},
		&model.CloudConnection{}, &model.DatabaseInstance{}, &model.DatabaseBinding{}, &model.MySQLCluster{}, &model.MySQLClusterTask{}, &model.MySQLClusterTaskLog{},
		&model.Release{}, &model.ReleaseOperation{}, &model.PersistentVolumeMigration{},
		&model.PersistentVolumeBackup{}, &model.HostDirectoryPVCImport{},
		&model.SystemComponentConfig{},
	); err != nil {
		return err
	}
	if legacyCatalog {
		return db.Model(&model.ApplicationCatalogItem{}).Where("1 = 1").Update("enabled", true).Error
	}
	return nil
}
