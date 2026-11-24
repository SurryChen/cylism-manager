package store

import (
	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

const applicationOwningProject = "COALESCE(applications.management_project_id, applications.project_id)"

func applicationTransferProjects(db *gorm.DB, ownerID uint) *gorm.DB {
	return db.Model(&model.Project{}).Joins("JOIN products ON products.project_id = projects.id").
		Where("projects.owner_id = ? AND products.owner_id = ? AND products.stage <> ?", ownerID, ownerID, model.ProductStageArchived)
}

func (s *Store) ListApplicationTransferProjects(ownerID uint) ([]model.Project, error) {
	projects := []model.Project{}
	err := applicationTransferProjects(s.db, ownerID).Order("projects.name, projects.id").Find(&projects).Error
	return projects, err
}

func (s *Store) TransferApplicationProject(move model.ApplicationProjectTransfer) error {
	if move.OwnerID == 0 {
		return gorm.ErrRecordNotFound
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var app model.Application
		if err := tx.Joins("JOIN projects ON projects.id = applications.project_id").
			Where("applications.id = ? AND projects.owner_id = ?", move.ApplicationID, move.OwnerID).First(&app).Error; err != nil {
			return err
		}
		owningProjectID := app.OwningProjectID()
		var owned int64
		if err := tx.Model(&model.Project{}).Where("id = ? AND owner_id = ?", owningProjectID, move.OwnerID).Count(&owned).Error; err != nil {
			return err
		}
		if owned == 0 {
			return gorm.ErrRecordNotFound
		}
		if owningProjectID == move.ToProjectID {
			return nil
		}
		if owningProjectID != move.FromProjectID {
			return model.ErrApplicationProjectChanged
		}
		var targets int64
		if err := applicationTransferProjects(tx, move.OwnerID).Where("projects.id = ?", move.ToProjectID).Count(&targets).Error; err != nil {
			return err
		}
		if targets == 0 {
			return model.ErrApplicationTransferTarget
		}
		var active int64
		if err := tx.Model(&model.Release{}).Where("application_id = ? AND status IN ?", app.ID, []string{
			model.ReleaseStatusDraft, model.ReleaseStatusValidating, model.ReleaseStatusMigrating, model.ReleaseStatusApplying, model.ReleaseStatusWaitingReady,
			model.ReleaseStatusVerifying, model.ReleaseStatusRollingBack,
		}).Count(&active).Error; err != nil {
			return err
		}
		if active != 0 {
			return model.ErrApplicationTransferBusy
		}
		var target *uint
		if move.ToProjectID != app.ProjectID {
			target = &move.ToProjectID
		}
		result := tx.Model(&model.Application{}).Where("id = ? AND "+applicationOwningProject+" = ?", app.ID, owningProjectID).
			Update("management_project_id", target)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return model.ErrApplicationProjectChanged
		}
		return nil
	})
}
