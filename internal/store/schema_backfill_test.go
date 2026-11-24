package store

import (
	"path/filepath"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type legacyAuditLog struct {
	ID           uint `gorm:"primaryKey"`
	Action       string
	ResourceType string
	ResourceID   uint
	UserID       uint
	Detail       string
}

func (legacyAuditLog) TableName() string { return "audit_logs" }

func TestBackfillLegacyAuditLogsMakesEventsReadable(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "audit-legacy.db")
	legacy, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.AutoMigrate(&legacyAuditLog{}); err != nil {
		t.Fatalf("create legacy audit schema: %v", err)
	}
	if err := legacy.Create(&legacyAuditLog{Action: "deploy", ResourceType: "application", ResourceID: 7, UserID: 3, Detail: `{"version":"1.2.3"}`}).Error; err != nil {
		t.Fatalf("create legacy audit event: %v", err)
	}

	migrated, err := New(dsn)
	if err != nil {
		t.Fatalf("migrate legacy audit schema: %v", err)
	}
	var event model.AuditLog
	if err := migrated.DB().First(&event, 1).Error; err != nil {
		t.Fatalf("load migrated event: %v", err)
	}
	if event.Source != "legacy" || event.Outcome != model.AuditOutcomeSucceeded || event.ActorType != model.AuditActorUser {
		t.Fatalf("legacy defaults not backfilled: %#v", event)
	}
	if event.Summary == "" || event.TargetName == "" {
		t.Fatalf("legacy event must have readable fallback fields: %#v", event)
	}
}

func TestBackfillRuntimeVersionsFromImageTags(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	items := []*model.RuntimeInstance{
		{Name: "nanobot-a", RuntimeType: "nanobot", Image: "cylism-nanobot-runtime:0.3.0", Namespace: "cylism-assistant", PVCName: "nanobot-a-data", Status: "draft"},
		{Name: "nanobot-b", RuntimeType: "nanobot", Image: "cylism-nanobot-runtime:latest", Namespace: "cylism-assistant", PVCName: "nanobot-b-data", Status: "draft"},
		{Name: "nanobot-c", RuntimeType: "nanobot", Image: "cylism-nanobot-runtime:1.2.3", Namespace: "cylism-assistant", PVCName: "nanobot-c-data", Status: "draft", RuntimeVersion: "fixed"},
	}
	for _, item := range items {
		if err := s.CreateRuntime(item); err != nil {
			t.Fatalf("create runtime: %v", err)
		}
	}
	if err := s.backfillRuntimeVersions(); err != nil {
		t.Fatalf("backfill versions: %v", err)
	}
	var a, b, c model.RuntimeInstance
	for name, target := range map[string]*model.RuntimeInstance{"nanobot-a": &a, "nanobot-b": &b, "nanobot-c": &c} {
		if err := s.db.Where("name = ?", name).First(target).Error; err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
	}
	if a.RuntimeVersion != "0.3.0" {
		t.Fatalf("image tag version not backfilled: %q", a.RuntimeVersion)
	}
	if b.RuntimeVersion != "" {
		t.Fatalf("latest tag must stay empty, got %q", b.RuntimeVersion)
	}
	if c.RuntimeVersion != "fixed" {
		t.Fatalf("explicit version must be preserved, got %q", c.RuntimeVersion)
	}
}

func TestBackfillClusterTailscaleIPsRestoresServerAddress(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	server := model.Server{Name: "control", Host: "203.0.113.10"}
	if err := s.DB().Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	legacy := model.Cluster{Name: "legacy-tailscale", Distribution: "k3s", Status: "ready", NetworkMode: "tailscale", ControlServerID: &server.ID, ControlAddress: "100.81.23.123"}
	public := model.Cluster{Name: "public", Distribution: "k3s", Status: "ready", NetworkMode: "public", ControlServerID: &server.ID, ControlAddress: "198.51.100.20"}
	for _, cluster := range []*model.Cluster{&legacy, &public} {
		if err := s.DB().Create(cluster).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DB().Where("key = ?", clusterTailscaleIPBackfillKey).Delete(&model.SystemConfig{}).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := s.backfillClusterTailscaleIPs(); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.GetCluster(legacy.ID)
	if err != nil || got.ControlAddress != server.Host || got.TailscaleIP != "100.81.23.123" {
		t.Fatalf("legacy cluster = %#v, %v", got, err)
	}
	got, err = s.GetCluster(public.ID)
	if err != nil || got.ControlAddress != "198.51.100.20" || got.TailscaleIP != "" {
		t.Fatalf("public cluster = %#v, %v", got, err)
	}
}
