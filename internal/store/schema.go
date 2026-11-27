package store

import (
	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

// migrateSchema creates and updates the current persistence schema.
func migrateSchema(db *gorm.DB) error {
	legacyCatalog := db.Migrator().HasTable(&model.ApplicationCatalogItem{}) && !db.Migrator().HasColumn(&model.ApplicationCatalogItem{}, "enabled")
	legacyRemoval := db.Migrator().HasTable(&model.Server{}) && !db.Migrator().HasColumn(&model.Server{}, "k3s_removal_task_id")
	if err := db.AutoMigrate(
		&model.Cluster{}, &model.ClusterDeployment{}, &model.ClusterDeploymentLog{}, &model.CrossClusterService{}, &model.AccessCA{}, &model.AccessCertificate{}, &model.Server{}, &model.ServerSpeedTest{}, &model.Site{}, &model.Cert{}, &model.AuditLog{},
		&model.OperationLog{}, &model.User{}, &model.TemporaryLoginToken{},
		&model.SystemConfig{}, &model.RuntimeInstance{}, &model.AgentCapabilityGrant{},
		&model.AgentOperation{}, &model.AlertEvent{}, &model.AlertHistory{}, &model.AlertSuppressionRule{}, &model.AlertAutomationPolicy{},
		&model.PlatformRelease{}, &model.PlatformWebhookNonce{}, &model.PlatformEndpoint{},
		&model.Project{}, &model.Environment{}, &model.Application{}, &model.ApplicationNamespaceMaintenance{},
		&model.IntegrationSession{}, &model.ApplicationEndpoint{}, &model.ApplicationDeployWebhook{}, &model.ApplicationDeployWebhookNonce{},
		&model.ApplicationDeploymentTemplate{}, &model.ApplicationCatalogItem{}, &model.ApplicationCatalogInstallation{}, &model.ApplicationCatalogInstallationComponent{}, &model.ApplicationManagedFile{},
		&model.ImageRegistry{}, &model.NodeRegistryMirror{}, &model.NodeRegistryMirrorNode{},
		&model.ManagedOCIRegistry{}, &model.RegistryProxy{}, &model.RegistryProxyRelay{}, &model.RegistryProxyNodeRoute{}, &model.ClusterDNSPolicy{},
		&model.ChartRepository{}, &model.DNSCredential{}, &model.ManagedDomain{},
		&model.CloudConnection{}, &model.DatabaseInstance{}, &model.DatabaseBinding{}, &model.DatabaseOperation{}, &model.DatabaseOperationLog{}, &model.DatabaseOperationLock{}, &model.DatabaseReplication{}, &model.DatabaseGateway{}, &model.DatabaseGatewayTask{}, &model.DatabaseGatewayTaskLog{}, &model.MySQLCluster{}, &model.MySQLClusterTask{}, &model.MySQLClusterTaskLog{},
		&model.Release{}, &model.ReleaseOperation{}, &model.PersistentVolumeMigration{},
		&model.PersistentVolumeBackup{}, &model.HostDirectoryPVCImport{},
		&model.SystemComponentConfig{},
		&model.ObservabilityMigration{},
		&model.Product{}, &model.ProductChannel{}, &model.ProductProject{}, &model.ProductApplication{}, &model.ProductReleaseUnit{},
		&model.ProductTaskGroup{}, &model.ProductTask{}, &model.ProductVersion{}, &model.ProductVersionTask{}, &model.ProductVersionDeployment{}, &model.ProductVersionPlatformRelease{},
		&model.ProductDistribution{}, &model.ProductDistributionEvent{},
		&model.ProductWeChatConnection{}, &model.WeChatDailyMetric{}, &model.WeChatComponentTicket{},
		&model.GitHubConnection{}, &model.GitHubRegistration{}, &model.ProductGitHubRepository{},
	); err != nil {
		return err
	}
	if err := db.Exec("DROP INDEX IF EXISTS idx_product_version_name").Error; err != nil {
		return err
	}
	if legacyRemoval {
		// Only a final successful safe removal proves that this unbound host
		// still has a stopped installation. Never infer it from a hostname.
		if err := db.Exec(`UPDATE servers SET k3s_removal_task_id = (
			SELECT id FROM cluster_deployments WHERE server_id = servers.id
			ORDER BY id DESC LIMIT 1
		) WHERE deleted_at IS NULL AND cluster_id IS NULL AND cluster_role = '' AND k8s_node_name = ''
		AND EXISTS (SELECT 1 FROM cluster_deployments d WHERE d.id = (
			SELECT id FROM cluster_deployments WHERE server_id = servers.id ORDER BY id DESC LIMIT 1
		) AND d.operation = 'remove' AND d.status = 'success' AND d.node_stopped = 1
		AND d.node_uninstalled = 0 AND d.node_machine_id <> '')`).Error; err != nil {
			return err
		}
	}
	if legacyCatalog {
		return db.Model(&model.ApplicationCatalogItem{}).Where("1 = 1").Update("enabled", true).Error
	}
	return nil
}
