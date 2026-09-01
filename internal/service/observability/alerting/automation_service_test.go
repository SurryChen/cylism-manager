package alerting

import (
	"context"
	"github.com/cylism/cylism-manager/internal/model"
	"testing"
)

type automationRepoFake struct {
	policy  *model.AlertAutomationPolicy
	runtime *model.RuntimeInstance
	saved   bool
}

func (f *automationRepoFake) UpsertAlertEvent(e *model.AlertEvent) (*model.AlertEvent, error) {
	return e, nil
}
func (f *automationRepoFake) UpdateAlertEvent(*model.AlertEvent) error { return nil }
func (f *automationRepoFake) GetAlertAutomationPolicy() (*model.AlertAutomationPolicy, error) {
	return f.policy, nil
}
func (f *automationRepoFake) SaveAlertAutomationPolicy(p *model.AlertAutomationPolicy) error {
	f.policy = p
	f.saved = true
	return nil
}
func (f *automationRepoFake) GetRuntime(uint) (*model.RuntimeInstance, error) { return f.runtime, nil }

func TestAutomationServiceUpdatePolicyValidatesRuntimeAndPersists(t *testing.T) {
	f := &automationRepoFake{runtime: &model.RuntimeInstance{RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged}}
	s := NewAutomationService(f, nil)
	p := &model.AlertAutomationPolicy{RuntimeID: 1, Enabled: true, MinimumSeverity: "warning", Mode: model.AlertAutomationReportOnly, CooldownMinutes: 30}
	count, err := s.UpdatePolicy(context.Background(), p, func(context.Context) (int, error) { return 2, nil })
	if err != nil || count != 2 || !f.saved {
		t.Fatalf("unexpected update: count=%d err=%v saved=%v", count, err, f.saved)
	}
}
