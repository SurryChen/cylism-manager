package store

import (
	"errors"
	"fmt"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func (s *Store) CreateRelease(release *model.Release) error {
	return s.db.Create(release).Error
}

func (s *Store) UpdateApplication(application *model.Application) error {
	if err := application.SetCapabilities(application.Capabilities); err != nil {
		return err
	}
	return s.db.Save(application).Error
}

func (s *Store) CreateApplicationManagedFile(file *model.ApplicationManagedFile) error {
	if file.Version == 0 {
		file.Version = 1
	}
	if file.Format == "" {
		file.Format = "text"
	}
	return s.db.Create(file).Error
}

func (s *Store) ListApplicationManagedFiles(applicationID uint) ([]model.ApplicationManagedFile, error) {
	var files []model.ApplicationManagedFile
	err := s.db.Where("application_id = ? AND enabled = ?", applicationID, true).Order("id asc").Find(&files).Error
	return files, err
}

func (s *Store) GetApplicationManagedFile(applicationID, fileID uint) (*model.ApplicationManagedFile, error) {
	var file model.ApplicationManagedFile
	err := s.db.Where("application_id = ? AND id = ? AND enabled = ?", applicationID, fileID, true).First(&file).Error
	return &file, err
}

func (s *Store) UpsertApplicationManagedFile(file *model.ApplicationManagedFile) error {
	var existing model.ApplicationManagedFile
	err := s.db.Where("application_id = ? AND resource_kind = ? AND resource_name = ? AND key = ?", file.ApplicationID, file.ResourceKind, file.ResourceName, file.Key).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.CreateApplicationManagedFile(file)
	}
	if err != nil {
		return err
	}
	file.ID = existing.ID
	file.Version = existing.Version
	file.CreatedAt = existing.CreatedAt
	return s.db.Model(&existing).Updates(map[string]interface{}{"mount_path": file.MountPath, "format": file.Format, "enabled": file.Enabled}).Error
}

// DisableApplicationManagedFilesNotIn disables bindings that are no longer
// present in the published template. The row is retained so a later re-enable
// keeps its optimistic version history.
func (s *Store) DisableApplicationManagedFilesNotIn(applicationID uint, bindings map[string]struct{}) error {
	files, err := s.ListApplicationManagedFiles(applicationID)
	if err != nil {
		return err
	}
	for _, file := range files {
		binding := fmt.Sprintf("%s\x00%s\x00%s", file.ResourceKind, file.ResourceName, file.Key)
		if _, keep := bindings[binding]; keep {
			continue
		}
		if err := s.db.Model(&model.ApplicationManagedFile{}).Where("id = ?", file.ID).Update("enabled", false).Error; err != nil {
			return err
		}
	}
	return nil
}

// AdvanceApplicationManagedFileVersion is optimistic concurrency protection
// for a resource mutation that has already succeeded in Kubernetes.
func (s *Store) AdvanceApplicationManagedFileVersion(applicationID, fileID, expected uint) (uint, error) {
	result := s.db.Model(&model.ApplicationManagedFile{}).
		Where("application_id = ? AND id = ? AND version = ? AND enabled = ?", applicationID, fileID, expected, true).
		Update("version", expected+1)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected != 1 {
		return 0, gorm.ErrRecordNotFound
	}
	return expected + 1, nil
}

// ReplaceApplicationCapabilities atomically replaces opaque application-level
// metadata without changing its templates or release history.
func (s *Store) ReplaceApplicationCapabilities(applicationID uint, values []string) (*model.Application, error) {
	capabilityHolder := &model.Application{}
	if err := capabilityHolder.SetCapabilities(values); err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.Application{}).Where("id = ?", applicationID).Update("capabilities", capabilityHolder.CapabilitiesData).Error; err != nil {
		return nil, err
	}
	return s.GetApplication(applicationID)
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

// ListReleasesByApplications loads release history for a workspace in one
// query. The workspace view otherwise turns this into one query per app.
func (s *Store) ListReleasesByApplications(applicationIDs []uint) (map[uint][]model.Release, error) {
	result := make(map[uint][]model.Release, len(applicationIDs))
	if len(applicationIDs) == 0 {
		return result, nil
	}
	var releases []model.Release
	if err := s.db.Where("application_id IN ?", applicationIDs).Order("sequence desc").Find(&releases).Error; err != nil {
		return nil, err
	}
	for _, release := range releases {
		result[release.ApplicationID] = append(result[release.ApplicationID], release)
	}
	return result, nil
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
