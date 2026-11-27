package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func TestRetireWeChatThirdPartyCredentialsPreservesDirectConnection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wechat-upgrade.db")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	product, item := linkedWeChatChannel(t, s)
	now := time.Now().UTC().Truncate(time.Second)
	issue := model.ProductDistribution{ProductID: product.ID, ChannelID: item.ChannelID,
		Status: "published", Audience: "production", PlatformVersion: "0.4.0", WeChatPublishedAt: &now}
	if err := s.SaveProductDistribution(product.OwnerID, &issue); err != nil {
		t.Fatal(err)
	}
	metric := model.WeChatDailyMetric{ChannelID: item.ChannelID, Day: "2026-11-27", Metric: "visits", FetchedAt: now}
	if err := s.SaveWeChatMetrics(item, []model.WeChatDailyMetric{metric}, ""); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		"ALTER TABLE product_we_chat_connections ADD COLUMN encrypted_refresh_token TEXT",
		"ALTER TABLE product_we_chat_connections ADD COLUMN component_app_id TEXT",
		"ALTER TABLE product_we_chat_connections ADD COLUMN authorized_at INTEGER",
		"ALTER TABLE product_we_chat_connections ADD COLUMN auth_state_hash TEXT",
		"ALTER TABLE product_we_chat_connections ADD COLUMN auth_expires_at DATETIME",
		"UPDATE product_we_chat_connections SET encrypted_refresh_token = 'cached-grant', component_app_id = 'component', authorized_at = 1, auth_state_hash = 'pending', auth_expires_at = CURRENT_TIMESTAMP",
		"CREATE TABLE we_chat_component_tickets (app_id TEXT PRIMARY KEY, encrypted_ticket TEXT)",
		"INSERT INTO we_chat_component_tickets VALUES ('component', 'cached-ticket')",
	} {
		if err := s.db.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		upgraded, err := New(path)
		if err != nil {
			t.Fatal(err)
		}
		var grants int64
		err = upgraded.db.Table("product_we_chat_connections").
			Where("encrypted_refresh_token <> '' OR component_app_id <> '' OR authorized_at <> 0 OR auth_state_hash <> '' OR auth_expires_at IS NOT NULL").
			Count(&grants).Error
		if err != nil || grants != 0 || upgraded.db.Migrator().HasTable("we_chat_component_tickets") {
			t.Fatalf("legacy credentials remain: %d, %v", grants, err)
		}
		got, err := upgraded.WeChatConnection(product.OwnerID, item.ProductID, item.ChannelID)
		if err != nil || got.AppID != item.AppID || got.EncryptedAppSecret != item.EncryptedAppSecret {
			t.Fatalf("direct connection changed: %#v, %v", got, err)
		}
		if samples, err := upgraded.WeChatMetrics(item.ChannelID, ""); err != nil || len(samples) != 1 || samples[0].Metric != metric.Metric {
			t.Fatalf("daily metrics changed: %#v, %v", samples, err)
		}
		issues, total, err := upgraded.ListProductDistributions(product.OwnerID, product.ID, model.ProductLifecycleFilter{Limit: 10})
		if err != nil || total != 1 || issues[0].WeChatPublishedAt == nil || !issues[0].WeChatPublishedAt.Equal(now) {
			t.Fatalf("distribution history changed: %#v, %d, %v", issues, total, err)
		}
	}
}

func linkedWeChatChannel(t *testing.T, s *Store) (model.Product, model.ProductWeChatConnection) {
	t.Helper()
	product := model.Product{OwnerID: 9, Name: "小游戏", Stage: "developing", Channels: []model.ProductChannel{{Name: "微信小游戏", Platform: model.ProductChannelMiniGame, PublishStatus: "draft"}}}
	if err := s.SaveProduct(&product, nil, nil); err != nil {
		t.Fatal(err)
	}
	channel := product.Channels[0].ID
	if err := s.SaveWeChatConnection(9, product.ID, channel, "wx1234567890abcdef", "encrypted-secret"); err != nil {
		t.Fatal(err)
	}
	item, err := s.WeChatConnection(9, product.ID, channel)
	if err != nil {
		t.Fatal(err)
	}
	return product, item
}

func TestWeChatConnectionOwnershipAndCleanup(t *testing.T) {
	s := setupTestDB(t)
	p, item := linkedWeChatChannel(t, s)
	if _, err := s.WeChatConnection(10, p.ID, item.ChannelID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("other owner read: %v", err)
	}
	if err := s.DeleteWeChatConnection(10, p.ID, item.ChannelID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("other owner delete: %v", err)
	}
	if err := s.SaveWeChatConnection(p.OwnerID, p.ID, item.ChannelID, item.AppID, ""); err != nil {
		t.Fatal(err)
	}
	got, err := s.WeChatConnection(p.OwnerID, p.ID, item.ChannelID)
	if err != nil || got.EncryptedAppSecret != item.EncryptedAppSecret {
		t.Fatalf("blank secret should preserve credential: %#v, %v", got, err)
	}
	saveMetric := func() error {
		return s.SaveWeChatMetrics(item, []model.WeChatDailyMetric{{ChannelID: item.ChannelID, Day: "2026-11-27", Metric: "visits", FetchedAt: time.Now()}}, "")
	}
	if err := saveMetric(); err != nil {
		t.Fatal(err)
	}
	p.Channels[0].Platform = "web"
	if err := s.SaveProduct(&p, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WeChatConnection(p.OwnerID, p.ID, item.ChannelID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("changed channel should remove credential: %v", err)
	}
	if rows, err := s.WeChatMetrics(item.ChannelID, ""); err != nil || len(rows) != 0 {
		t.Fatalf("changed channel should remove analytics: %#v, %v", rows, err)
	}
	if err := saveMetric(); !errors.Is(err, model.ErrWeChatConnectionChanged) {
		t.Fatalf("in-flight metrics repopulated disconnected channel: %v", err)
	}
}
