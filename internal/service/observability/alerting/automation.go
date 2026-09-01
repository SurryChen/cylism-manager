package alerting

import (
	"github.com/cylism/cylism-manager/internal/model"
	"strings"
	"time"
)

func ValidPolicy(policy *model.AlertAutomationPolicy) bool {
	if policy == nil || len(policy.AlertName) > 128 || policy.CooldownMinutes < 5 || policy.CooldownMinutes > 24*60 {
		return false
	}
	if policy.Mode != model.AlertAutomationReportOnly && policy.Mode != model.AlertAutomationApproval {
		return false
	}
	return policy.MinimumSeverity == "warning" || policy.MinimumSeverity == "critical"
}
func Matches(policy *model.AlertAutomationPolicy, event *model.AlertEvent) bool {
	if policy == nil || event == nil || !policy.Enabled || policy.AlertName != "" && policy.AlertName != event.AlertName {
		return false
	}
	return severityRank(event.Severity) >= severityRank(policy.MinimumSeverity)
}
func ShouldDispatch(policy *model.AlertAutomationPolicy, event *model.AlertEvent, force bool, now time.Time) bool {
	if !Matches(policy, event) || event.Status == model.AlertEventAwaitingApproval || event.Status == model.AlertEventRemediating {
		return false
	}
	if force || event.LastDispatchedAt == nil {
		return true
	}
	return now.Sub(*event.LastDispatchedAt) >= time.Duration(policy.CooldownMinutes)*time.Minute
}
func severityRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical":
		return 2
	case "warning":
		return 1
	default:
		return 0
	}
}
