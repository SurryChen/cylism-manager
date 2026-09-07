package alerting

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type SecretReader interface {
	GetSecretDataContext(context.Context, string, string) (map[string][]byte, error)
}

type EmailConfig struct {
	Enabled                               bool
	SMTPHost                              string
	SMTPPort                              int
	Username, Password, From, To, TLSMode string
}

type NotificationSecrets struct {
	FeishuWebhookURL string
	RelayToken       string
	Email            EmailConfig
}

func (s NotificationSecrets) Configured() bool { return s.FeishuWebhookURL != "" || s.Email.Enabled }

func LoadSecrets(ctx context.Context, reader SecretReader, namespace, name string) (NotificationSecrets, error) {
	if reader == nil {
		return NotificationSecrets{}, fmt.Errorf("读取告警通知配置失败")
	}
	data, err := reader.GetSecretDataContext(ctx, namespace, name)
	if err != nil {
		return NotificationSecrets{}, fmt.Errorf("读取告警通知配置失败")
	}
	result := NotificationSecrets{FeishuWebhookURL: strings.TrimSpace(string(data["feishu-webhook-url"])), RelayToken: strings.TrimSpace(string(data["relay-token"]))}
	if result.RelayToken == "" {
		return NotificationSecrets{}, fmt.Errorf("告警回调令牌尚未配置")
	}
	if result.FeishuWebhookURL != "" {
		u, parseErr := url.Parse(result.FeishuWebhookURL)
		if parseErr != nil || u.Scheme != "https" || u.Hostname() != "open.feishu.cn" {
			return NotificationSecrets{}, fmt.Errorf("飞书通知地址无效")
		}
	}
	if to := strings.TrimSpace(string(data["email-to"])); to != "" {
		port, parseErr := strconv.Atoi(strings.TrimSpace(string(data["email-smtp-port"])))
		result.Email = EmailConfig{Enabled: true, SMTPHost: strings.TrimSpace(string(data["email-smtp-host"])), SMTPPort: port, Username: strings.TrimSpace(string(data["email-username"])), Password: string(data["email-password"]), From: strings.TrimSpace(string(data["email-from"])), To: to, TLSMode: strings.TrimSpace(string(data["email-tls-mode"]))}
		if parseErr != nil || result.Email.SMTPHost == "" || result.Email.SMTPPort < 1 || result.Email.From == "" || result.Email.TLSMode == "" {
			return NotificationSecrets{}, fmt.Errorf("邮件通知配置无效")
		}
	}
	return result, nil
}
