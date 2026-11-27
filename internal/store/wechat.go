package store

import (
	"errors"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func removeLegacyWeChatCredentials(db *gorm.DB) error {
	hasGrant := db.Migrator().HasColumn(&model.ProductWeChatConnection{}, "encrypted_refresh_token")
	if !hasGrant && !db.Migrator().HasTable("we_chat_component_tickets") {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if hasGrant {
			if err := tx.Exec(`UPDATE product_we_chat_connections
				SET encrypted_refresh_token = '', component_app_id = '', authorized_at = 0, auth_state_hash = '', auth_expires_at = NULL
				WHERE COALESCE(encrypted_refresh_token, '') <> '' OR COALESCE(component_app_id, '') <> ''
				OR COALESCE(authorized_at, 0) <> 0 OR COALESCE(auth_state_hash, '') <> '' OR auth_expires_at IS NOT NULL`).Error; err != nil {
				return err
			}
		}
		return tx.Exec("DROP TABLE IF EXISTS we_chat_component_tickets").Error
	})
}

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
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return model.ErrWeChatConnectionChanged
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
