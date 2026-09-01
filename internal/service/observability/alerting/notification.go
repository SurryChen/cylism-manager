package alerting

import (
	"context"
	"fmt"
	"strings"
)

type Notification struct {
	FeishuWebhook string
	EmailEnabled  bool
}
type Sender interface {
	Feishu(context.Context, string, interface{}) error
	Email(context.Context, interface{}, interface{}) error
}

func Dispatch(ctx context.Context, secrets NotificationSecrets, payload AlertNotification, feishu func(context.Context, string, interface{}) error, email func(context.Context, EmailConfig, AlertNotification) error) error {
	return Send(ctx, Notification{FeishuWebhook: secrets.FeishuWebhookURL, EmailEnabled: secrets.Email.Enabled}, payload,
		func(ctx context.Context, endpoint string, value interface{}) error {
			return feishu(ctx, endpoint, BuildFeishuCard(payload, payload.PlatformURL))
		},
		func(ctx context.Context, _ interface{}, value interface{}) error {
			return email(ctx, secrets.Email, payload)
		})
}

func TestDispatch(ctx context.Context, secrets NotificationSecrets, payload AlertNotification, channel string, feishu func(context.Context, string, interface{}) error, email func(context.Context, EmailConfig, AlertNotification) error) error {
	switch channel {
	case "feishu":
		if secrets.FeishuWebhookURL == "" {
			return fmt.Errorf("飞书通知渠道尚未配置")
		}
		if err := feishu(ctx, secrets.FeishuWebhookURL, BuildFeishuCard(payload, payload.PlatformURL)); err != nil {
			return fmt.Errorf("飞书: %w", err)
		}
		return nil
	case "email":
		if !secrets.Email.Enabled {
			return fmt.Errorf("邮件通知渠道尚未配置")
		}
		if err := email(ctx, secrets.Email, payload); err != nil {
			return fmt.Errorf("邮件: %w", err)
		}
		return nil
	default:
		return Dispatch(ctx, secrets, payload, feishu, email)
	}
}

func Send(ctx context.Context, settings Notification, payload interface{}, feishu func(context.Context, string, interface{}) error, email func(context.Context, interface{}, interface{}) error) error {
	var failures []string
	if settings.FeishuWebhook != "" {
		if err := feishu(ctx, settings.FeishuWebhook, payload); err != nil {
			failures = append(failures, "飞书: "+err.Error())
		}
	}
	if settings.EmailEnabled {
		if err := email(ctx, nil, payload); err != nil {
			failures = append(failures, "邮件: "+err.Error())
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil
}
