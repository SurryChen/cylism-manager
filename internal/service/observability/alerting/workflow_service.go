package alerting

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

// Workflow contains the stateful Alertmanager webhook and notification
// orchestration. HTTP handlers only translate requests into these types.
type Workflow struct {
	Client     *Client
	Ready      func() bool
	Automation *AutomationService
	Store      interface {
		EventStore
		ListAlertEvents(int) ([]model.AlertEvent, error)
	}
	Dispatcher  Dispatcher
	Cache       *ResolvedCache
	PlatformURL string
	Feishu      func(context.Context, string, AlertNotification) error
	Email       func(context.Context, EmailConfig, AlertNotification) error
	Secrets     SecretReader
	Sender      NotificationSender
}

func (w *Workflow) LoadConfiguredSecrets(ctx context.Context, namespace, name string) (NotificationSecrets, error) {
	if w == nil {
		return NotificationSecrets{}, fmt.Errorf("读取告警通知配置失败")
	}
	return LoadSecrets(ctx, w.Secrets, namespace, name)
}

// Query methods keep Alertmanager protocol and readiness checks in the
// workflow boundary so HTTP handlers only map request/response DTOs.
func (w *Workflow) query() *QueryService {
	if w == nil {
		return nil
	}
	return NewQueryService(w.Client, w.Ready)
}

func (w *Workflow) Overview(ctx context.Context) (Overview, error) {
	if err := w.EnsureReady(); err != nil {
		return Overview{}, err
	}
	return w.query().Overview(ctx, w.RecentResolved())
}

func (w *Workflow) Silences(ctx context.Context) ([]Silence, error) {
	if err := w.EnsureReady(); err != nil {
		return nil, err
	}
	return w.query().Silences(ctx)
}

func (w *Workflow) CreateSilence(ctx context.Context, req SilenceRequest, now time.Time) (SilenceResult, time.Time, error) {
	if err := w.EnsureReady(); err != nil {
		return SilenceResult{}, time.Time{}, err
	}
	return w.query().CreateSilence(ctx, req, now)
}

func (w *Workflow) DeleteSilence(ctx context.Context, id string) error {
	if err := w.EnsureReady(); err != nil {
		return err
	}
	return w.query().DeleteSilence(ctx, id)
}

func (w *Workflow) LoadSecrets(ctx context.Context, reader SecretReader, namespace, name string) (NotificationSecrets, error) {
	return LoadSecrets(ctx, reader, namespace, name)
}

func (w *Workflow) ValidateNotification(payload AlertNotification) error {
	if len(payload.Alerts) == 0 || len(payload.Alerts) > 64 {
		return ErrInvalidNotification
	}
	return nil
}

func (w *Workflow) Policy() (*model.AlertAutomationPolicy, error) {
	if w == nil || w.Automation == nil {
		return nil, fmt.Errorf("告警自动化不可用")
	}
	return w.Automation.Policy()
}

func (w *Workflow) UpdatePolicy(ctx context.Context, policy *model.AlertAutomationPolicy) (int, error) {
	if w == nil || w.Automation == nil {
		return 0, fmt.Errorf("告警自动化不可用")
	}
	return w.Automation.UpdatePolicy(ctx, policy, w.SyncCurrent)
}

func (w *Workflow) EnsureReady() error {
	if w == nil || w.Client == nil {
		return fmt.Errorf("Alertmanager 查询不可用")
	}
	if w.Ready != nil && !w.Ready() {
		return fmt.Errorf("Alertmanager 尚未就绪")
	}
	return nil
}

func (w *Workflow) Persist(ctx context.Context, payload AlertNotification, force bool) (int, error) {
	if w == nil {
		return 0, nil
	}
	return (EventWorkflow{Automation: w.Automation, Store: w.Store, Dispatcher: w.Dispatcher}).Persist(ctx, payload, force)
}

func (w *Workflow) SyncCurrent(ctx context.Context) (int, error) {
	if err := w.EnsureReady(); err != nil {
		return 0, err
	}
	var alerts []Alert
	if err := w.Client.Request(ctx, http.MethodGet, "/api/v2/alerts", nil, &alerts); err != nil {
		return 0, fmt.Errorf("读取 Alertmanager 活跃告警失败: %w", err)
	}
	firing := make([]Alert, 0, len(alerts))
	for _, alert := range alerts {
		if IsFiring(alert) {
			firing = append(firing, alert)
		}
	}
	if len(firing) == 0 {
		return 0, nil
	}
	return w.Persist(ctx, AlertNotification{Status: "firing", Alerts: firing}, true)
}

func (w *Workflow) ListEvents(limit int) ([]model.AlertEvent, error) {
	if w == nil || w.Store == nil {
		return nil, fmt.Errorf("告警自动化不可用")
	}
	return w.Store.ListAlertEvents(limit)
}

func (w *Workflow) RecordResolved(payload AlertNotification) {
	if w == nil || w.Cache == nil {
		return
	}
	resolved := make([]Alert, 0, len(payload.Alerts))
	for _, alert := range payload.Alerts {
		if payload.Status != "resolved" && alert.Status.State != "resolved" {
			continue
		}
		if alert.EndsAt.IsZero() {
			alert.EndsAt = time.Now().UTC()
		}
		alert.Status.State = "resolved"
		resolved = append(resolved, alert)
	}
	w.Cache.Add(resolved)
}

func (w *Workflow) RecentResolved() []Alert {
	if w == nil || w.Cache == nil {
		return nil
	}
	return w.Cache.List()
}

func ValidatePolicy(policy *model.AlertAutomationPolicy) bool {
	return policy != nil && len(policy.AlertName) <= 128 && policy.CooldownMinutes >= 5 && policy.CooldownMinutes <= 24*60 && ValidPolicy(policy)
}

func (w *Workflow) Send(ctx context.Context, secrets NotificationSecrets, payload AlertNotification) error {
	if w == nil {
		return fmt.Errorf("告警通知不可用")
	}
	if w.Sender != nil {
		return w.Sender.Send(ctx, secrets, payload)
	}
	return Dispatch(ctx, secrets, payload, func(ctx context.Context, endpoint string, _ interface{}) error {
		if w.Feishu == nil {
			return fmt.Errorf("飞书通知发送不可用")
		}
		return w.Feishu(ctx, endpoint, payload)
	}, func(ctx context.Context, config EmailConfig, value AlertNotification) error {
		if w.Email == nil {
			return fmt.Errorf("邮件通知发送不可用")
		}
		return w.Email(ctx, config, value)
	})
}

func (w *Workflow) SendTest(ctx context.Context, secrets NotificationSecrets, payload AlertNotification, channel string) error {
	if w == nil {
		return fmt.Errorf("告警通知不可用")
	}
	if w.Sender != nil {
		return w.Sender.SendTest(ctx, secrets, payload, channel)
	}
	return TestDispatch(ctx, secrets, payload, strings.ToLower(strings.TrimSpace(channel)), func(ctx context.Context, endpoint string, _ interface{}) error {
		if w.Feishu == nil {
			return fmt.Errorf("飞书通知发送不可用")
		}
		return w.Feishu(ctx, endpoint, payload)
	}, func(ctx context.Context, config EmailConfig, value AlertNotification) error {
		if w.Email == nil {
			return fmt.Errorf("邮件通知发送不可用")
		}
		return w.Email(ctx, config, value)
	})
}

func TestPayload(platformURL string, now time.Time) AlertNotification {
	return AlertNotification{Status: "firing", Alerts: []Alert{{Status: AlertStatus{State: "firing"}, Labels: map[string]string{"alertname": "CylismAlertingTest", "node": "示例节点", "severity": "warning"}, Annotations: map[string]string{"summary": "测试消息使用真实告警的完整卡片布局", "description": "模拟节点根磁盘使用率超过阈值", "rule_name": "节点根磁盘使用率过高（测试）", "current_value": "92.4%", "threshold": "85%", "duration": "10 分钟"}, StartsAt: now.UTC()}}, PlatformURL: platformURL}
}
