package store

import (
	"errors"
	"fmt"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// attachProductProject preserves lifecycle IDs and never moves existing environments.
func attachProductProject(tx *gorm.DB, product *model.Product, candidates []uint) error {
	if product.ProjectID == nil && len(candidates) == 1 {
		var project model.Project
		if err := tx.Where("owner_id = ?", product.OwnerID).First(&project, candidates[0]).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&model.Product{}).Where("project_id = ?", project.ID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			product.ProjectID = &project.ID
		}
	}
	if product.ProjectID == nil {
		project := model.Project{OwnerID: product.OwnerID, Name: product.Name, Description: product.Summary}
		for suffix := 0; ; suffix++ {
			var count int64
			if err := tx.Model(&model.Project{}).Where("name = ?", project.Name).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				break
			}
			project.Name = fmt.Sprintf("%.90s · %d-%d", product.Name, product.OwnerID, suffix+1)
		}
		if err := tx.Create(&project).Error; err != nil {
			return err
		}
		// Numeric IDs provide a stable, valid namespace prefix without requiring a cluster.
		for suffix := 0; ; suffix++ {
			project.Slug = fmt.Sprintf("p-%d", project.ID)
			if suffix > 0 {
				project.Slug = fmt.Sprintf("p-%d-%d", project.ID, suffix)
			}
			var count int64
			if err := tx.Model(&model.Project{}).Where("slug = ?", project.Slug).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				break
			}
		}
		if err := tx.Model(&project).Update("slug", project.Slug).Error; err != nil {
			return err
		}
		product.ProjectID = &project.ID
	}
	var project model.Project
	if err := tx.Where("owner_id = ?", product.OwnerID).First(&project, *product.ProjectID).Error; err != nil {
		return err
	}
	if product.ProjectSlug != "" && product.ProjectSlug != project.Slug {
		var count int64
		if err := tx.Model(&model.Environment{}).Where("project_id = ?", project.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return model.ErrProjectWorkspaceConflict
		}
		if err := tx.Model(&project).Update("slug", product.ProjectSlug).Error; err != nil {
			return model.ErrProjectIdentityConflict
		}
		project.Slug = product.ProjectSlug
	}
	updateSummary := product.Summary != ""
	if product.ID != 0 {
		var current model.Product
		if err := tx.First(&current, product.ID).Error; err != nil {
			return err
		}
		updateSummary = current.Summary != product.Summary
		if current.Name != product.Name {
			if err := tx.Model(&project).Update("name", product.Name).Error; err != nil {
				return model.ErrProjectIdentityConflict
			}
		}
	}
	product.ProjectSlug = project.Slug
	if updateSummary {
		return tx.Model(&project).Update("description", product.Summary).Error
	}
	return nil
}

func createProjectMetadata(tx *gorm.DB, project *model.Project) error {
	if project.OwnerID == 0 {
		return nil
	}
	product := model.Product{ProjectID: &project.ID, OwnerID: project.OwnerID, Name: project.Name, Summary: project.Description, Stage: model.ProductStageIdea}
	if err := tx.Create(&product).Error; err != nil {
		return err
	}
	return tx.Create(&model.ProductProject{ProductID: product.ID, ProjectID: project.ID}).Error
}

func (s *Store) backfillProjectWorkspaces() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var marker model.SystemConfig
		err := tx.Where("key = ?", "migration.project_workspaces.v1").First(&marker).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var products []model.Product
		if err := tx.Order("id").Find(&products).Error; err != nil {
			return err
		}
		for i := range products {
			p := &products[i]
			var candidates []uint
			// Only exclusively associated, owned projects are eligible for automatic adoption.
			if err := tx.Table("product_projects").Joins("JOIN projects ON projects.id = product_projects.project_id").Where("product_id = ? AND projects.owner_id = ? AND (SELECT COUNT(*) FROM product_projects pp WHERE pp.project_id = projects.id) = 1", p.ID, p.OwnerID).Pluck("projects.id", &candidates).Error; err != nil {
				return err
			}
			if err := attachProductProject(tx, p, candidates); err != nil {
				return err
			}
			if err := tx.Model(p).Update("project_id", p.ProjectID).Error; err != nil {
				return err
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.ProductProject{ProductID: p.ID, ProjectID: *p.ProjectID}).Error; err != nil {
				return err
			}
		}
		var projects []model.Project
		if err := tx.Where("owner_id <> 0 AND id NOT IN (SELECT project_id FROM products WHERE project_id IS NOT NULL)").Find(&projects).Error; err != nil {
			return err
		}
		for i := range projects {
			if err := createProjectMetadata(tx, &projects[i]); err != nil {
				return err
			}
		}
		// Old distribution summaries had no audience; preserve history and await classification.
		if err := tx.Model(&model.ProductChannel{}).Where("id IN (SELECT channel_id FROM product_distributions)").Updates(map[string]interface{}{"online_version": "", "pending_version": "", "publish_status": gorm.Expr("CASE WHEN publish_status = 'paused' THEN 'paused' ELSE 'draft' END")}).Error; err != nil {
			return err
		}
		return tx.Create(&model.SystemConfig{Key: "migration.project_workspaces.v1", Value: "completed"}).Error
	})
}

func (s *Store) GetProjectWorkspace(ownerID, projectID uint) (*model.Product, error) {
	var product model.Product
	if err := s.db.Where("owner_id = ? AND project_id = ?", ownerID, projectID).First(&product).Error; err != nil {
		return nil, err
	}
	return s.GetProduct(ownerID, product.ID)
}

func (s *Store) DeleteProjectWorkspace(ownerID, projectID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var project model.Project
		if err := tx.Where("owner_id = ?", ownerID).First(&project, projectID).Error; err != nil {
			return err
		}
		return (&Store{db: tx}).DeleteProject(projectID)
	})
}
