package alerting

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
)

// NotificationSender owns delivery transports and presentation rendering for
// alert notifications. HTTP handlers should not construct channel payloads.
type NotificationSender interface {
	Send(context.Context, NotificationSecrets, AlertNotification) error
	SendTest(context.Context, NotificationSecrets, AlertNotification, string) error
}

type DefaultNotificationSender struct{}

func (DefaultNotificationSender) Send(ctx context.Context, secrets NotificationSecrets, payload AlertNotification) error {
	return Dispatch(ctx, secrets, payload,
		func(ctx context.Context, endpoint string, value interface{}) error {
			return SendFeishuJSON(ctx, endpoint, value)
		},
		func(ctx context.Context, config EmailConfig, value AlertNotification) error {
			return sendEmail(ctx, config, value, payload.PlatformURL)
		})
}

func (DefaultNotificationSender) SendTest(ctx context.Context, secrets NotificationSecrets, payload AlertNotification, channel string) error {
	return TestDispatch(ctx, secrets, payload, strings.ToLower(strings.TrimSpace(channel)),
		func(ctx context.Context, endpoint string, value interface{}) error {
			return SendFeishuJSON(ctx, endpoint, value)
		},
		func(ctx context.Context, config EmailConfig, value AlertNotification) error {
			return sendEmail(ctx, config, value, payload.PlatformURL)
		})
}

func sendEmail(ctx context.Context, config EmailConfig, payload AlertNotification, platformURL string) error {
	from, err := mail.ParseAddress(config.From)
	if err != nil {
		return fmt.Errorf("发件人地址无效: %w", err)
	}
	recipients, err := mail.ParseAddressList(config.To)
	if err != nil || len(recipients) == 0 {
		return fmt.Errorf("收件人地址无效")
	}
	addresses := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		addresses = append(addresses, recipient.Address)
	}
	plainBody := BuildEmailText(payload, platformURL)
	mimeMessage := BuildEmailMIME(from.String(), strings.Join(addresses, ", "), payload, platformURL)
	return SendSMTP(ctx, config, plainBody, mimeMessage)
}
