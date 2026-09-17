package store

import (
	"errors"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func (s *Store) GetActiveClusterDNSPolicy() (*model.ClusterDNSPolicy, error) {
	var policy model.ClusterDNSPolicy
	err := s.db.Where("active = ?", true).Order("revision desc").First(&policy).Error
	return &policy, err
}

func (s *Store) ListClusterDNSPolicies(limit int) ([]model.ClusterDNSPolicy, error) {
	if limit < 1 || limit > 50 {
		limit = 20
	}
	var policies []model.ClusterDNSPolicy
	err := s.db.Order("revision desc").Limit(limit).Find(&policies).Error
	return policies, err
}

func (s *Store) CreateClusterDNSPolicy(policy *model.ClusterDNSPolicy) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var maxRevision uint
		if err := tx.Model(&model.ClusterDNSPolicy{}).Select("COALESCE(MAX(revision), 0)").Scan(&maxRevision).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.ClusterDNSPolicy{}).Where("active = ?", true).Update("active", false).Error; err != nil {
			return err
		}
		policy.Revision, policy.Active = maxRevision+1, true
		return tx.Create(policy).Error
	})
}

// GetEnvironmentByID is shared infrastructure read state: PVC, domain and
// certificate workflows need the namespace but do not own Environment writes.
func (s *Store) GetEnvironmentByID(environmentID uint) (*model.Environment, error) {
	var environment model.Environment
	err := s.db.First(&environment, environmentID).Error
	return &environment, err
}

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
	now := time.Now()
	threshold := now.Add(time.Duration(daysBefore) * 24 * time.Hour)
	err := s.db.Where("status = ? AND valid_to > ? AND valid_to <= ?", "issued", now, threshold).Find(&certs).Error
	return certs, err
}

func (s *Store) UpdateCert(cert *model.Cert) error { return s.db.Save(cert).Error }

func (s *Store) DeleteCert(id uint) error { return s.db.Delete(&model.Cert{}, id).Error }
