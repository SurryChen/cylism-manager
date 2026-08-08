package store

import (
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

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
