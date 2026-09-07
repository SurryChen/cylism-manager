package alerting

import (
	"context"
	"testing"
)

type secretReaderFake map[string][]byte

func (f secretReaderFake) GetSecretDataContext(context.Context, string, string) (map[string][]byte, error) {
	return f, nil
}

func TestLoadSecretsValidatesAndParsesChannels(t *testing.T) {
	secrets, err := LoadSecrets(context.Background(), secretReaderFake{
		"relay-token": []byte("relay"), "feishu-webhook-url": []byte("https://open.feishu.cn/open-apis/bot/v2/hook/x"),
		"email-to": []byte("ops@example.com"), "email-smtp-host": []byte("smtp.example.com"), "email-smtp-port": []byte("587"), "email-from": []byte("alerts@example.com"), "email-tls-mode": []byte("starttls"),
	}, "monitoring", "secret")
	if err != nil || !secrets.Configured() || !secrets.Email.Enabled || secrets.Email.SMTPPort != 587 {
		t.Fatalf("unexpected secrets: %#v err=%v", secrets, err)
	}
}
