package store

import (
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func TestSystemComponentConfigCRUD(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	config := &model.SystemComponentConfig{
		ChartName:     "coredns",
		Namespace:     "kube-system",
		ValuesContent: "replicas: 2\nmaxUnavailable: 0\n",
		Enabled:       true,
		ApplyStatus:   "succeeded",
		CreatedBy:     1,
	}
	if err := s.UpsertSystemComponentConfig(config); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if config.ID == 0 {
		t.Fatal("expected generated id")
	}
	loaded, err := s.GetSystemComponentConfig("coredns")
	if err != nil || loaded.ValuesContent != config.ValuesContent {
		t.Fatalf("unexpected loaded config: %#v err=%v", loaded, err)
	}
	config.ValuesContent = "replicas: 1\n"
	if err := s.UpsertSystemComponentConfig(config); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	loaded, _ = s.GetSystemComponentConfig("coredns")
	if loaded.ValuesContent != "replicas: 1\n" || loaded.ID != config.ID {
		t.Fatalf("update must keep identity and replace values: %#v", loaded)
	}
	all, err := s.ListSystemComponentConfigs()
	if err != nil || len(all) != 1 {
		t.Fatalf("unexpected list: %#v err=%v", all, err)
	}
	if err := s.DeleteSystemComponentConfig("coredns"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetSystemComponentConfig("coredns"); err != gorm.ErrRecordNotFound {
		t.Fatalf("expected record not found, got %v", err)
	}
}
