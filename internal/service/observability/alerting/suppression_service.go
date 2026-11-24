package alerting

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cylism/cylism-manager/internal/model"
)

type SuppressionStore interface {
	ListAlertSuppressionRules(context.Context, model.AlertSuppressionFilter) ([]model.AlertSuppressionRule, int64, error)
	ActiveAlertSuppressionRules(context.Context, uint, time.Time) ([]model.AlertSuppressionRule, error)
	CreateAlertSuppressionRule(context.Context, *model.AlertSuppressionRule) error
	UpdateAlertSuppressionRule(context.Context, uint, uint, *model.AlertSuppressionRule, bool) error
	SetAlertSuppressionEnabled(context.Context, uint, uint, bool) error
	DeleteAlertSuppressionRule(context.Context, uint, uint) error
}

func ValidateSuppressionRule(rule *model.AlertSuppressionRule) error {
	rule.Name, rule.NodeName, rule.Comment = strings.TrimSpace(rule.Name), strings.TrimSpace(rule.NodeName), strings.TrimSpace(rule.Comment)
	if rule.Name == "" || utf8.RuneCountInString(rule.Name) > 100 || len(rule.NodeName) > 253 || utf8.RuneCountInString(rule.Comment) > 300 || len(rule.AlertNames) > 20 {
		return fmt.Errorf("规则名称必填且不超过 100 字，节点不超过 253 字节，说明不超过 300 字，类型不超过 20 项")
	}
	names := make([]string, 0, len(rule.AlertNames))
	seen := map[string]bool{}
	for _, name := range rule.AlertNames {
		name = strings.TrimSpace(name)
		if name == "" || len(name) > 128 {
			return fmt.Errorf("告警类型不能为空且不超过 128 字节")
		}
		if !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	rule.AlertNames = names
	if rule.NodeName == "" && len(names) == 0 {
		return fmt.Errorf("至少指定节点或一种告警类型，不能屏蔽整个集群的全部通知")
	}
	return nil
}

// ApplyNotificationSuppression marks every alert for durable history while
// returning only notifications that should be delivered. Automation is unchanged.
func ApplyNotificationSuppression(payload AlertNotification, rules []model.AlertSuppressionRule, now time.Time) (AlertNotification, AlertNotification, int) {
	all, deliver := payload, payload
	all.Alerts, deliver.Alerts = make([]Alert, 0, len(payload.Alerts)), make([]Alert, 0, len(payload.Alerts))
	suppressed := 0
	for _, alert := range payload.Alerts {
		alert.NotificationSuppressed = false
		for _, rule := range rules {
			if !rule.Enabled || rule.StartsAt.After(now) || (rule.EndsAt != nil && !rule.EndsAt.After(now)) {
				continue
			}
			if rule.NodeName == "" && len(rule.AlertNames) == 0 {
				continue
			}
			if rule.NodeName != "" && rule.NodeName != alert.Labels["node"] {
				continue
			}
			matches := len(rule.AlertNames) == 0
			for _, name := range rule.AlertNames {
				if name == alert.Labels["alertname"] {
					matches = true
					break
				}
			}
			if matches {
				alert.NotificationSuppressed = true
				break
			}
		}
		all.Alerts = append(all.Alerts, alert)
		if alert.NotificationSuppressed {
			suppressed++
		} else {
			deliver.Alerts = append(deliver.Alerts, alert)
		}
	}
	// Removing firing alerts from a mixed group can leave only recovery notices.
	if len(deliver.Alerts) > 0 {
		resolved := true
		for _, alert := range deliver.Alerts {
			if !strings.EqualFold(alert.Status.State, "resolved") && !strings.EqualFold(payload.Status, "resolved") {
				resolved = false
				break
			}
		}
		if resolved {
			deliver.Status = "resolved"
		}
	}
	return all, deliver, suppressed
}
