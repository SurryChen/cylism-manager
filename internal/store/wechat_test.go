package store

import (
	"errors"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

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

func grantWeChatChannel(t *testing.T, s *Store, product model.Product, item model.ProductWeChatConnection) model.ProductWeChatConnection {
	t.Helper()
	now := time.Now().UTC()
	if err := s.StartWeChatAuthorization(item, "state", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	claimed, err := s.ClaimWeChatAuthorization("state", now)
	if err != nil || claimed.ChannelID != item.ChannelID || claimed.AppID != item.AppID || claimed.AuthStateHash != "state" || claimed.AuthExpiresAt != nil {
		t.Fatalf("claim: %#v, %v", claimed, err)
	}
	if _, err := s.ClaimWeChatAuthorization("state", now); !errors.Is(err, model.ErrWeChatAuthorization) {
		t.Fatalf("repeated claim: %v", err)
	}
	if err := s.FinishWeChatAuthorization(claimed, "component", "encrypted-refresh", now.Unix()); err != nil {
		t.Fatal(err)
	}
	granted, err := s.WeChatConnection(product.OwnerID, item.ProductID, item.ChannelID)
	if err != nil || granted.EncryptedRefreshToken != "encrypted-refresh" || granted.AuthStateHash != "" {
		t.Fatalf("grant: %#v, %v", granted, err)
	}
	return granted
}

func TestWeChatTicketKeepsNewestEvent(t *testing.T) {
	s := setupTestDB(t)
	for _, stamp := range []int64{5, 7, 6} {
		if err := s.SaveWeChatTicket(&model.WeChatComponentTicket{AppID: "component", IssuedAt: stamp}); err != nil {
			t.Fatal(err)
		}
	}
	ticket, err := s.WeChatTicket("component")
	if err != nil || ticket.IssuedAt != 7 {
		t.Fatalf("ticket: %#v, %v", ticket, err)
	}
}

func TestWeChatImportKeepsManualReleaseAndFinishesDraft(t *testing.T) {
	s := setupTestDB(t)
	p, item := linkedWeChatChannel(t, s)
	item = grantWeChatChannel(t, s, p, item)
	version := model.ProductVersion{ProductID: p.ID, Name: "0.4.0", Status: "completed", ReleasedOn: "2026-11-27", Notes: "原有版本说明"}
	if err := s.SaveProductVersion(p.OwnerID, &version); err != nil {
		t.Fatal(err)
	}
	manual := model.ProductDistribution{ProductID: p.ID, VersionID: version.ID, ChannelID: item.ChannelID, PlatformVersion: version.Name, Status: "published", Audience: "production", Notes: "原有发行说明"}
	draft := model.ProductDistribution{ProductID: p.ID, ChannelID: item.ChannelID, PlatformVersion: "0.5.0", Status: "approved", Audience: "production"}
	testing := model.ProductDistribution{ProductID: p.ID, ChannelID: item.ChannelID, PlatformVersion: "0.5.0", Status: "draft", Audience: "testing"}
	for _, row := range []*model.ProductDistribution{&manual, &draft, &testing} {
		if err := s.SaveProductDistribution(p.OwnerID, row); err != nil {
			t.Fatal(err)
		}
	}
	first := time.Date(2026, 11, 27, 2, 43, 28, 0, time.UTC)
	second := first.Add(20 * time.Hour)
	for _, snap := range []model.WeChatReleaseSnapshot{
		{OnlineVersion: "0.4.0", OnlineAt: &first, OnlineNotes: "微信说明"},
		{OnlineVersion: "0.5.0", OnlineAt: &second},
		{OnlineVersion: "0.5.0", OnlineAt: &second},
		{OnlineVersion: "0.4.0", OnlineAt: &first},
	} {
		if err := s.ImportWeChatRelease(item, snap, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	var old, promoted, testRecord model.ProductDistribution
	for index, id := range []uint{manual.ID, draft.ID, testing.ID} {
		if err := s.db.First([]*model.ProductDistribution{&old, &promoted, &testRecord}[index], id).Error; err != nil {
			t.Fatal(err)
		}
	}
	if old.Notes != manual.Notes || old.VersionID != version.ID || old.WeChatPublishedAt == nil || !old.WeChatPublishedAt.Equal(first) {
		t.Fatalf("manual release changed or duplicated: %#v", old)
	}
	if promoted.Status != "published" || promoted.VersionID == 0 || promoted.WeChatPublishedAt == nil || !promoted.WeChatPublishedAt.Equal(second) || testRecord.Status != "draft" {
		t.Fatalf("draft promotion: %#v, test: %#v", promoted, testRecord)
	}
	var newVersion model.ProductVersion
	if err := s.db.First(&newVersion, promoted.VersionID).Error; err != nil || newVersion.ReleasedOn != "2026-11-28" {
		t.Fatalf("Shanghai calendar date: %#v, %v", newVersion, err)
	}
	filter := model.ProductLifecycleFilter{Limit: 10}
	if _, n, err := s.ListProductDistributions(p.OwnerID, p.ID, filter); err != nil || n != 3 {
		t.Fatalf("distribution count: %d, %v", n, err)
	}
	if _, n, err := s.ListProductDistributionEvents(p.OwnerID, p.ID, manual.ID, filter); err != nil || n != 1 {
		t.Fatalf("manual events: %d, %v", n, err)
	}
	if events, n, err := s.ListProductDistributionEvents(p.OwnerID, p.ID, draft.ID, filter); err != nil || n != 2 || events[0].ActorID != 0 {
		t.Fatalf("automatic events: %#v, %d, %v", events, n, err)
	}
	got, err := s.WeChatConnection(p.OwnerID, p.ID, item.ChannelID)
	if err != nil || got.OnlineVersion != "0.5.0" {
		t.Fatalf("out-of-order release: %#v, %v", got, err)
	}
}

func TestWeChatImportRollsBackWhenHistoryWriteFails(t *testing.T) {
	s := setupTestDB(t)
	p, item := linkedWeChatChannel(t, s)
	item = grantWeChatChannel(t, s, p, item)
	if err := s.db.Exec(`CREATE TRIGGER reject_wechat_history BEFORE INSERT ON product_distribution_events
		BEGIN SELECT RAISE(ABORT, 'history unavailable'); END`).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := s.ImportWeChatRelease(item, model.WeChatReleaseSnapshot{OnlineVersion: "1.0", OnlineAt: &now}, now); err == nil {
		t.Fatal("expected history insert failure")
	}
	filter := model.ProductLifecycleFilter{Limit: 10}
	if _, n, err := s.ListProductVersions(p.OwnerID, p.ID, filter); err != nil || n != 0 {
		t.Fatalf("version count: %d, %v", n, err)
	}
	if _, n, err := s.ListProductDistributions(p.OwnerID, p.ID, filter); err != nil || n != 0 {
		t.Fatalf("distribution count: %d, %v", n, err)
	}
	connection, err := s.WeChatConnection(p.OwnerID, p.ID, item.ChannelID)
	if err != nil || connection.OnlineVersion != "" || connection.LastImportedAt != nil {
		t.Fatalf("rollback state: %#v, %v", connection, err)
	}
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
	if err := saveMetric(); !errors.Is(err, model.ErrWeChatAuthorization) {
		t.Fatalf("in-flight metrics repopulated disconnected channel: %v", err)
	}
}
