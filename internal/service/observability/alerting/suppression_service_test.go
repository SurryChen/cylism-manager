package alerting

import (
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestSuppressionMatchesNodeAndAnySelectedType(t *testing.T) {
	now := time.Now().UTC()
	end := now.Add(time.Hour)
	rules := []model.AlertSuppressionRule{{Enabled: true, StartsAt: now.Add(-time.Minute), EndsAt: &end, NodeName: "cloudcone", AlertNames: []string{"NodeCPUHigh", "NodeMemoryHigh"}}}
	payload := AlertNotification{Status: "firing", PlatformURL: "https://example.com", Alerts: []Alert{
		{Labels: map[string]string{"node": "cloudcone", "alertname": "NodeCPUHigh"}, Status: AlertStatus{State: "firing"}},
		{Labels: map[string]string{"node": "cloudcone", "alertname": "NodeMemoryHigh"}, Status: AlertStatus{State: "resolved"}},
		{Labels: map[string]string{"node": "other", "alertname": "NodeCPUHigh"}, Status: AlertStatus{State: "resolved"}},
		{Labels: map[string]string{"node": "cloudcone", "alertname": "NodeDown"}, Status: AlertStatus{State: "firing"}},
	}}
	all, delivery, count := ApplyNotificationSuppression(payload, rules, now)
	if count != 2 || len(all.Alerts) != 4 || len(delivery.Alerts) != 2 || delivery.Alerts[0].Labels["node"] != "other" || delivery.PlatformURL != payload.PlatformURL || delivery.Status != "firing" {
		t.Fatalf("unexpected partial suppression: %+v %+v %d", all, delivery, count)
	}
	if !all.Alerts[0].NotificationSuppressed || !all.Alerts[1].NotificationSuppressed || all.Alerts[2].NotificationSuppressed || payload.Alerts[0].NotificationSuppressed {
		t.Fatal("suppression should annotate a copy of history, preserving input")
	}
	for _, tc := range []struct {
		name  string
		rule  model.AlertSuppressionRule
		at    time.Time
		count int
	}{
		{"disabled", model.AlertSuppressionRule{NodeName: "cloudcone"}, now, 0},
		{"expired", rules[0], end, 0},
		{"future", model.AlertSuppressionRule{Enabled: true, NodeName: "cloudcone", StartsAt: end}, now, 0},
		{"all-types-on-node", model.AlertSuppressionRule{Enabled: true, NodeName: "cloudcone"}, now, 3},
		{"type-on-all-nodes", model.AlertSuppressionRule{Enabled: true, AlertNames: []string{"NodeCPUHigh"}}, now, 2},
		{"reject-match-everything", model.AlertSuppressionRule{Enabled: true}, now, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, count := ApplyNotificationSuppression(payload, []model.AlertSuppressionRule{tc.rule}, tc.at)
			if count != tc.count {
				t.Fatalf("suppressed %d, want %d", count, tc.count)
			}
		})
	}
	_, delivery, _ = ApplyNotificationSuppression(AlertNotification{Status: "firing", Alerts: payload.Alerts[:3]}, rules, now)
	if delivery.Status != "resolved" || len(delivery.Alerts) != 1 {
		t.Fatalf("filtered mixed group must become recovery notice: %+v", delivery)
	}
}

func TestValidateSuppressionRule(t *testing.T) {
	for _, rule := range []model.AlertSuppressionRule{{Name: ""}, {Name: "all"}, {Name: "invalid", NodeName: "node", AlertNames: []string{" "}}} {
		if ValidateSuppressionRule(&rule) == nil {
			t.Fatalf("accepted invalid rule: %+v", rule)
		}
	}
	rule := model.AlertSuppressionRule{Name: " cloudcone ", NodeName: " cloudcone ", AlertNames: []string{" NodeCPUHigh ", "NodeCPUHigh"}}
	if err := ValidateSuppressionRule(&rule); err != nil || rule.Name != "cloudcone" || len(rule.AlertNames) != 1 {
		t.Fatalf("normalization: %+v %v", rule, err)
	}
}
