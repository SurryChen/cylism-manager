package store

import (
	"fmt"
	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
	"slices"
)

// CheckApplicationNamespaceMove keeps project ownership and physical scope
// independent until adoption has succeeded.
func (s *Store) CheckApplicationNamespaceMove(move model.ApplicationNamespaceMove) error {
	_, _, err := s.checkApplicationNamespaceMove(move)
	return err
}

func (s *Store) checkApplicationNamespaceMove(move model.ApplicationNamespaceMove) (*model.Application, *model.Environment, error) {
	app, err := s.GetApplication(move.ApplicationID)
	if err != nil {
		return nil, nil, err
	}
	if move.OwnerID == 0 || app.Project.OwnerID != move.OwnerID {
		return nil, nil, gorm.ErrRecordNotFound
	}
	target, err := s.GetEnvironment(app.OwningProjectID(), move.TargetEnvironmentID)
	if err != nil {
		return nil, nil, err
	}
	project, err := s.GetProject(target.ProjectID)
	if err != nil {
		return nil, nil, err
	}
	if project.OwnerID != move.OwnerID || target.ClusterID == 0 || target.ClusterID != app.Environment.ClusterID {
		return nil, nil, model.ErrApplicationTransferTarget
	}
	if app.EnvironmentID == target.ID {
		return app, target, nil
	}
	if app.EnvironmentID != move.SourceEnvironmentID || app.Environment.Namespace == target.Namespace {
		return nil, nil, model.ErrApplicationNamespaceMoveChanged
	}
	var latest model.Release
	if err := s.db.Where("application_id = ? AND status = ?", app.ID, model.ReleaseStatusSucceeded).Order("sequence DESC").First(&latest).Error; err != nil {
		return nil, nil, err
	}
	if latest.ID != move.ReleaseID {
		return nil, nil, model.ErrApplicationNamespaceMoveChanged
	}
	var active int64
	if err := s.db.Model(&model.Release{}).Where("application_id = ? AND status NOT IN ?", app.ID, []string{
		model.ReleaseStatusSucceeded, model.ReleaseStatusFailed, model.ReleaseStatusRolledBack,
	}).Count(&active).Error; err != nil {
		return nil, nil, err
	}
	if active > 0 {
		return nil, nil, model.ErrApplicationTransferBusy
	}
	var existing int64
	if err := s.db.Model(&model.Application{}).Where("environment_id = ? AND name = ?", target.ID, app.Name).Count(&existing).Error; err != nil {
		return nil, nil, err
	}
	if existing > 0 {
		return nil, nil, model.ErrApplicationNamespaceMoveChanged
	}
	return app, target, nil
}

func (s *Store) AdoptApplicationNamespace(move model.ApplicationNamespaceMove) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		app, target, err := (&Store{db: tx}).checkApplicationNamespaceMove(move)
		if err != nil || app.EnvironmentID == target.ID {
			return err
		}
		result := tx.Model(&model.Application{}).Where("id = ? AND environment_id = ? AND "+applicationOwningProject+" = ?", app.ID, move.SourceEnvironmentID, target.ProjectID).
			Updates(map[string]interface{}{"project_id": target.ProjectID, "environment_id": target.ID, "management_project_id": nil})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return model.ErrApplicationNamespaceMoveChanged
		}
		var domains []uint
		if err := tx.Model(&model.ApplicationEndpoint{}).Distinct().Where("application_id = ? AND domain_id <> 0", app.ID).Pluck("domain_id", &domains).Error; err != nil {
			return err
		}
		if len(domains) != len(move.DomainIDs) {
			return model.ErrApplicationNamespaceMoveChanged
		}
		for _, id := range move.DomainIDs {
			if !slices.Contains(domains, id) {
				return model.ErrApplicationNamespaceMoveChanged
			}
			var count int64
			if err := tx.Model(&model.ApplicationEndpoint{}).Where("domain_id = ? AND application_id <> ?", id, app.ID).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return fmt.Errorf("应用入口使用共享域名，需要先迁移其共享路由")
			}
			result := tx.Model(&model.ManagedDomain{}).Where("id = ? AND environment_id = ?", id, app.EnvironmentID).
				Updates(map[string]interface{}{"environment_id": target.ID, "namespace": target.Namespace})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return model.ErrApplicationNamespaceMoveChanged
			}
		}
		return nil
	})
}
