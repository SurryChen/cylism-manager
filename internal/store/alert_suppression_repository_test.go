package store

import (
	"errors"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func TestSuppressionRulesPersistScopeDeadlineAndExplicitFalseValues(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	ctx, now := t.Context(), time.Now().UTC()
	end := now.Add(time.Hour)
	rule := model.AlertSuppressionRule{ClusterID: 2, Name: "cloudcone", NodeName: "cloudcone", AlertNames: []string{"NodeCPUHigh", "NodeMemoryHigh"}, Enabled: true, StartsAt: now, EndsAt: &end}
	if err := s.CreateAlertSuppressionRule(ctx, &rule); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlertSuppressionEnabled(ctx, 3, rule.ID, false); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-cluster toggle: %v", err)
	}
	if err := s.DeleteAlertSuppressionRule(ctx, 3, rule.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-cluster delete: %v", err)
	}
	if err := s.UpdateAlertSuppressionRule(ctx, 3, rule.ID, &rule, true); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-cluster update: %v", err)
	}
	items, err := s.ActiveAlertSuppressionRules(ctx, 2, now.Add(time.Minute))
	if err != nil || len(items) != 1 || len(items[0].AlertNames) != 2 {
		t.Fatalf("persisted JSON: %+v %v", items, err)
	}
	// Editing content must preserve deadline, while accepting empty strings,
	// an empty type list, and false for enabled.
	edit := model.AlertSuppressionRule{Name: "all types on node", NodeName: "cloudcone", AlertNames: []string{}, Enabled: false}
	if err := s.UpdateAlertSuppressionRule(ctx, 2, rule.ID, &edit, false); err != nil {
		t.Fatal(err)
	}
	items, total, err := s.ListAlertSuppressionRules(ctx, model.AlertSuppressionFilter{ClusterID: 2, Now: now, Status: "disabled", Limit: 10})
	if err != nil || total != 1 || len(items) != 1 || items[0].Enabled || len(items[0].AlertNames) != 0 || items[0].EndsAt == nil || !items[0].EndsAt.Equal(end) {
		t.Fatalf("update preserving deadline: %+v %d %v", items, total, err)
	}
	if err := s.SetAlertSuppressionEnabled(ctx, 2, rule.ID, true); err != nil {
		t.Fatal(err)
	}
	items, err = s.ActiveAlertSuppressionRules(ctx, 2, end)
	if err != nil || len(items) != 0 {
		t.Fatalf("expires at boundary: %+v %v", items, err)
	}
	edit.Enabled, edit.StartsAt, edit.EndsAt = true, now, nil
	if err := s.UpdateAlertSuppressionRule(ctx, 2, rule.ID, &edit, true); err != nil {
		t.Fatal(err)
	}
	items, err = s.ActiveAlertSuppressionRules(ctx, 2, end.Add(365*24*time.Hour))
	if err != nil || len(items) != 1 || items[0].EndsAt != nil {
		t.Fatalf("permanent deadline reset: %+v %v", items, err)
	}
	items, err = s.ActiveAlertSuppressionRules(ctx, 3, now)
	if err != nil || len(items) != 0 {
		t.Fatalf("cluster isolation: %+v %v", items, err)
	}
	if err := s.DeleteAlertSuppressionRule(ctx, 2, rule.ID); err != nil {
		t.Fatal(err)
	}
	items, total, err = s.ListAlertSuppressionRules(ctx, model.AlertSuppressionFilter{ClusterID: 2, Now: now})
	if err != nil || total != 0 || items == nil || len(items) != 0 {
		t.Fatalf("delete: %+v %d %v", items, total, err)
	}
}

func TestSuppressionRulesListFiltersAndPagination(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	past := now.Add(-time.Minute)
	for _, rule := range []model.AlertSuppressionRule{
		{ClusterID: 2, Name: "active", NodeName: "cloudcone", Enabled: true, StartsAt: past},
		{ClusterID: 2, Name: "disabled", NodeName: "cloudcone", Enabled: false, StartsAt: past},
		{ClusterID: 2, Name: "expired", NodeName: "cloudcone", Enabled: true, StartsAt: past.Add(-time.Hour), EndsAt: &past},
		{ClusterID: 3, Name: "other cluster", NodeName: "cloudcone", Enabled: true, StartsAt: past},
	} {
		if err := s.CreateAlertSuppressionRule(t.Context(), &rule); err != nil {
			t.Fatal(err)
		}
	}
	for _, status := range []string{"active", "disabled", "expired"} {
		items, total, err := s.ListAlertSuppressionRules(t.Context(), model.AlertSuppressionFilter{ClusterID: 2, NodeName: "cloudcone", Status: status, Now: now, Limit: 1})
		if err != nil || total != 1 || len(items) != 1 || items[0].Name != status {
			t.Fatalf("%s filter: %+v %d %v", status, items, total, err)
		}
	}
	items, total, err := s.ListAlertSuppressionRules(t.Context(), model.AlertSuppressionFilter{ClusterID: 2, Now: now, Limit: 1, Offset: 1})
	if err != nil || total != 3 || len(items) != 1 {
		t.Fatalf("page: %+v %d %v", items, total, err)
	}
}
