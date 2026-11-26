package store

import (
	"errors"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func (s *Store) ProductTaskGroups(ownerID, productID uint) ([]model.ProductTaskGroup, error) {
	if err := ownedProduct(s.db, ownerID, productID); err != nil {
		return nil, err
	}
	groups := []model.ProductTaskGroup{}
	err := s.db.Where("product_id = ?", productID).Order("id").Find(&groups).Error
	return groups, err
}

func productTaskGroupAccess(tx *gorm.DB, productID, groupID uint) error {
	if groupID == 0 {
		return nil
	}
	err := tx.Select("id").Where("product_id = ? AND id = ?", productID, groupID).Take(&model.ProductTaskGroup{}).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.ErrProductResourceAccess
	}
	return err
}

func (s *Store) SaveProductTaskGroup(ownerID uint, group *model.ProductTaskGroup) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := ownedProduct(tx, ownerID, group.ProductID); err != nil {
			return err
		}
		if err := productTaskGroupAccess(tx, group.ProductID, group.ID); err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&model.ProductTaskGroup{}).Where("product_id = ? AND name = ? AND id <> ?", group.ProductID, group.Name, group.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return model.ErrProductTaskGroupName
		}
		if group.ID == 0 {
			return tx.Create(group).Error
		}
		return tx.Model(group).Where("product_id = ?", group.ProductID).Update("name", group.Name).Error
	})
}

func (s *Store) DeleteProductTaskGroup(ownerID, productID, groupID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := ownedProduct(tx, ownerID, productID); err != nil {
			return err
		}
		result := tx.Where("product_id = ? AND id = ?", productID, groupID).Delete(&model.ProductTaskGroup{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return tx.Model(&model.ProductTask{}).Where("product_id = ? AND group_id = ?", productID, groupID).
			Updates(map[string]any{"group_id": 0, "revision": gorm.Expr("revision + 1")}).Error
	})
}
