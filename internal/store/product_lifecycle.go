package store

import (
	"errors"
	"strings"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func ownedProduct(db *gorm.DB, ownerID, productID uint) error {
	var product model.Product
	return db.Select("id").Where("id = ? AND owner_id = ?", productID, ownerID).First(&product).Error
}

func lifecycleQuery(db *gorm.DB, table interface{}, productID uint, f model.ProductLifecycleFilter) *gorm.DB {
	q := db.Model(table).Where("product_id = ?", productID)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.VersionID != 0 {
		q = q.Where("version_id = ?", f.VersionID)
	}
	return q
}

func (s *Store) ListProductTasks(ownerID, productID uint, f model.ProductLifecycleFilter) ([]model.ProductTask, int64, error) {
	if err := ownedProduct(s.db, ownerID, productID); err != nil {
		return nil, 0, err
	}
	q := lifecycleQuery(s.db, &model.ProductTask{}, productID, f)
	if f.Keyword != "" {
		q = q.Where("instr(LOWER(title), ?) > 0", strings.ToLower(f.Keyword))
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.ProductTask{}
	err := q.Order("CASE status WHEN 'blocked' THEN 0 WHEN 'in_progress' THEN 1 WHEN 'todo' THEN 2 ELSE 3 END").Order("CASE WHEN due_date = '' THEN 1 ELSE 0 END, due_date, id DESC").Limit(f.Limit).Offset(f.Offset).Find(&rows).Error
	return rows, count, err
}

func (s *Store) ListProductVersions(ownerID, productID uint, f model.ProductLifecycleFilter) ([]model.ProductVersion, int64, error) {
	if err := ownedProduct(s.db, ownerID, productID); err != nil {
		return nil, 0, err
	}
	q := lifecycleQuery(s.db, &model.ProductVersion{}, productID, f)
	if f.Keyword != "" {
		q = q.Where("instr(LOWER(name), ?) > 0", strings.ToLower(f.Keyword))
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.ProductVersion{}
	if err := q.Order("id DESC").Limit(f.Limit).Offset(f.Offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	for i := range rows {
		rows[i].DeploymentIDs = []uint{}
		rows[i].PlatformReleaseIDs = []uint{}
		err := s.productDeployments(ownerID, productID).Joins("JOIN product_version_deployments ON product_version_deployments.release_id = releases.id").Where("product_version_deployments.version_id = ?", rows[i].ID).Order("releases.id DESC").Pluck("releases.id", &rows[i].DeploymentIDs).Error
		if err != nil {
			return nil, 0, err
		}
		if err := s.productPlatformReleases(ownerID, productID).Joins("JOIN product_version_platform_releases ON product_version_platform_releases.release_id = platform_releases.id").Where("product_version_platform_releases.version_id = ?", rows[i].ID).Order("platform_releases.id DESC").Pluck("platform_releases.id", &rows[i].PlatformReleaseIDs).Error; err != nil {
			return nil, 0, err
		}
	}
	return rows, count, nil
}

func (s *Store) ListProductDistributions(ownerID, productID uint, f model.ProductLifecycleFilter) ([]model.ProductDistribution, int64, error) {
	if err := ownedProduct(s.db, ownerID, productID); err != nil {
		return nil, 0, err
	}
	q := lifecycleQuery(s.db, &model.ProductDistribution{}, productID, f)
	if f.Keyword != "" {
		q = q.Where("instr(LOWER(platform_version), ?) > 0 OR instr(LOWER(build_identifier), ?) > 0", strings.ToLower(f.Keyword), strings.ToLower(f.Keyword))
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.ProductDistribution{}
	err := q.Order("id DESC").Limit(f.Limit).Offset(f.Offset).Find(&rows).Error
	return rows, count, err
}

func lifecycleVersionAccess(tx *gorm.DB, productID uint, versionID *uint) error {
	if versionID == nil {
		return nil
	}
	var count int64
	if err := tx.Model(&model.ProductVersion{}).Where("id = ? AND product_id = ?", *versionID, productID).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return model.ErrProductResourceAccess
	}
	return nil
}

func (s *Store) SaveProductTask(ownerID uint, task *model.ProductTask) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := ownedProduct(tx, ownerID, task.ProductID); err != nil {
			return err
		}
		if err := lifecycleVersionAccess(tx, task.ProductID, task.VersionID); err != nil {
			return err
		}
		if task.ID == 0 {
			task.Revision = 1
			return tx.Create(task).Error
		}
		var current model.ProductTask
		if err := tx.Where("id = ? AND product_id = ?", task.ID, task.ProductID).First(&current).Error; err != nil {
			return err
		}
		if current.Revision != task.Revision {
			return model.ErrProductLifecycleConflict
		}
		task.Revision++
		task.CreatedAt = current.CreatedAt
		result := tx.Model(&model.ProductTask{}).Where("id = ? AND revision = ?", task.ID, current.Revision).Select("*").Omit("id", "created_at").Updates(task)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return model.ErrProductLifecycleConflict
		}
		return nil
	})
}

func (s *Store) SaveProductVersion(ownerID uint, version *model.ProductVersion) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := ownedProduct(tx, ownerID, version.ProductID); err != nil {
			return err
		}
		var duplicate int64
		if err := tx.Model(&model.ProductVersion{}).Where("product_id = ? AND name = ? AND id <> ?", version.ProductID, version.Name, version.ID).Count(&duplicate).Error; err != nil {
			return err
		}
		if duplicate != 0 {
			return model.ErrProductVersionName
		}
		ids := map[uint]bool{}
		for _, id := range version.DeploymentIDs {
			var count int64
			if err := (&Store{db: tx}).productDeployments(ownerID, version.ProductID).Where("releases.id = ?", id).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return model.ErrProductResourceAccess
			}
			ids[id] = true
		}
		platformIDs := map[uint]bool{}
		for _, id := range version.PlatformReleaseIDs {
			var count int64
			if err := (&Store{db: tx}).productPlatformReleases(ownerID, version.ProductID).Where("platform_releases.id = ?", id).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return model.ErrProductResourceAccess
			}
			platformIDs[id] = true
		}
		if version.ID == 0 {
			version.Revision = 1
			if err := tx.Create(version).Error; err != nil {
				return err
			}
		} else {
			var current model.ProductVersion
			if err := tx.Where("id = ? AND product_id = ?", version.ID, version.ProductID).First(&current).Error; err != nil {
				return err
			}
			if current.Revision != version.Revision {
				return model.ErrProductLifecycleConflict
			}
			version.Revision++
			version.CreatedAt = current.CreatedAt
			result := tx.Model(&model.ProductVersion{}).Where("id = ? AND revision = ?", version.ID, current.Revision).Select("*").Omit("id", "created_at").Updates(version)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return model.ErrProductLifecycleConflict
			}
		}
		if err := tx.Where("version_id = ?", version.ID).Delete(&model.ProductVersionDeployment{}).Error; err != nil {
			return err
		}
		for id := range ids {
			if err := tx.Create(&model.ProductVersionDeployment{VersionID: version.ID, ReleaseID: id}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("version_id = ?", version.ID).Delete(&model.ProductVersionPlatformRelease{}).Error; err != nil {
			return err
		}
		for id := range platformIDs {
			if err := tx.Create(&model.ProductVersionPlatformRelease{VersionID: version.ID, ReleaseID: id}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) SaveProductDistribution(ownerID uint, record *model.ProductDistribution) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := ownedProduct(tx, ownerID, record.ProductID); err != nil {
			return err
		}
		if err := lifecycleVersionAccess(tx, record.ProductID, &record.VersionID); err != nil {
			return err
		}
		becamePublished := record.Status == "published"
		var channel model.ProductChannel
		if err := tx.Where("id = ? AND product_id = ?", record.ChannelID, record.ProductID).First(&channel).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.ErrProductResourceAccess
			}
			return err
		}
		if record.ID == 0 {
			record.Revision = 1
			if err := tx.Create(record).Error; err != nil {
				return err
			}
		} else {
			var current model.ProductDistribution
			if err := tx.Where("id = ? AND product_id = ?", record.ID, record.ProductID).First(&current).Error; err != nil {
				return err
			}
			becamePublished = record.Status == "published" && (current.Status != "published" || current.PlatformVersion != record.PlatformVersion)
			// A submission's identity is immutable; resubmission is a new record.
			if current.ChannelID != record.ChannelID || current.VersionID != record.VersionID {
				return model.ErrProductResourceAccess
			}
			if current.Revision != record.Revision {
				return model.ErrProductLifecycleConflict
			}
			record.Revision++
			record.CreatedAt = current.CreatedAt
			result := tx.Model(&model.ProductDistribution{}).Where("id = ? AND revision = ?", record.ID, current.Revision).Select("*").Omit("id", "created_at").Updates(record)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return model.ErrProductLifecycleConflict
			}
		}
		event := model.ProductDistributionEvent{BecamePublished: becamePublished, DistributionID: record.ID, ActorID: ownerID, Status: record.Status, PlatformVersion: record.PlatformVersion, BuildIdentifier: record.BuildIdentifier, Notes: record.Notes}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		return syncProductChannel(tx, &channel)
	})
}

func syncProductChannel(tx *gorm.DB, channel *model.ProductChannel) error {
	var latest model.ProductDistribution
	err := tx.Where("channel_id = ?", channel.ID).Order("id DESC").First(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	// Pausing a channel is an explicit user choice independent of submission status.
	if channel.PublishStatus != "paused" {
		channel.PublishStatus = latest.Status
		if latest.Status == "cancelled" {
			channel.PublishStatus = "draft"
		}
	}
	channel.PendingVersion = latest.PlatformVersion
	if latest.Status == "published" || latest.Status == "cancelled" {
		channel.PendingVersion = ""
	}
	// Latest recorded go-live event is authoritative even while the next submission is pending.
	var published model.ProductDistributionEvent
	err = tx.Joins("JOIN product_distributions ON product_distributions.id = product_distribution_events.distribution_id").Where("product_distributions.channel_id = ? AND product_distribution_events.became_published = ?", channel.ID, true).Order("product_distribution_events.id DESC").Select("product_distribution_events.*").First(&published).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil {
		channel.OnlineVersion = published.PlatformVersion
	}
	return tx.Model(channel).Select("publish_status", "pending_version", "online_version", "updated_at").Updates(channel).Error
}

func (s *Store) ListProductDistributionEvents(ownerID, productID, distributionID uint, f model.ProductLifecycleFilter) ([]model.ProductDistributionEvent, int64, error) {
	if err := ownedProduct(s.db, ownerID, productID); err != nil {
		return nil, 0, err
	}
	var record model.ProductDistribution
	if err := s.db.Where("id = ? AND product_id = ?", distributionID, productID).First(&record).Error; err != nil {
		return nil, 0, err
	}
	q := s.db.Model(&model.ProductDistributionEvent{}).Where("distribution_id = ?", distributionID)
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.ProductDistributionEvent{}
	err := q.Order("id DESC").Limit(f.Limit).Offset(f.Offset).Find(&rows).Error
	return rows, count, err
}

func (s *Store) productDeployments(ownerID, productID uint) *gorm.DB {
	return s.db.Model(&model.Release{}).Joins("JOIN applications ON applications.id = releases.application_id").Joins("JOIN projects ON projects.id = applications.project_id").Where("projects.owner_id = ?", ownerID).
		Where("EXISTS (SELECT 1 FROM product_applications WHERE product_applications.product_id = ? AND product_applications.application_id = applications.id) OR EXISTS (SELECT 1 FROM product_projects WHERE product_projects.product_id = ? AND product_projects.project_id = applications.project_id)", productID, productID)
}

func (s *Store) productPlatformReleases(ownerID, productID uint) *gorm.DB {
	return s.db.Model(&model.PlatformRelease{}).Where("EXISTS (SELECT 1 FROM products WHERE products.id = ? AND products.owner_id = ? AND products.platform_linked = ?)", productID, ownerID, true)
}

func (s *Store) ProductDeploymentPage(ownerID, productID uint, f model.ProductLifecycleFilter) ([]model.ProductDeploymentOption, int64, error) {
	if err := ownedProduct(s.db, ownerID, productID); err != nil {
		return nil, 0, err
	}
	q := s.productDeployments(ownerID, productID)
	platform := s.productPlatformReleases(ownerID, productID)
	if f.VersionID != 0 {
		if err := lifecycleVersionAccess(s.db, productID, &f.VersionID); err != nil {
			return nil, 0, err
		}
		q = q.Joins("JOIN product_version_deployments ON product_version_deployments.release_id = releases.id").Where("product_version_deployments.version_id = ?", f.VersionID)
		platform = platform.Joins("JOIN product_version_platform_releases ON product_version_platform_releases.release_id = platform_releases.id").Where("product_version_platform_releases.version_id = ?", f.VersionID)
	}
	if f.Keyword != "" {
		q = q.Where("instr(LOWER(applications.name || ' ' || releases.version), ?) > 0", strings.ToLower(f.Keyword))
		platform = platform.Where("instr(LOWER('Cylism Manager 当前管理平台 ' || platform_releases.image), ?) > 0", strings.ToLower(f.Keyword))
	}
	if f.Status != "" {
		q = q.Where("releases.status = ?", f.Status)
		platform = platform.Where("platform_releases.status = ?", f.Status)
	}
	applications := q.Joins("LEFT JOIN environments ON environments.id = applications.environment_id").Select("'application' AS source, releases.id, releases.application_id, applications.name AS application_name, environments.name AS environment_name, releases.version, releases.status, releases.created_at")
	updates := platform.Select("'platform' AS source, platform_releases.id, 0 AS application_id, 'Cylism Manager' AS application_name, '当前管理平台' AS environment_name, platform_releases.image AS version, platform_releases.status, platform_releases.created_at")
	combined := s.db.Table("(? UNION ALL ?) AS deployments", applications, updates)
	var count int64
	if err := combined.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.ProductDeploymentOption{}
	err := combined.Select("*").Order("created_at DESC, id DESC, source").Limit(f.Limit).Offset(f.Offset).Scan(&rows).Error
	return rows, count, err
}

func (s *Store) ProductVersionOptions(ownerID, productID uint) ([]model.ProductVersion, error) {
	if err := ownedProduct(s.db, ownerID, productID); err != nil {
		return nil, err
	}
	rows := []model.ProductVersion{}
	err := s.db.Select("id", "name", "status").Where("product_id = ?", productID).Order("id DESC").Find(&rows).Error
	return rows, err
}

func (s *Store) ProductWorkbench(ownerID uint) (*model.ProductWorkbench, error) {
	result := &model.ProductWorkbench{Tasks: []model.ProductTask{}, Versions: []model.ProductVersion{}, Distributions: []model.ProductDistribution{}}
	var err error
	result.Products, err = s.ListProducts(ownerID)
	if err != nil {
		return nil, err
	}
	products := s.db.Model(&model.Product{}).Select("id").Where("owner_id = ? AND stage IN ?", ownerID, []string{"idea", "developing", "operating"})
	if err := s.db.Where("product_id IN (?) AND status NOT IN ?", products, []string{"done", "cancelled"}).Order("CASE status WHEN 'blocked' THEN 0 ELSE 1 END, CASE WHEN due_date = '' THEN 1 ELSE 0 END, due_date, id DESC").Find(&result.Tasks).Error; err != nil {
		return nil, err
	}
	if err := s.db.Where("product_id IN (?) AND status NOT IN ?", products, []string{"completed", "archived"}).Order("id DESC").Find(&result.Versions).Error; err != nil {
		return nil, err
	}
	// Older attempts must not remain urgent after a replacement submission exists.
	if err := s.db.Where("product_id IN (?) AND status NOT IN ?", products, []string{"published", "cancelled"}).Where("EXISTS (SELECT 1 FROM product_channels WHERE product_channels.id = product_distributions.channel_id AND product_channels.publish_status <> 'paused')").Where("NOT EXISTS (SELECT 1 FROM product_distributions newer WHERE newer.channel_id = product_distributions.channel_id AND newer.id > product_distributions.id)").Order("updated_at DESC").Find(&result.Distributions).Error; err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) ImportProductNextAction(ownerID, productID uint) (*model.ProductTask, error) {
	task := &model.ProductTask{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var product model.Product
		if err := tx.Where("id = ? AND owner_id = ?", productID, ownerID).First(&product).Error; err != nil {
			return err
		}
		if strings.TrimSpace(product.NextAction) == "" {
			return model.ErrProductLifecycleConflict
		}
		task.ProductID, task.Title, task.Kind, task.Status, task.Priority, task.Revision = productID, product.NextAction, "action", "todo", "normal", 1
		if product.NextDueDate != nil {
			task.DueDate = product.NextDueDate.Format("2006-01-02")
		}
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		return tx.Model(&product).Updates(map[string]interface{}{"next_action": "", "next_due_date": nil}).Error
	})
	return task, err
}
