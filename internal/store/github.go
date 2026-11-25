package store

import (
	"errors"
	"slices"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Store) GitHubConnection(ownerID uint) (model.GitHubConnection, error) {
	var connection model.GitHubConnection
	err := s.db.Where("owner_id = ?", ownerID).Take(&connection).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = model.ErrGitHubNotConfigured
	}
	return connection, err
}

func (s *Store) StartGitHubRegistration(registration *model.GitHubRegistration) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		_, err := (&Store{db: tx}).GitHubConnection(registration.OwnerID)
		if err == nil {
			return model.ErrGitHubAlreadyConnected
		}
		if !errors.Is(err, model.ErrGitHubNotConfigured) {
			return err
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "owner_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"state_hash", "browser_hash", "organization", "expires_at", "claimed"}),
		}).Create(registration).Error
	})
}

func (s *Store) ClaimGitHubRegistration(stateHash, browserHash string, now time.Time) (model.GitHubRegistration, error) {
	var registration model.GitHubRegistration
	result := s.db.Model(&registration).Clauses(clause.Returning{}).
		Where("state_hash = ? AND browser_hash = ? AND expires_at > ? AND claimed = ?", stateHash, browserHash, now, false).
		UpdateColumn("claimed", true)
	if result.Error == nil && result.RowsAffected != 1 {
		return registration, model.ErrGitHubRegistrationExpired
	}
	return registration, result.Error
}

func (s *Store) FinishGitHubRegistration(registration model.GitHubRegistration, connection *model.GitHubConnection) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("owner_id = ? AND state_hash = ? AND claimed = ?", registration.OwnerID, registration.StateHash, true).
			Delete(&model.GitHubRegistration{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return model.ErrGitHubRegistrationExpired
		}
		connection.OwnerID = registration.OwnerID
		result = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(connection)
		if result.Error == nil && result.RowsAffected != 1 {
			return model.ErrGitHubAlreadyConnected
		}
		return result.Error
	})
}

func (s *Store) DeleteGitHubConnection(ownerID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("owner_id = ?", ownerID).Delete(&model.GitHubRegistration{}).Error; err != nil {
			return err
		}
		return tx.Where("owner_id = ?", ownerID).Delete(&model.GitHubConnection{}).Error
	})
}

func (s *Store) ListProductGitHubRepositories(ownerID, productID uint) ([]model.ProductGitHubRepository, error) {
	if err := ownedProduct(s.db, ownerID, productID); err != nil {
		return nil, err
	}
	items := []model.ProductGitHubRepository{}
	err := s.db.Where("product_id = ?", productID).Order("release_unit_id, component, id").Find(&items).Error
	return items, err
}

func (s *Store) ProductGitHubRepository(ownerID, productID, id uint) (model.ProductGitHubRepository, error) {
	var binding model.ProductGitHubRepository
	if err := ownedProduct(s.db, ownerID, productID); err != nil {
		return binding, err
	}
	err := s.db.Where("product_id = ? AND id = ?", productID, id).Take(&binding).Error
	return binding, err
}

func (s *Store) SaveProductGitHubRepository(ownerID uint, binding *model.ProductGitHubRepository) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := ownedProduct(tx, ownerID, binding.ProductID); err != nil {
			return err
		}
		if err := productReleaseUnitAccess(tx, binding.ProductID, binding.ReleaseUnitID); err != nil {
			return err
		}
		if !binding.IsPublic() {
			connection, err := (&Store{db: tx}).GitHubConnection(ownerID)
			if err != nil {
				return err
			}
			if connection.AppID != binding.AppID {
				return model.ErrProductLifecycleConflict
			}
		}
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "product_id"}, {Name: "release_unit_id"}, {Name: "component"}},
			DoNothing: true,
		}).Create(binding)
		if result.Error == nil && result.RowsAffected == 0 {
			return model.ErrGitHubComponentExists
		}
		return result.Error
	})
}

func (s *Store) DeleteProductGitHubRepository(ownerID, productID, id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := ownedProduct(tx, ownerID, productID); err != nil {
			return err
		}
		result := tx.Where("product_id = ? AND id = ?", productID, id).Delete(&model.ProductGitHubRepository{})
		if result.Error == nil && result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return result.Error
	})
}

func (s *Store) ProductVersionGitHubSources(ownerID, productID, versionID uint) (model.ProductVersion, error) {
	var version model.ProductVersion
	if err := ownedProduct(s.db, ownerID, productID); err != nil {
		return version, err
	}
	err := s.db.Select("release_unit_id", "github_sources", "revision").
		Where("product_id = ? AND id = ?", productID, versionID).Take(&version).Error
	return version, err
}

func validateVersionGitHubSources(tx *gorm.DB, ownerID uint, version, current *model.ProductVersion) error {
	if version.GitHubSources == nil {
		version.GitHubSources = current.GitHubSources
	}
	for _, source := range version.GitHubSources {
		if current.ReleaseUnitID == version.ReleaseUnitID && slices.Contains(current.GitHubSources, source) {
			continue
		}
		var count int64
		err := tx.Model(&model.ProductGitHubRepository{}).
			Where("id = ? AND product_id = ? AND release_unit_id = ? AND repository_id = ? AND component = ?", source.BindingID, version.ProductID, version.ReleaseUnitID, source.RepositoryID, source.Component).
			Where("(app_id = 0 AND installation_id = 0) OR EXISTS (SELECT 1 FROM github_connections WHERE owner_id = ? AND app_id = product_github_repositories.app_id)", ownerID).
			Count(&count).Error
		if err != nil {
			return err
		}
		if count != 1 {
			return model.ErrProductLifecycleConflict
		}
	}
	return nil
}
