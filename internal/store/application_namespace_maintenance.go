package store

import (
	"errors"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func (s *Store) GetApplicationNamespaceMaintenance(applicationID uint) (*model.ApplicationNamespaceMaintenance, error) {
	var move model.ApplicationNamespaceMaintenance
	err := s.db.First(&move, "application_id = ?", applicationID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &move, err
}

func (s *Store) BeginApplicationNamespaceMaintenance(move model.ApplicationNamespaceMove, plan string) (*model.ApplicationNamespaceMaintenance, error) {
	task := model.ApplicationNamespaceMaintenance{
		ApplicationID: move.ApplicationID, SourceEnvironmentID: move.SourceEnvironmentID,
		TargetEnvironmentID: move.TargetEnvironmentID, ReleaseID: move.ReleaseID, OwnerID: move.OwnerID,
		Phase: model.NamespaceMovePreparing, Plan: plan,
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		app, target, err := (&Store{db: tx}).checkApplicationNamespaceMove(move)
		if err != nil {
			return err
		}
		if app.EnvironmentID == target.ID {
			return model.ErrApplicationNamespaceMoveChanged
		}
		return tx.Create(&task).Error
	})
	return &task, err
}

func (s *Store) UpdateApplicationNamespaceMaintenance(task *model.ApplicationNamespaceMaintenance, phase, detail string) error {
	result := s.db.Model(&model.ApplicationNamespaceMaintenance{}).Where("id = ? AND phase = ?", task.ID, task.Phase).Updates(map[string]interface{}{"phase": phase, "detail": detail})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return model.ErrApplicationNamespaceMoveChanged
	}
	task.Phase, task.Detail = phase, detail
	return nil
}

func (s *Store) CancelApplicationNamespaceMaintenance(task *model.ApplicationNamespaceMaintenance) error {
	result := s.db.Where("phase = ?", task.Phase).Delete(task)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return model.ErrApplicationNamespaceMoveChanged
	}
	return nil
}

func ensureNoNamespaceMaintenance(tx *gorm.DB, applicationID uint) error {
	var active int64
	if err := tx.Model(&model.ApplicationNamespaceMaintenance{}).Where("application_id = ?", applicationID).Count(&active).Error; err != nil {
		return err
	}
	if active != 0 {
		return model.ErrApplicationNamespaceMaintenanceActive
	}
	return nil
}

func ensureNoEnvironmentNamespaceMaintenance(tx *gorm.DB, environmentID uint) error {
	var active int64
	if err := tx.Model(&model.ApplicationNamespaceMaintenance{}).
		Where("source_environment_id = ? OR target_environment_id = ?", environmentID, environmentID).Count(&active).Error; err != nil {
		return err
	}
	if active != 0 {
		return model.ErrApplicationNamespaceMaintenanceActive
	}
	return nil
}
