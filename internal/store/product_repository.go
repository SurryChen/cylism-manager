package store

import (
	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrProductResourceAccess = model.ErrProductResourceAccess

func (s *Store) ListProducts(ownerID uint) ([]model.Product, error) {
	products := []model.Product{}
	err := s.db.Where("owner_id = ?", ownerID).Preload("Channels", func(db *gorm.DB) *gorm.DB { return db.Order("id asc") }).Order("updated_at desc, id desc").Find(&products).Error
	return products, err
}

func (s *Store) GetProduct(ownerID, id uint) (*model.Product, error) {
	var product model.Product
	if err := s.db.Where("owner_id = ?", ownerID).Preload("Channels", func(db *gorm.DB) *gorm.DB { return db.Order("id asc") }).First(&product, id).Error; err != nil {
		return nil, err
	}
	product.Projects = []model.Project{}
	product.Applications = []model.Application{}
	if err := s.db.Joins("JOIN product_projects ON product_projects.project_id = projects.id").Where("product_projects.product_id = ? AND projects.owner_id = ?", id, ownerID).Preload("Environments").Order("projects.id").Find(&product.Projects).Error; err != nil {
		return nil, err
	}
	if err := s.db.Joins("JOIN product_applications ON product_applications.application_id = applications.id").Joins("JOIN projects ON projects.id = applications.project_id").Where("product_applications.product_id = ? AND projects.owner_id = ?", id, ownerID).Preload("Environment").Preload("Endpoints").Order("applications.id").Find(&product.Applications).Error; err != nil {
		return nil, err
	}
	product.RelatedApplications = append([]model.Application(nil), product.Applications...)
	var projectApps []model.Application
	if err := s.db.Joins("JOIN product_projects ON product_projects.project_id = applications.project_id").Joins("JOIN projects ON projects.id = applications.project_id").Where("product_projects.product_id = ? AND projects.owner_id = ?", id, ownerID).Preload("Environment").Preload("Endpoints").Order("applications.id").Find(&projectApps).Error; err != nil {
		return nil, err
	}
	seen := make(map[uint]bool, len(product.RelatedApplications))
	for _, app := range product.RelatedApplications {
		seen[app.ID] = true
	}
	for _, app := range projectApps {
		if !seen[app.ID] {
			product.RelatedApplications = append(product.RelatedApplications, app)
		}
	}
	product.DatabaseResources = []model.ProductDatabaseResource{}
	appIDs := make([]uint, 0, len(product.RelatedApplications))
	for _, app := range product.RelatedApplications {
		appIDs = append(appIDs, app.ID)
	}
	if len(appIDs) > 0 {
		if err := s.db.Table("database_bindings").Select("DISTINCT database_instances.id AS instance_id, database_instances.name, database_bindings.application_id").Joins("JOIN database_instances ON database_instances.id = database_bindings.instance_id").Where("database_bindings.application_id IN ?", appIDs).Order("database_instances.name").Scan(&product.DatabaseResources).Error; err != nil {
			return nil, err
		}
	}
	return &product, nil
}

// ProductResourceOptions is limited to owned projects and their existing components.
func (s *Store) ProductResourceOptions(ownerID uint) ([]model.Project, []model.Application, error) {
	projects := []model.Project{}
	apps := []model.Application{}
	if err := s.db.Where("owner_id = ?", ownerID).Order("name").Find(&projects).Error; err != nil {
		return nil, nil, err
	}
	err := s.db.Joins("JOIN projects ON projects.id = applications.project_id").Where("projects.owner_id = ?", ownerID).Order("applications.name").Find(&apps).Error
	return projects, apps, err
}

// SaveProduct atomically replaces editable metadata, channels and lightweight links.
// It never mutates the linked deployment records, even when archiving a product.
func (s *Store) SaveProduct(product *model.Product, projectIDs, applicationIDs []uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if product.OwnerID == 0 {
			return ErrProductResourceAccess
		}
		if product.ID != 0 {
			var current model.Product
			if err := tx.Where("owner_id = ?", product.OwnerID).First(&current, product.ID).Error; err != nil {
				return err
			}
			product.CreatedAt = current.CreatedAt
		}
		if err := validateProductResources(tx, product.OwnerID, projectIDs, applicationIDs); err != nil {
			return err
		}
		seen := map[uint]bool{}
		for _, channel := range product.Channels {
			if channel.ID == 0 {
				continue
			}
			if seen[channel.ID] || product.ID == 0 {
				return ErrProductResourceAccess
			}
			seen[channel.ID] = true
			var count int64
			if err := tx.Model(&model.ProductChannel{}).Where("id = ? AND product_id = ?", channel.ID, product.ID).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return ErrProductResourceAccess
			}
		}
		if err := tx.Omit(clause.Associations).Save(product).Error; err != nil {
			return err
		}
		keep := []uint{0}
		for i := range product.Channels {
			channel := &product.Channels[i]
			channel.ProductID = product.ID
			if channel.ID != 0 {
				var current model.ProductChannel
				if err := tx.First(&current, channel.ID).Error; err != nil {
					return err
				}
				channel.CreatedAt = current.CreatedAt
			}
			if err := tx.Save(channel).Error; err != nil {
				return err
			}
			keep = append(keep, channel.ID)
		}
		if err := tx.Where("product_id = ? AND id NOT IN ?", product.ID, keep).Delete(&model.ProductChannel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("product_id = ?", product.ID).Delete(&model.ProductProject{}).Error; err != nil {
			return err
		}
		if err := tx.Where("product_id = ?", product.ID).Delete(&model.ProductApplication{}).Error; err != nil {
			return err
		}
		for _, id := range projectIDs {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.ProductProject{ProductID: product.ID, ProjectID: id}).Error; err != nil {
				return err
			}
		}
		for _, id := range applicationIDs {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.ProductApplication{ProductID: product.ID, ApplicationID: id}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func validateProductResources(tx *gorm.DB, ownerID uint, projectIDs, appIDs []uint) error {
	for _, id := range projectIDs {
		var count int64
		if err := tx.Model(&model.Project{}).Where("id = ? AND owner_id = ?", id, ownerID).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return ErrProductResourceAccess
		}
	}
	for _, id := range appIDs {
		var count int64
		if err := tx.Model(&model.Application{}).Joins("JOIN projects ON projects.id = applications.project_id").Where("applications.id = ? AND projects.owner_id = ?", id, ownerID).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return ErrProductResourceAccess
		}
	}
	return nil
}
