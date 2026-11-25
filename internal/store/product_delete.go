package store

import (
	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

// DeleteProduct removes product-owned data and links, never the linked resources.
func (s *Store) DeleteProduct(ownerID, productID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if ownerID == 0 {
			return gorm.ErrRecordNotFound
		}
		if err := ownedProduct(tx, ownerID, productID); err != nil {
			return err
		}
		_, err := deleteProducts(tx, ownerID, []uint{productID})
		return err
	})
}

// ClearProducts includes archived products, scoped to the authenticated owner.
func (s *Store) ClearProducts(ownerID uint) (int64, error) {
	var deleted int64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if ownerID == 0 {
			return gorm.ErrRecordNotFound
		}
		var ids []uint
		if err := tx.Model(&model.Product{}).Where("owner_id = ?", ownerID).Pluck("id", &ids).Error; err != nil {
			return err
		}
		var err error
		deleted, err = deleteProducts(tx, ownerID, ids)
		return err
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

func deleteProducts(tx *gorm.DB, ownerID uint, ids []uint) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	distributions := tx.Model(&model.ProductDistribution{}).Select("id").Where("product_id IN ?", ids)
	if err := tx.Where("distribution_id IN (?)", distributions).Delete(&model.ProductDistributionEvent{}).Error; err != nil {
		return 0, err
	}
	for _, link := range []interface{}{&model.ProductVersionTask{}, &model.ProductVersionDeployment{}, &model.ProductVersionPlatformRelease{}} {
		versions := tx.Model(&model.ProductVersion{}).Select("id").Where("product_id IN ?", ids)
		if err := tx.Where("version_id IN (?)", versions).Delete(link).Error; err != nil {
			return 0, err
		}
	}
	for _, record := range []interface{}{&model.ProductTask{}, &model.ProductDistribution{}, &model.ProductVersion{}, &model.ProductChannel{}, &model.ProductGitHubRepository{}, &model.ProductReleaseUnit{}, &model.ProductProject{}, &model.ProductApplication{}} {
		if err := tx.Where("product_id IN ?", ids).Delete(record).Error; err != nil {
			return 0, err
		}
	}
	result := tx.Where("id IN ? AND owner_id = ?", ids, ownerID).Delete(&model.Product{})
	return result.RowsAffected, result.Error
}
