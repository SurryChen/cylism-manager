package store

import (
	"errors"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func weChatChannel(tx *gorm.DB, product, channel uint) (model.ProductChannel, error) {
	var row model.ProductChannel
	err := tx.Where("id = ? AND product_id = ? AND platform IN ?", channel, product,
		[]string{model.ProductChannelMiniGame, model.ProductChannelWeChatMiniProgram}).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = model.ErrWeChatChannel
	}
	return row, err
}

func (s *Store) WeChatConnections(owner, product uint) ([]model.ProductWeChatConnection, error) {
	if err := ownedProduct(s.db, owner, product); err != nil {
		return nil, err
	}
	connections := []model.ProductWeChatConnection{}
	err := s.db.Where("product_id = ?", product).Order("channel_id").Find(&connections).Error
	return connections, err
}

func (s *Store) WeChatConnection(owner, product, channel uint) (model.ProductWeChatConnection, error) {
	var item model.ProductWeChatConnection
	if err := ownedProduct(s.db, owner, product); err != nil {
		return item, err
	}
	err := s.db.Where("product_id = ? AND channel_id = ?", product, channel).Take(&item).Error
	return item, err
}

func (s *Store) SaveWeChatConnection(owner, product, channel uint, appID, secret string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := ownedProduct(tx, owner, product); err != nil {
			return err
		}
		row, err := weChatChannel(tx, product, channel)
		if err != nil {
			return err
		}
		if row.AppIdentifier != "" && row.AppIdentifier != appID {
			return model.ErrWeChatAppID
		}
		item := model.ProductWeChatConnection{ProductID: product, ChannelID: channel, AppID: appID, EncryptedAppSecret: secret}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&item)
		if result.Error != nil || result.RowsAffected == 1 {
			return result.Error
		}
		result = tx.Model(&item).Where("product_id = ? AND app_id = ?", product, appID).Update("encrypted_app_secret", gorm.Expr("CASE WHEN ? = '' THEN encrypted_app_secret ELSE ? END", secret, secret))
		if result.Error == nil && result.RowsAffected != 1 {
			return model.ErrWeChatAppID
		}
		return result.Error
	})
}

func (s *Store) DeleteWeChatConnection(owner, product, channel uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := ownedProduct(tx, owner, product); err != nil {
			return err
		}
		if err := tx.Where("channel_id = ? AND product_id = ?", channel, product).Delete(&model.ProductWeChatConnection{}).Error; err != nil {
			return err
		}
		ids := tx.Model(&model.ProductWeChatConnection{}).Select("channel_id")
		return tx.Where("channel_id = ? AND channel_id NOT IN (?)", channel, ids).Delete(&model.WeChatDailyMetric{}).Error
	})
}

func (s *Store) SaveWeChatTicket(item *model.WeChatComponentTicket) error {
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "app_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"encrypted_ticket", "issued_at"}),
		Where:     clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "excluded.issued_at > we_chat_component_tickets.issued_at"}}},
	}).Create(item).Error
}

func (s *Store) WeChatTicket(appID string) (model.WeChatComponentTicket, error) {
	var item model.WeChatComponentTicket
	err := s.db.Where("app_id = ?", appID).Take(&item).Error
	return item, err
}

func (s *Store) StartWeChatAuthorization(item model.ProductWeChatConnection, state string, expiry time.Time) error {
	return updateWeChatConnection(s.db.Model(&item).Where("app_id = ?", item.AppID).Updates(map[string]any{
		"auth_state_hash": state, "auth_expires_at": expiry,
	}))
}

func (s *Store) ClaimWeChatAuthorization(state string, now time.Time) (model.ProductWeChatConnection, error) {
	var item model.ProductWeChatConnection
	result := s.db.Model(&item).Clauses(clause.Returning{}).Where("auth_state_hash = ? AND auth_expires_at > ?", state, now).
		Update("auth_expires_at", nil)
	if result.Error == nil && result.RowsAffected != 1 {
		return item, model.ErrWeChatAuthorization
	}
	return item, result.Error
}

func (s *Store) FinishWeChatAuthorization(item model.ProductWeChatConnection, component, encrypted string, grantedAt int64) error {
	return updateWeChatConnection(s.db.Model(&item).Where("app_id = ? AND auth_state_hash = ? AND auth_expires_at IS NULL", item.AppID, item.AuthStateHash).Updates(map[string]any{
		"component_app_id": component, "encrypted_refresh_token": encrypted, "authorized_at": grantedAt,
		"auth_state_hash": "", "release_checked_at": nil, "release_error": "",
	}))
}

func (s *Store) RevokeWeChatAuthorization(component, appID string, issuedAt int64) error {
	return s.db.Model(&model.ProductWeChatConnection{}).Where("component_app_id = ? AND app_id = ? AND authorized_at <= ?", component, appID, issuedAt).
		Updates(map[string]any{"encrypted_refresh_token": "", "release_error": "微信版本授权已解除，请重新连接"}).Error
}

func weChatGrant(db *gorm.DB, item model.ProductWeChatConnection) *gorm.DB {
	return db.Model(&item).Where("app_id = ? AND component_app_id = ? AND authorized_at = ? AND encrypted_refresh_token <> ''", item.AppID, item.ComponentAppID, item.AuthorizedAt)
}

func (s *Store) RefreshWeChatGrant(item model.ProductWeChatConnection, replacement string) error {
	return updateWeChatConnection(weChatGrant(s.db, item).Where("encrypted_refresh_token = ?", item.EncryptedRefreshToken).Update("encrypted_refresh_token", replacement))
}

func updateWeChatConnection(result *gorm.DB) error {
	if result.Error == nil && result.RowsAffected != 1 {
		return model.ErrWeChatAuthorization
	}
	return result.Error
}

func (s *Store) WeChatReleaseSyncTargets(component string, before time.Time) ([]model.ProductWeChatConnection, error) {
	var items []model.ProductWeChatConnection
	err := s.db.Where("component_app_id = ? AND encrypted_refresh_token <> '' AND (release_checked_at IS NULL OR release_checked_at < ?)", component, before).
		Order("release_checked_at, channel_id").Limit(100).Find(&items).Error
	return items, err
}

func (s *Store) RecordWeChatReleaseError(item model.ProductWeChatConnection, message string, at time.Time) error {
	return updateWeChatConnection(weChatGrant(s.db, item).Updates(map[string]any{"release_error": message, "release_checked_at": at}))
}

func (s *Store) ImportWeChatRelease(item model.ProductWeChatConnection, snapshot model.WeChatReleaseSnapshot, checkedAt time.Time) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Obtain the write lock before reading and merging lifecycle rows.
		values := map[string]any{
			"online_version": gorm.Expr("CASE WHEN online_at > ? THEN online_version ELSE ? END", snapshot.OnlineAt, snapshot.OnlineVersion),
			"online_at":      gorm.Expr("CASE WHEN online_at > ? THEN online_at ELSE ? END", snapshot.OnlineAt, snapshot.OnlineAt),
			"trial_version":  snapshot.TrialVersion, "release_error": "", "release_checked_at": checkedAt,
			"audit_status": snapshot.AuditStatus, "audit_reason": snapshot.AuditReason, "audit_error": snapshot.AuditError,
		}
		if snapshot.OnlineVersion != "" && snapshot.OnlineAt == nil {
			values["release_error"] = "微信未返回有效的线上发布时间，暂未登记发行记录"
		}
		if err := updateWeChatConnection(weChatGrant(tx, item).Updates(values)); err != nil {
			return err
		}
		var current model.ProductWeChatConnection
		if err := tx.Where("channel_id = ?", item.ChannelID).Take(&current).Error; err != nil {
			return err
		}
		if snapshot.OnlineAt == nil || snapshot.OnlineVersion == "" ||
			(current.LastImportedAt != nil && !snapshot.OnlineAt.After(*current.LastImportedAt)) {
			return nil
		}
		channel, err := weChatChannel(tx, item.ProductID, item.ChannelID)
		if err != nil {
			return err
		}
		var product model.Product
		if err = tx.Select("owner_id").Where("id = ?", item.ProductID).Take(&product).Error; err != nil {
			return err
		}
		if err = importWeChatDistribution(tx, product.OwnerID, channel, snapshot); err != nil {
			return err
		}
		return tx.Model(&current).Update("last_imported_at", snapshot.OnlineAt).Error
	})
}

func importWeChatDistribution(tx *gorm.DB, owner uint, channel model.ProductChannel, snapshot model.WeChatReleaseSnapshot) error {
	store := &Store{db: tx}
	unit := uint(0)
	if channel.ReleaseUnitID != nil {
		unit = *channel.ReleaseUnitID
	}
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	releasedOn := snapshot.OnlineAt.In(location).Format("2006-01-02")
	var record model.ProductDistribution
	err := tx.Where("product_id = ? AND channel_id = ? AND platform_version = ? AND ((audience = 'production' AND status IN ?) OR (audience = 'unclassified' AND status = 'published'))",
		channel.ProductID, channel.ID, snapshot.OnlineVersion, []string{"published", "draft", "preparing", "ready", "in_review", "approved"}).
		Order("CASE WHEN status = 'published' AND audience = 'production' THEN 0 WHEN status = 'published' THEN 1 ELSE 2 END, id DESC").Take(&record).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	version := model.ProductVersion{ID: record.VersionID}
	if version.ID == 0 {
		err = tx.Where("product_id = ? AND release_unit_id = ? AND name = ?", channel.ProductID, unit, snapshot.OnlineVersion).Take(&version).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			version = model.ProductVersion{ProductID: channel.ProductID, ReleaseUnitID: unit, Name: snapshot.OnlineVersion, ReleasedOn: releasedOn, Notes: snapshot.OnlineNotes, Status: "completed"}
			err = store.SaveProductVersion(owner, &version)
		}
	} else {
		err = tx.Where("product_id = ? AND id = ?", channel.ProductID, version.ID).Take(&version).Error
	}
	if err != nil {
		return err
	}
	if version.ReleasedOn == "" {
		if err := tx.Model(&version).Updates(map[string]any{"released_on": releasedOn, "status": "completed", "revision": gorm.Expr("revision + 1")}).Error; err != nil {
			return err
		}
	}
	if record.Status == "published" && record.Audience == "production" && record.VersionID != 0 {
		return tx.Model(&record).UpdateColumn("we_chat_published_at", snapshot.OnlineAt).Error
	}
	record.ProductID, record.ChannelID, record.VersionID = channel.ProductID, channel.ID, version.ID
	record.Audience, record.Status, record.PlatformVersion = "production", "published", snapshot.OnlineVersion
	record.WeChatPublishedAt = snapshot.OnlineAt
	if record.Notes == "" {
		record.Notes = snapshot.OnlineNotes
	}
	return store.saveProductDistribution(owner, 0, &record)
}

func (s *Store) WeChatMetrics(channel uint, from string) ([]model.WeChatDailyMetric, error) {
	items := []model.WeChatDailyMetric{}
	err := s.db.Where("channel_id = ? AND day >= ?", channel, from).Order("day, metric").Find(&items).Error
	return items, err
}

func (s *Store) SaveWeChatMetrics(item model.ProductWeChatConnection, samples []model.WeChatDailyMetric, before string) error {
	if len(samples) == 0 {
		return nil
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		// An in-flight refresh must not save data after a disconnect or credential change.
		result := tx.Model(&item).Where("product_id = ? AND app_id = ? AND encrypted_app_secret = ?", item.ProductID, item.AppID, item.EncryptedAppSecret).
			UpdateColumn("app_id", gorm.Expr("app_id"))
		if err := updateWeChatConnection(result); err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "channel_id"}, {Name: "day"}, {Name: "metric"}},
			DoUpdates: clause.AssignmentColumns([]string{"value", "fetched_at"}),
		}).CreateInBatches(samples, 100).Error; err != nil {
			return err
		}
		return tx.Where("channel_id = ? AND day < ?", item.ChannelID, before).Delete(&model.WeChatDailyMetric{}).Error
	})
}

func (s *Store) WeChatAnalyticsTargets() ([]model.ProductWeChatConnection, error) {
	var items []model.ProductWeChatConnection
	err := s.db.Where("encrypted_app_secret <> ''").Order("channel_id").Find(&items).Error
	return items, err
}

func (s *Store) WeChatChannelPlatform(item model.ProductWeChatConnection) (string, error) {
	channel, err := weChatChannel(s.db, item.ProductID, item.ChannelID)
	return channel.Platform, err
}

func deleteRemovedWeChatConnections(tx *gorm.DB, product uint, keep []uint) error {
	removed := tx.Model(&model.ProductWeChatConnection{}).Where("product_id = ? AND (channel_id NOT IN ? OR NOT EXISTS (SELECT 1 FROM product_channels c WHERE c.id = product_we_chat_connections.channel_id AND c.platform IN ? AND (c.app_identifier = '' OR c.app_identifier = product_we_chat_connections.app_id)))",
		product, keep, []string{model.ProductChannelMiniGame, model.ProductChannelWeChatMiniProgram})
	if err := tx.Where("channel_id IN (?)", removed.Select("channel_id")).Delete(&model.WeChatDailyMetric{}).Error; err != nil {
		return err
	}
	return removed.Delete(&model.ProductWeChatConnection{}).Error
}
