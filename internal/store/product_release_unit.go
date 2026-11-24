package store

import (
	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func productReleaseUnitAccess(tx *gorm.DB, productID, unitID uint) error {
	if unitID == 0 {
		return nil
	}
	var count int64
	if err := tx.Model(&model.ProductReleaseUnit{}).Where("product_id = ? AND id = ?", productID, unitID).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return model.ErrProductResourceAccess
	}
	return nil
}

func (s *Store) SaveProductReleaseUnit(ownerID uint, unit *model.ProductReleaseUnit) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := ownedProduct(tx, ownerID, unit.ProductID); err != nil {
			return err
		}
		if err := productReleaseUnitAccess(tx, unit.ProductID, unit.ID); err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&model.ProductReleaseUnit{}).Where("product_id = ? AND name = ? AND id <> ?", unit.ProductID, unit.Name, unit.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return model.ErrProductReleaseUnitName
		}
		if unit.ID == 0 {
			return tx.Create(unit).Error
		}
		return tx.Model(unit).Where("product_id = ?", unit.ProductID).Update("name", unit.Name).Error
	})
}

func (s *Store) DeleteProductReleaseUnit(ownerID, productID, unitID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := ownedProduct(tx, ownerID, productID); err != nil {
			return err
		}
		if err := productReleaseUnitAccess(tx, productID, unitID); err != nil {
			return err
		}
		for _, table := range []any{&model.ProductVersion{}, &model.ProductChannel{}} {
			var count int64
			if err := tx.Model(table).Where("product_id = ? AND release_unit_id = ?", productID, unitID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return model.ErrProductReleaseUnitInUse
			}
		}
		return tx.Where("product_id = ? AND id = ?", productID, unitID).Delete(&model.ProductReleaseUnit{}).Error
	})
}
