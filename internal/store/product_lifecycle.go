package store

import (
	"errors"
	"slices"
	"strings"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	versionID := f.VersionID
	f.VersionID = 0
	q := lifecycleQuery(s.db, &model.ProductTask{}, productID, f)
	if f.TaskGroupID != nil {
		q = q.Where("group_id = ?", *f.TaskGroupID)
	}
	if f.OpenTasksOnly {
		q = q.Where("status IN ?", []string{"todo", "in_progress", "blocked"})
	}
	if versionID != 0 && !f.AssignableTasks {
		q = q.Where("id IN (?)", s.db.Model(&model.ProductVersionTask{}).Select("task_id").Where("version_id = ?", versionID))
	}
	if f.Keyword != "" {
		q = q.Where("instr(LOWER(title), ?) > 0", strings.ToLower(f.Keyword))
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.ProductTask{}
	err := q.Order("CASE status WHEN 'blocked' THEN 0 WHEN 'in_progress' THEN 1 WHEN 'todo' THEN 2 ELSE 3 END").Order("CASE WHEN due_date = '' THEN 1 ELSE 0 END, due_date, id DESC").Limit(f.Limit).Offset(f.Offset).Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return rows, count, err
	}
	ids := make([]uint, 0, len(rows))
	for _, task := range rows {
		ids = append(ids, task.ID)
	}
	var links []model.ProductVersionTask
	if err := s.db.Where("task_id IN ?", ids).Order("version_id").Find(&links).Error; err != nil {
		return nil, 0, err
	}
	byTask := make(map[uint][]uint)
	for _, link := range links {
		byTask[link.TaskID] = append(byTask[link.TaskID], link.VersionID)
	}
	for i := range rows {
		rows[i].VersionIDs = append([]uint{}, byTask[rows[i].ID]...)
	}
	return rows, count, nil
}

func (s *Store) ListProductVersions(ownerID, productID uint, f model.ProductLifecycleFilter) ([]model.ProductVersion, int64, error) {
	if err := ownedProduct(s.db, ownerID, productID); err != nil {
		return nil, 0, err
	}
	q := lifecycleQuery(s.db, &model.ProductVersion{}, productID, f)
	if f.ReleaseUnitID != nil {
		q = q.Where("release_unit_id = ?", *f.ReleaseUnitID)
	}
	if f.Keyword != "" {
		q = q.Where("instr(LOWER(name), ?) > 0", strings.ToLower(f.Keyword))
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.ProductVersion{}
	if err := q.Order("released_on DESC NULLS LAST, id DESC").Limit(f.Limit).Offset(f.Offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	for i := range rows {
		rows[i].TaskIDs = []uint{}
		if err := s.db.Model(&model.ProductVersionTask{}).Where("version_id = ?", rows[i].ID).Order("task_id").Pluck("task_id", &rows[i].TaskIDs).Error; err != nil {
			return nil, 0, err
		}
		rows[i].DeploymentIDs = []uint{}
		rows[i].PlatformReleaseIDs = []uint{}
		err := s.productDeployments(ownerID, productID, rows[i].ID).Joins("JOIN product_version_deployments ON product_version_deployments.release_id = releases.id").Where("product_version_deployments.version_id = ?", rows[i].ID).Order("releases.id DESC").Pluck("releases.id", &rows[i].DeploymentIDs).Error
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
	if f.ReleaseUnitID != nil {
		versions := s.db.Model(&model.ProductVersion{}).Select("id").Where("product_id = ? AND release_unit_id = ?", productID, *f.ReleaseUnitID)
		channels := s.db.Model(&model.ProductChannel{}).Select("id").Where("product_id = ? AND release_unit_id = ?", productID, *f.ReleaseUnitID)
		q = q.Where("(version_id IN (?) OR (version_id = 0 AND channel_id IN (?)))", versions, channels)
	}
	if f.Audience != "" {
		q = q.Where("audience = ?", f.Audience)
	}
	if f.EnvironmentID != nil {
		q = q.Where("environment_id = ?", *f.EnvironmentID)
	}
	if f.Keyword != "" {
		keyword := strings.ToLower(f.Keyword)
		versions := s.db.Model(&model.ProductVersion{}).Select("id").Where("product_id = ? AND instr(LOWER(name), ?) > 0", productID, keyword)
		q = q.Where("instr(LOWER(platform_version), ?) > 0 OR instr(LOWER(build_identifier), ?) > 0 OR version_id IN (?)", keyword, keyword, versions)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.ProductDistribution{}
	err := q.Order("id DESC").Limit(f.Limit).Offset(f.Offset).Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return rows, count, err
	}
	environmentIDs := make([]uint, 0, len(rows))
	for _, row := range rows {
		if row.EnvironmentID != 0 {
			environmentIDs = append(environmentIDs, row.EnvironmentID)
		}
	}
	if len(environmentIDs) > 0 {
		var environments []model.Environment
		if err := s.db.Where("id IN ?", environmentIDs).Find(&environments).Error; err != nil {
			return nil, 0, err
		}
		names := make(map[uint]string, len(environments))
		for _, environment := range environments {
			names[environment.ID] = environment.Name
		}
		for i := range rows {
			rows[i].EnvironmentName = names[rows[i].EnvironmentID]
		}
	}
	return rows, count, nil
}

func productEnvironmentAccess(tx *gorm.DB, ownerID, productID, environmentID uint) error {
	var count int64
	err := tx.Table("environments").
		Joins("JOIN projects ON projects.id = environments.project_id").
		Where("environments.id = ? AND projects.owner_id = ?", environmentID, ownerID).
		Where("(EXISTS (SELECT 1 FROM products p WHERE p.id = ? AND p.project_id = environments.project_id) OR EXISTS (SELECT 1 FROM product_projects pp WHERE pp.product_id = ? AND pp.project_id = environments.project_id) OR EXISTS (SELECT 1 FROM product_applications pa JOIN applications a ON a.id = pa.application_id WHERE pa.product_id = ? AND a.environment_id = environments.id))", productID, productID, productID).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count != 1 {
		return model.ErrProductResourceAccess
	}
	return nil
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

func taskVersionIDs(tx *gorm.DB, taskID uint) ([]uint, error) {
	ids := []uint{}
	err := tx.Model(&model.ProductVersionTask{}).Where("task_id = ?", taskID).Order("version_id").Pluck("version_id", &ids).Error
	return ids, err
}

func bumpProductVersionRevisions(tx *gorm.DB, productID uint, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return tx.Model(&model.ProductVersion{}).Where("product_id = ? AND id IN ?", productID, ids).
		UpdateColumn("revision", gorm.Expr("revision + 1")).Error
}

func (s *Store) SaveProductTask(ownerID uint, task *model.ProductTask) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := ownedProduct(tx, ownerID, task.ProductID); err != nil {
			return err
		}
		var current model.ProductTask
		var previous []uint
		if task.ID != 0 {
			if err := tx.Where("id = ? AND product_id = ?", task.ID, task.ProductID).First(&current).Error; err != nil {
				return err
			}
			if current.Revision != task.Revision {
				return model.ErrProductLifecycleConflict
			}
			if task.KeepGroup {
				task.GroupID = current.GroupID
			}
			var err error
			previous, err = taskVersionIDs(tx, task.ID)
			if err != nil {
				return err
			}
		}
		if err := productTaskGroupAccess(tx, task.ProductID, task.GroupID); err != nil {
			return err
		}
		ids := task.VersionIDs
		if ids == nil {
			// Older callers edit the primary link only. Preserve other release links.
			ids = append([]uint{}, previous...)
			if current.VersionID != nil {
				ids = slices.DeleteFunc(ids, func(id uint) bool { return id == *current.VersionID })
			}
			if task.VersionID != nil {
				ids = append(ids, *task.VersionID)
			}
		}
		slices.Sort(ids)
		ids = slices.Compact(ids)
		if len(ids) > 0 {
			var count int64
			if err := tx.Model(&model.ProductVersion{}).Where("product_id = ? AND id IN ?", task.ProductID, ids).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(ids)) {
				return model.ErrProductResourceAccess
			}
			task.VersionID = &ids[0]
		} else {
			task.VersionID = nil
		}
		task.VersionIDs = ids
		if task.ID == 0 {
			task.Revision = 1
			if err := tx.Create(task).Error; err != nil {
				return err
			}
		} else {
			task.Revision++
			task.CreatedAt = current.CreatedAt
			result := tx.Model(&model.ProductTask{}).Where("id = ? AND revision = ?", task.ID, current.Revision).Select("*").Omit("id", "created_at").Updates(task)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return model.ErrProductLifecycleConflict
			}
		}
		var changed []uint
		for _, id := range previous {
			if slices.Contains(ids, id) {
				continue
			}
			if err := tx.Where("task_id = ? AND version_id = ?", task.ID, id).Delete(&model.ProductVersionTask{}).Error; err != nil {
				return err
			}
			changed = append(changed, id)
		}
		for _, id := range ids {
			if slices.Contains(previous, id) {
				continue
			}
			if err := tx.Create(&model.ProductVersionTask{TaskID: task.ID, VersionID: id}).Error; err != nil {
				return err
			}
			changed = append(changed, id)
		}
		return bumpProductVersionRevisions(tx, task.ProductID, changed)
	})
}

func (s *Store) DeleteProductTask(ownerID, productID, taskID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := ownedProduct(tx, ownerID, productID); err != nil {
			return err
		}
		result := tx.Where("product_id = ? AND id = ?", productID, taskID).Delete(&model.ProductTask{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		ids, err := taskVersionIDs(tx, taskID)
		if err != nil {
			return err
		}
		if err := tx.Where("task_id = ?", taskID).Delete(&model.ProductVersionTask{}).Error; err != nil {
			return err
		}
		return bumpProductVersionRevisions(tx, productID, ids)
	})
}

func (s *Store) SaveProductVersion(ownerID uint, version *model.ProductVersion) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := ownedProduct(tx, ownerID, version.ProductID); err != nil {
			return err
		}
		var current model.ProductVersion
		if version.ID != 0 {
			if err := tx.Where("id = ? AND product_id = ?", version.ID, version.ProductID).First(&current).Error; err != nil {
				return err
			}
			if current.Revision != version.Revision {
				return model.ErrProductLifecycleConflict
			}
			if version.KeepReleaseUnit {
				version.ReleaseUnitID = current.ReleaseUnitID
			}
			if version.ReleasedOn == "" {
				version.ReleasedOn = current.ReleasedOn
			}
		}
		if version.ReleasedOn != "" {
			version.Status = "completed"
		}
		if err := productReleaseUnitAccess(tx, version.ProductID, version.ReleaseUnitID); err != nil {
			return err
		}
		if err := validateVersionGitHubSources(tx, ownerID, version, &current); err != nil {
			return err
		}
		var duplicate int64
		if err := tx.Model(&model.ProductVersion{}).Where("product_id = ? AND release_unit_id = ? AND name = ? AND id <> ?", version.ProductID, version.ReleaseUnitID, version.Name, version.ID).Count(&duplicate).Error; err != nil {
			return err
		}
		if duplicate != 0 {
			return model.ErrProductVersionName
		}
		ids := map[uint]bool{}
		for _, id := range version.DeploymentIDs {
			var count int64
			if err := (&Store{db: tx}).productDeployments(ownerID, version.ProductID, version.ID).Where("releases.id = ?", id).Count(&count).Error; err != nil {
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
			if current.ReleaseUnitID != version.ReleaseUnitID {
				var incompatible int64
				err := tx.Model(&model.ProductDistribution{}).Joins("JOIN product_channels ON product_channels.id = product_distributions.channel_id").
					Where("product_distributions.version_id = ? AND product_channels.release_unit_id <> ?", version.ID, version.ReleaseUnitID).Count(&incompatible).Error
				if err != nil {
					return err
				}
				if incompatible > 0 {
					return model.ErrProductChannelReleaseUnit
				}
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
		if version.TaskIDs != nil {
			if err := saveProductVersionTasks(tx, version); err != nil {
				return err
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

func saveProductVersionTasks(tx *gorm.DB, version *model.ProductVersion) error {
	ids := version.TaskIDs
	slices.Sort(ids)
	ids = slices.Compact(ids)
	version.TaskIDs = ids
	if len(ids) > 0 {
		var available int64
		if err := tx.Model(&model.ProductTask{}).Where("product_id = ? AND id IN ?", version.ProductID, ids).Count(&available).Error; err != nil {
			return err
		}
		if available != int64(len(ids)) {
			return model.ErrProductLifecycleConflict
		}
	}
	var previous []uint
	if err := tx.Model(&model.ProductVersionTask{}).Where("version_id = ?", version.ID).Pluck("task_id", &previous).Error; err != nil {
		return err
	}
	var changed []uint
	for _, id := range previous {
		if slices.Contains(ids, id) {
			continue
		}
		if err := tx.Where("version_id = ? AND task_id = ?", version.ID, id).Delete(&model.ProductVersionTask{}).Error; err != nil {
			return err
		}
		changed = append(changed, id)
	}
	for _, id := range ids {
		if slices.Contains(previous, id) {
			continue
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.ProductVersionTask{VersionID: version.ID, TaskID: id}).Error; err != nil {
			return err
		}
		changed = append(changed, id)
	}
	if len(changed) == 0 {
		return nil
	}
	primary := gorm.Expr("(SELECT MIN(version_id) FROM product_version_tasks WHERE task_id = product_tasks.id)")
	return tx.Model(&model.ProductTask{}).Where("product_id = ? AND id IN ?", version.ProductID, changed).
		Updates(map[string]any{"version_id": primary, "revision": gorm.Expr("revision + 1")}).Error
}

func (s *Store) SaveProductDistribution(ownerID uint, record *model.ProductDistribution) error {
	if record.Audience == "" {
		record.Audience = "unclassified"
	}
	if record.Audience != "unclassified" && record.Audience != "testing" && record.Audience != "production" {
		return model.ErrProductResourceAccess
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := ownedProduct(tx, ownerID, record.ProductID); err != nil {
			return err
		}
		var version model.ProductVersion
		if record.VersionID != 0 {
			if err := tx.Select("name", "release_unit_id").Where("id = ? AND product_id = ?", record.VersionID, record.ProductID).First(&version).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return model.ErrProductResourceAccess
				}
				return err
			}
		}
		if record.PlatformVersion == "" {
			record.PlatformVersion = version.Name
		}
		var current model.ProductDistribution
		if record.ID != 0 {
			if err := tx.Where("id = ? AND product_id = ?", record.ID, record.ProductID).First(&current).Error; err != nil {
				return err
			}
			if record.KeepEnvironment {
				record.EnvironmentID = current.EnvironmentID
			}
			if record.KeepDeployments {
				record.DeploymentIDs = append([]uint(nil), current.DeploymentIDs...)
			}
		}
		if record.EnvironmentID == 0 && len(record.DeploymentIDs) > 0 {
			return model.ErrProductResourceAccess
		}
		if record.EnvironmentID != 0 {
			if err := productEnvironmentAccess(tx, ownerID, record.ProductID, record.EnvironmentID); err != nil {
				return err
			}
			if len(record.DeploymentIDs) > 0 {
				ids := append([]uint(nil), record.DeploymentIDs...)
				slices.Sort(ids)
				ids = slices.Compact(ids)
				if len(ids) != len(record.DeploymentIDs) {
					return model.ErrProductResourceAccess
				}
				q := tx.Model(&model.Release{}).
					Joins("JOIN applications ON applications.id = releases.application_id").
					Joins("JOIN projects ON projects.id = applications.project_id").
					Where("projects.owner_id = ? AND applications.environment_id = ?", ownerID, record.EnvironmentID)
				applications := tx.Model(&model.ProductApplication{}).Select("application_id").Where("product_id = ?", record.ProductID)
				projects := tx.Model(&model.ProductProject{}).Select("project_id").Where("product_id = ?", record.ProductID)
				q = q.Where("(applications.id IN (?) OR applications.project_id IN (?) OR EXISTS (SELECT 1 FROM products p WHERE p.id = ? AND p.project_id = applications.project_id))", applications, projects, record.ProductID)
				var count int64
				if err := q.Where("releases.id IN ?", ids).Distinct("releases.id").Count(&count).Error; err != nil {
					return err
				}
				if count != int64(len(ids)) {
					return model.ErrProductResourceAccess
				}
				record.DeploymentIDs = ids
			}
		} else {
			record.DeploymentIDs = []uint{}
		}
		becamePublished := record.Status == "published"
		var channel model.ProductChannel
		if err := tx.Where("id = ? AND product_id = ?", record.ChannelID, record.ProductID).First(&channel).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.ErrProductResourceAccess
			}
			return err
		}
		if record.ID == 0 && record.VersionID != 0 && channel.ReleaseUnitID != nil && *channel.ReleaseUnitID != version.ReleaseUnitID {
			return model.ErrProductChannelReleaseUnit
		}
		if record.ID == 0 {
			record.Revision = 1
			if err := tx.Create(record).Error; err != nil {
				return err
			}
		} else {
			becamePublished = record.Status == "published" && (current.Status != "published" || current.PlatformVersion != record.PlatformVersion || current.Audience != record.Audience)
			// A submission's identity is immutable; resubmission is a new record.
			if current.ChannelID != record.ChannelID || (current.VersionID != 0 && current.VersionID != record.VersionID) {
				return model.ErrProductResourceAccess
			}
			if current.VersionID == 0 && record.VersionID != 0 && channel.ReleaseUnitID != nil && *channel.ReleaseUnitID != version.ReleaseUnitID {
				return model.ErrProductChannelReleaseUnit
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
		environmentName := ""
		if record.EnvironmentID != 0 {
			var environment model.Environment
			if err := tx.Select("name").First(&environment, record.EnvironmentID).Error; err != nil {
				return err
			}
			environmentName = environment.Name
		}
		event := model.ProductDistributionEvent{Audience: record.Audience, BecamePublished: becamePublished, DistributionID: record.ID, EnvironmentID: record.EnvironmentID, EnvironmentName: environmentName, DeploymentIDs: append([]uint(nil), record.DeploymentIDs...), ActorID: ownerID, Status: record.Status, PlatformVersion: record.PlatformVersion, BuildIdentifier: record.BuildIdentifier, Notes: record.Notes}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		return syncProductChannel(tx, &channel)
	})
}

func syncProductChannel(tx *gorm.DB, channel *model.ProductChannel) error {
	var latest model.ProductDistribution
	err := tx.Where("channel_id = ? AND audience = ?", channel.ID, "production").Order("id DESC").First(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		var count int64
		if err := tx.Model(&model.ProductDistribution{}).Where("channel_id = ?", channel.ID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
		channel.OnlineVersion, channel.PendingVersion = "", ""
		if channel.PublishStatus != "paused" {
			channel.PublishStatus = "draft"
		}
		return tx.Model(channel).Select("publish_status", "pending_version", "online_version", "updated_at").Updates(channel).Error
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
	err = tx.Joins("JOIN product_distributions ON product_distributions.id = product_distribution_events.distribution_id").Where("product_distributions.channel_id = ? AND product_distribution_events.became_published = ? AND product_distribution_events.audience = ? AND product_distributions.audience = 'production'", channel.ID, true, "production").Order("product_distribution_events.id DESC").Select("product_distribution_events.*").First(&published).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	channel.OnlineVersion = ""
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

func (s *Store) productDeployments(ownerID, productID, versionID uint) *gorm.DB {
	scope := s.db.Where("EXISTS (SELECT 1 FROM product_applications WHERE product_applications.product_id = ? AND product_applications.application_id = applications.id)", productID).
		Or("EXISTS (SELECT 1 FROM product_projects WHERE product_projects.product_id = ? AND product_projects.project_id = "+applicationOwningProject+")", productID)
	if versionID != 0 {
		// Preserve existing version links even after an application's project changes.
		scope = scope.Or("EXISTS (SELECT 1 FROM product_version_deployments pvd JOIN product_versions pv ON pv.id = pvd.version_id WHERE pv.product_id = ? AND pvd.version_id = ? AND pvd.release_id = releases.id)", productID, versionID)
	}
	return s.db.Model(&model.Release{}).Joins("JOIN applications ON applications.id = releases.application_id").Joins("JOIN projects ON projects.id = applications.project_id").Where("projects.owner_id = ?", ownerID).
		Where(scope)
}

func (s *Store) productPlatformReleases(ownerID, productID uint) *gorm.DB {
	return s.db.Model(&model.PlatformRelease{}).Where("EXISTS (SELECT 1 FROM products WHERE products.id = ? AND products.owner_id = ? AND products.platform_linked = ?)", productID, ownerID, true)
}

func (s *Store) ProductDeploymentPage(ownerID, productID uint, f model.ProductLifecycleFilter) ([]model.ProductDeploymentOption, int64, error) {
	if err := ownedProduct(s.db, ownerID, productID); err != nil {
		return nil, 0, err
	}
	q := s.productDeployments(ownerID, productID, f.VersionID)
	platform := s.productPlatformReleases(ownerID, productID)
	if f.EnvironmentID != nil {
		if err := productEnvironmentAccess(s.db, ownerID, productID, *f.EnvironmentID); err != nil {
			return nil, 0, err
		}
		q = q.Where("applications.environment_id = ?", *f.EnvironmentID)
		platform = platform.Where("1 = 0")
	}
	if f.VersionID != 0 {
		if err := lifecycleVersionAccess(s.db, productID, &f.VersionID); err != nil {
			return nil, 0, err
		}
		if !f.IncludeVersionDeployments {
			q = q.Joins("JOIN product_version_deployments ON product_version_deployments.release_id = releases.id").Where("product_version_deployments.version_id = ?", f.VersionID)
			platform = platform.Joins("JOIN product_version_platform_releases ON product_version_platform_releases.release_id = platform_releases.id").Where("product_version_platform_releases.version_id = ?", f.VersionID)
		}
	}
	if f.Keyword != "" {
		q = q.Where("instr(LOWER(applications.name || ' ' || releases.version), ?) > 0", strings.ToLower(f.Keyword))
		platform = platform.Where("instr(LOWER('Cylism Manager 当前管理平台 ' || platform_releases.image), ?) > 0", strings.ToLower(f.Keyword))
	}
	if f.Status != "" {
		q = q.Where("releases.status = ?", f.Status)
		platform = platform.Where("platform_releases.status = ?", f.Status)
	}
	applications := q.Joins("LEFT JOIN environments ON environments.id = applications.environment_id").Select("'application' AS source, releases.id, releases.application_id, applications.name AS application_name, environments.name AS environment_name, environments.kind AS environment_kind, releases.version, releases.status, releases.created_at")
	updates := platform.Select("'platform' AS source, platform_releases.id, 0 AS application_id, 'Cylism Manager' AS application_name, '当前管理平台' AS environment_name, 'unclassified' AS environment_kind, platform_releases.image AS version, platform_releases.status, platform_releases.created_at")
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
	err := s.db.Select("id", "name", "status", "release_unit_id", "released_on").Where("product_id = ?", productID).Order("id DESC").Find(&rows).Error
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
