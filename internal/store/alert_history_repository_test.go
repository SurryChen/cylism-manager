package store

import (
	"context"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	alertingservice "github.com/cylism/cylism-manager/internal/service/observability/alerting"
)

func TestAlertHistoryOccurrencesDeduplicateAndIsolateClusters(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	start := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	alert := alertingservice.Alert{StartsAt: start, Fingerprint: "disk-cloudcone", Status: alertingservice.AlertStatus{State: "firing"},
		Labels: map[string]string{"alertname": "NodeDiskHigh", "node": "cloudcone", "severity": "warning"}, Annotations: map[string]string{"rule_name": "根磁盘空间不足", "summary": "磁盘使用率过高"}}
	record := func(cluster uint, a alertingservice.Alert) {
		t.Helper()
		if err := alertingservice.RecordHistory(ctx, s, cluster, alertingservice.AlertNotification{Alerts: []alertingservice.Alert{a}}); err != nil {
			t.Fatal(err)
		}
	}
	record(2, alert)
	record(2, alert) // Alertmanager retry / repeat notification.
	record(3, alert) // The same fingerprint in a different cluster is independent.
	resolved := alert
	resolved.Status.State, resolved.EndsAt = "resolved", start.Add(10*time.Minute)
	record(2, resolved)
	record(2, alert) // Late firing retry must not reopen the occurrence.
	alert.StartsAt = start.Add(30 * time.Minute)
	record(2, alert) // A recurrence must count separately.
	filter := model.AlertHistoryFilter{ClusterID: 2, Since: start.Add(-time.Minute), Limit: 10}
	items, total, err := s.ListAlertHistory(ctx, filter)
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("history: %+v total=%d err=%v", items, total, err)
	}
	if items[0].Status != "firing" || items[1].Status != "resolved" || items[1].EndsAt == nil || !items[1].EndsAt.Equal(resolved.EndsAt) {
		t.Fatalf("unexpected lifecycle: %+v", items)
	}
	groups, count, err := s.ListAlertFrequency(ctx, filter)
	if err != nil || count != 1 || len(groups) != 1 || groups[0].Count != 2 || groups[0].NodeName != "cloudcone" {
		t.Fatalf("frequency: %+v total=%d err=%v", groups, count, err)
	}
	last, err := time.Parse(time.RFC3339Nano, groups[0].LastSeen)
	if err != nil || !last.Equal(alert.StartsAt) {
		t.Fatalf("last seen = %q, want %v: %v", groups[0].LastSeen, alert.StartsAt, err)
	}
	filter.ClusterID = 3
	items, total, err = s.ListAlertHistory(ctx, filter)
	if err != nil || total != 1 || len(items) != 1 || items[0].Status != "firing" {
		t.Fatalf("other cluster: %+v total=%d err=%v", items, total, err)
	}
}

func TestAlertHistoryFiltersFrequencyAndPagination(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	ctx, now := context.Background(), time.Now().UTC()
	for _, record := range []model.AlertHistory{
		{ClusterID: 2, Fingerprint: "disk", StartsAt: now.Add(-time.Hour), NodeName: "cloudcone", AlertName: "NodeDiskHigh", Status: "firing"},
		{ClusterID: 2, Fingerprint: "disk", StartsAt: now.Add(-2 * time.Hour), NodeName: "cloudcone", AlertName: "NodeDiskHigh", Status: "resolved"},
		{ClusterID: 2, Fingerprint: "cpu", StartsAt: now.Add(-time.Hour), NodeName: "cloudcone", AlertName: "NodeCPUHigh", Status: "firing"},
		{ClusterID: 2, Fingerprint: "cpu-other", StartsAt: now.Add(-time.Hour), NodeName: "other", AlertName: "NodeCPUHigh", Status: "firing"},
		{ClusterID: 2, Fingerprint: "old", StartsAt: now.Add(-40 * 24 * time.Hour), NodeName: "cloudcone", AlertName: "NodeDown", Status: "resolved"},
	} {
		if err := s.RecordAlertHistory(ctx, []model.AlertHistory{record}); err != nil {
			t.Fatal(err)
		}
	}
	f := model.AlertHistoryFilter{ClusterID: 2, Since: now.Add(-24 * time.Hour), NodeName: "cloudcone", Limit: 1}
	groups, total, err := s.ListAlertFrequency(ctx, f)
	if err != nil || total != 2 || len(groups) != 1 || groups[0].AlertName != "NodeDiskHigh" || groups[0].Count != 2 {
		t.Fatalf("first group page: %+v %d %v", groups, total, err)
	}
	f.Offset = 1
	groups, total, err = s.ListAlertFrequency(ctx, f)
	if err != nil || total != 2 || len(groups) != 1 || groups[0].AlertName != "NodeCPUHigh" {
		t.Fatalf("second group page: %+v %d %v", groups, total, err)
	}
	f.AlertName, f.Status, f.Offset = "NodeDiskHigh", "resolved", 0
	items, total, err := s.ListAlertHistory(ctx, f)
	if err != nil || total != 1 || len(items) != 1 || items[0].Status != "resolved" {
		t.Fatalf("filtered history: %+v %d %v", items, total, err)
	}
	f.Offset = 10
	items, total, err = s.ListAlertHistory(ctx, f)
	if err != nil || total != 1 || items == nil || len(items) != 0 {
		t.Fatalf("empty page: %+v %d %v", items, total, err)
	}
}
