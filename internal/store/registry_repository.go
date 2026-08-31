package store

import (
	"errors"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

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

func (s *Store) CreateManagedOCIRegistry(registry *model.ManagedOCIRegistry, imageRegistry *model.ImageRegistry, mirror *model.NodeRegistryMirror, projectIDs []uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		projects, err := imageRegistryProjects(tx, projectIDs)
		if err != nil {
			return err
		}
		if err := tx.Create(registry).Error; err != nil {
			return err
		}
		imageRegistry.ManagedRegistryID = &registry.ID
		if err := tx.Create(imageRegistry).Error; err != nil {
			return err
		}
		if err := tx.Model(imageRegistry).Association("Projects").Replace(projects); err != nil {
			return err
		}
		mirror.ManagedRegistryID = &registry.ID
		if err := tx.Create(mirror).Error; err != nil {
			return err
		}
		registry.ImageRegistryID, registry.NodeRegistryMirrorID = &imageRegistry.ID, &mirror.ID
		return tx.Save(registry).Error
	})
}

func (s *Store) GetManagedOCIRegistry(id uint) (*model.ManagedOCIRegistry, error) {
	var registry model.ManagedOCIRegistry
	err := s.db.First(&registry, id).Error
	if err == nil {
		registry.CredentialConfigured = registry.EncryptedCredential != ""
	}
	return &registry, err
}

func (s *Store) GetManagedOCIRegistryByEndpoint(endpoint string) (*model.ManagedOCIRegistry, error) {
	var registry model.ManagedOCIRegistry
	err := s.db.Where("endpoint = ?", endpoint).First(&registry).Error
	if err == nil {
		registry.CredentialConfigured = registry.EncryptedCredential != ""
	}
	return &registry, err
}

func (s *Store) ListManagedOCIRegistries() ([]model.ManagedOCIRegistry, error) {
	var registries []model.ManagedOCIRegistry
	err := s.db.Order("created_at desc").Find(&registries).Error
	for i := range registries {
		registries[i].CredentialConfigured = registries[i].EncryptedCredential != ""
	}
	return registries, err
}

func (s *Store) UpdateManagedOCIRegistry(registry *model.ManagedOCIRegistry) error {
	return s.db.Save(registry).Error
}
func (s *Store) DeleteManagedOCIRegistry(id uint) error {
	return s.db.Delete(&model.ManagedOCIRegistry{}, id).Error
}

func (s *Store) CountManagedOCIRegistryReferences(registryID uint) (releases, projectDefaults int64, err error) {
	registry, err := s.GetManagedOCIRegistry(registryID)
	if err != nil || registry.ImageRegistryID == nil {
		return 0, 0, err
	}
	if err = s.db.Model(&model.Release{}).Where("image_registry_id = ?", *registry.ImageRegistryID).Count(&releases).Error; err != nil {
		return 0, 0, err
	}
	err = s.db.Model(&model.Project{}).Where("default_image_registry_id = ?", *registry.ImageRegistryID).Count(&projectDefaults).Error
	return releases, projectDefaults, err
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
	for i := range registries {
		registries[i].CredentialConfigured = registries[i].Credential != ""
	}
	return registries, nil
}

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
func (s *Store) GetNodeRegistryMirrorByRegistry(registry string) (*model.NodeRegistryMirror, error) {
	var mirror model.NodeRegistryMirror
	err := s.db.Preload("NodeStatuses.Server").Where("registry = ?", registry).First(&mirror).Error
	if err == nil {
		mirror.CredentialConfigured = mirror.Credential != ""
	}
	return &mirror, err
}
func (s *Store) ListNodeRegistryMirrors() ([]model.NodeRegistryMirror, error) {
	var mirrors []model.NodeRegistryMirror
	err := s.db.Preload("NodeStatuses.Server").Order("created_at desc").Find(&mirrors).Error
	for i := range mirrors {
		mirrors[i].CredentialConfigured = mirrors[i].Credential != ""
	}
	return mirrors, err
}
func (s *Store) UpdateNodeRegistryMirror(mirror *model.NodeRegistryMirror) error {
	return s.db.Save(mirror).Error
}
func (s *Store) UpdateNodeRegistryMirrorVerification(id uint, status, detail string, verifiedAt time.Time) error {
	return s.db.Model(&model.NodeRegistryMirror{}).Where("id = ?", id).Updates(map[string]interface{}{"last_verified_at": verifiedAt, "last_verify_status": status, "last_verify_error": detail}).Error
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
func (s *Store) SaveRegistryProxy(proxy *model.RegistryProxy) error { return s.db.Save(proxy).Error }

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

func (s *Store) GetImageRegistry(id uint) (*model.ImageRegistry, error) {
	var registry model.ImageRegistry
	err := s.db.Preload("Projects").First(&registry, id).Error
	if err == nil {
		registry.CredentialConfigured = registry.Credential != ""
	}
	return &registry, err
}
func (s *Store) GetImageRegistryByEndpoint(endpoint string) (*model.ImageRegistry, error) {
	var registry model.ImageRegistry
	err := s.db.Preload("Projects").Where("endpoint = ?", endpoint).First(&registry).Error
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
	return s.db.Model(&model.ImageRegistry{}).Where("id = ?", id).Updates(map[string]interface{}{"last_verified_at": verifiedAt, "last_verify_status": status, "last_verify_error": detail}).Error
}
func (s *Store) CountImageRegistryReleases(id uint) (int64, error) {
	var count int64
	err := s.db.Model(&model.Release{}).Where("image_registry_id = ?", id).Count(&count).Error
	return count, err
}
func (s *Store) DeleteImageRegistry(id uint) error {
	return s.db.Delete(&model.ImageRegistry{}, id).Error
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
