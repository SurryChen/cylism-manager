package alerting

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

func SendSMTP(ctx context.Context, config EmailConfig, plainMessage string, mimeMessage string) error {
	from, err := mail.ParseAddress(config.From)
	if err != nil {
		return fmt.Errorf("发件人地址无效: %w", err)
	}
	recipients, err := mail.ParseAddressList(config.To)
	if err != nil || len(recipients) == 0 {
		return fmt.Errorf("收件人地址无效")
	}
	endpoint := net.JoinHostPort(config.SMTPHost, strconv.Itoa(config.SMTPPort))
	tlsConfig := &tls.Config{ServerName: config.SMTPHost, MinVersion: tls.VersionTLS12}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var connection net.Conn
	if config.TLSMode == "tls" {
		connection, err = tls.DialWithDialer(dialer, "tcp", endpoint, tlsConfig)
	} else {
		connection, err = dialer.DialContext(ctx, "tcp", endpoint)
	}
	if err != nil {
		return fmt.Errorf("连接 SMTP 服务失败: %w", err)
	}
	client, err := smtp.NewClient(connection, config.SMTPHost)
	if err != nil {
		_ = connection.Close()
		return fmt.Errorf("初始化 SMTP 会话失败: %w", err)
	}
	defer client.Quit()
	if config.TLSMode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("SMTP 服务不支持 STARTTLS")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("启动 SMTP TLS 失败: %w", err)
		}
	}
	if config.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", config.Username, config.Password, config.SMTPHost)); err != nil {
			return fmt.Errorf("SMTP 认证失败: %w", err)
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("SMTP 发件人被拒绝: %w", err)
	}
	addresses := make([]string, 0, len(recipients))
	for _, r := range recipients {
		addresses = append(addresses, r.Address)
		if err := client.Rcpt(r.Address); err != nil {
			return fmt.Errorf("SMTP 收件人被拒绝: %w", err)
		}
	}
	_ = addresses
	if maximumSMTPLineLength(mimeMessage) > 998 {
		return fmt.Errorf("邮件内容存在超过 SMTP 限制的行")
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("开始 SMTP 邮件内容失败: %w", err)
	}
	_, writeErr := io.WriteString(writer, mimeMessage)
	closeErr := writer.Close()
	if writeErr != nil {
		return fmt.Errorf("写入 SMTP 邮件内容失败: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("提交 SMTP 邮件失败: %w", closeErr)
	}
	_ = plainMessage
	return nil
}

func maximumSMTPLineLength(message string) int {
	max := 0
	for _, line := range strings.Split(message, "\r\n") {
		if n := len([]byte(line)); n > max {
			max = n
		}
	}
	return max
}

func MaximumSMTPLineLength(message string) int { return maximumSMTPLineLength(message) }
