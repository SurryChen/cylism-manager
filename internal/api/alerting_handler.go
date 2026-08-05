package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	alertingNamespace          = "monitoring"
	alertingSecretName         = "cylism-alerting-secret"
	maxAlertPayload            = 512 << 10
	defaultAlertingPlatformURL = "https://cylism.crazycoding.top"
)

type alertmanagerRequestFunc func(context.Context, string, string, interface{}, interface{}) error
type alertNotifyFunc func(context.Context, string, alertmanagerNotification) error
type alertEmailNotifyFunc func(context.Context, k8s.EmailConfig, alertmanagerNotification) error

type AlertingHandler struct {
	alertmanager alertmanagerRequestFunc
	notify       alertNotifyFunc
	emailNotify  alertEmailNotifyFunc
	platformURL  string
	resolvedMu   sync.Mutex
	resolved     []alertmanagerAlert
}

type alertmanagerNotification struct {
	Version           string              `json:"version,omitempty"`
	Status            string              `json:"status,omitempty"`
	Receiver          string              `json:"receiver,omitempty"`
	GroupLabels       map[string]string   `json:"groupLabels,omitempty"`
	CommonLabels      map[string]string   `json:"commonLabels,omitempty"`
	CommonAnnotations map[string]string   `json:"commonAnnotations,omitempty"`
	ExternalURL       string              `json:"externalURL,omitempty"`
	Alerts            []alertmanagerAlert `json:"alerts"`
	PlatformURL       string              `json:"-"`
}

type alertmanagerAlert struct {
	Fingerprint  string            `json:"fingerprint,omitempty"`
	Status       alertStatus       `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL,omitempty"`
}

type alertStatus struct {
	State string `json:"state"`
}

// Alertmanager uses a string status in webhook batches but an object in its
// v2 query API. Accept both forms at this protocol boundary.
func (s *alertStatus) UnmarshalJSON(data []byte) error {
	var state string
	if err := json.Unmarshal(data, &state); err == nil {
		s.State = state
		return nil
	}
	var object struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	s.State = object.State
	return nil
}

type alertOverview struct {
	Active   []alertmanagerAlert `json:"active"`
	Resolved []alertmanagerAlert `json:"resolved"`
	Firing   int                 `json:"firing"`
	Silenced int                 `json:"silenced"`
}

type alertSilenceRequest struct {
	Matchers        []alertMatcher `json:"matchers"`
	DurationMinutes int            `json:"duration_minutes"`
	Comment         string         `json:"comment"`
}

type alertMatcher struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"isRegex"`
	IsEqual bool   `json:"isEqual"`
}

type alertmanagerSilence struct {
	ID        string         `json:"id,omitempty"`
	Matchers  []alertMatcher `json:"matchers"`
	StartsAt  time.Time      `json:"startsAt"`
	EndsAt    time.Time      `json:"endsAt"`
	CreatedBy string         `json:"createdBy,omitempty"`
	Comment   string         `json:"comment,omitempty"`
	Status    alertStatus    `json:"status"`
}

func NewAlertingHandler(platformURLs ...string) *AlertingHandler {
	platformURL := defaultAlertingPlatformURL
	if len(platformURLs) > 0 {
		platformURL = platformURLs[0]
	}
	return &AlertingHandler{alertmanager: alertmanagerRequest, notify: sendFeishuNotification, emailNotify: sendEmailNotification, platformURL: normalizeAlertingPlatformURL(platformURL)}
}

func (h *AlertingHandler) Status(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	model.Success(c, K8s.AlertingStatus())
}

func (h *AlertingHandler) Install(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	var config k8s.AlertingConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "告警配置无效")
		return
	}
	status, err := K8s.InstallAlerting(config)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	model.SuccessWithMessage(c, status, "告警组件配置已提交")
}

func (h *AlertingHandler) Update(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	var config k8s.AlertingConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "告警配置无效")
		return
	}
	status, err := K8s.UpdateAlerting(config)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	model.SuccessWithMessage(c, status, "告警设置已保存")
}

func (h *AlertingHandler) Uninstall(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	if err := K8s.UninstallAlerting(); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeK8sAPIError, err.Error())
		return
	}
	model.SuccessWithMessage(c, gin.H{"data_retained": true}, "告警工作负载已卸载，通知 Secret 和本地 PVC 已保留")
}

func (h *AlertingHandler) Overview(c *gin.Context) {
	if !h.readyForAlertmanager(c) {
		return
	}
	alerts := []alertmanagerAlert{}
	if err := h.alertmanager(c.Request.Context(), http.MethodGet, "/api/v2/alerts", nil, &alerts); err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "读取 Alertmanager 告警失败: "+err.Error())
		return
	}
	overview := alertOverview{Active: make([]alertmanagerAlert, 0), Resolved: make([]alertmanagerAlert, 0)}
	for _, alert := range alerts {
		switch alert.Status.State {
		case "resolved":
			overview.Resolved = append(overview.Resolved, alert)
		case "suppressed":
			overview.Silenced++
			overview.Active = append(overview.Active, alert)
		default:
			overview.Active = append(overview.Active, alert)
			if alert.Status.State == "firing" {
				overview.Firing++
			}
		}
	}
	if len(overview.Resolved) > 12 {
		overview.Resolved = overview.Resolved[:12]
	}
	if recent := h.recentResolved(); len(recent) > 0 {
		overview.Resolved = append(recent, overview.Resolved...)
		if len(overview.Resolved) > 12 {
			overview.Resolved = overview.Resolved[:12]
		}
	}
	model.Success(c, overview)
}

func (h *AlertingHandler) ListSilences(c *gin.Context) {
	if !h.readyForAlertmanager(c) {
		return
	}
	silences := []alertmanagerSilence{}
	if err := h.alertmanager(c.Request.Context(), http.MethodGet, "/api/v2/silences", nil, &silences); err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "读取 Alertmanager 静默失败: "+err.Error())
		return
	}
	model.Success(c, silences)
}

func (h *AlertingHandler) CreateSilence(c *gin.Context) {
	if !h.readyForAlertmanager(c) {
		return
	}
	var request alertSilenceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "静默配置无效")
		return
	}
	if len(request.Matchers) == 0 || len(request.Matchers) > 12 || request.DurationMinutes < 1 || request.DurationMinutes > 7*24*60 {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "静默需包含匹配条件，且时长应在 1 分钟到 7 天之间")
		return
	}
	for index, matcher := range request.Matchers {
		request.Matchers[index].Name = strings.TrimSpace(matcher.Name)
		request.Matchers[index].Value = strings.TrimSpace(matcher.Value)
		request.Matchers[index].IsEqual = true
		if request.Matchers[index].Name == "" || request.Matchers[index].Value == "" {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "静默匹配条件不能为空")
			return
		}
	}
	now := time.Now().UTC()
	payload := alertmanagerSilence{Matchers: request.Matchers, StartsAt: now, EndsAt: now.Add(time.Duration(request.DurationMinutes) * time.Minute), CreatedBy: "cylism-manager", Comment: strings.TrimSpace(request.Comment)}
	var response struct {
		SilenceID string `json:"silenceID"`
	}
	if err := h.alertmanager(c.Request.Context(), http.MethodPost, "/api/v2/silences", payload, &response); err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "创建 Alertmanager 静默失败: "+err.Error())
		return
	}
	model.Success(c, gin.H{"id": response.SilenceID, "ends_at": payload.EndsAt})
}

func (h *AlertingHandler) DeleteSilence(c *gin.Context) {
	if !h.readyForAlertmanager(c) {
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" || len(id) > 128 {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "静默 ID 无效")
		return
	}
	if err := h.alertmanager(c.Request.Context(), http.MethodDelete, "/api/v2/silence/"+id, nil, nil); err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "删除 Alertmanager 静默失败: "+err.Error())
		return
	}
	model.Success(c, gin.H{"id": id})
}

func (h *AlertingHandler) TestNotification(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	channel := strings.ToLower(strings.TrimSpace(c.Query("channel")))
	if channel != "" && channel != "feishu" && channel != "email" {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "测试通知渠道必须为 feishu 或 email")
		return
	}
	notifications, err := alertingSecrets()
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	payload := alertmanagerNotification{Status: "firing", PlatformURL: h.platformURL, Alerts: []alertmanagerAlert{{Status: alertStatus{State: "firing"}, Labels: map[string]string{"alertname": "CylismAlertingTest", "node": "示例节点", "severity": "warning"}, Annotations: map[string]string{"summary": "测试消息使用真实告警的完整卡片布局", "description": "模拟节点根磁盘使用率超过阈值", "rule_name": "节点根磁盘使用率过高（测试）", "current_value": "92.4%", "threshold": "85%", "duration": "10 分钟"}, StartsAt: time.Now().UTC()}}}
	if err := h.sendTestNotification(c.Request.Context(), notifications, payload, channel); err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "发送测试通知失败: "+err.Error())
		return
	}
	message := "测试通知已发送"
	if channel == "feishu" {
		message = "飞书测试通知已发送"
	} else if channel == "email" {
		message = "邮件测试通知已发送"
	}
	model.SuccessWithMessage(c, gin.H{"configured": true}, message)
}

// Notify receives Alertmanager's cluster-internal webhook. It deliberately does
// not use JWT because Alertmanager is not a browser client; the per-install token
// mounted into Alertmanager authenticates this one endpoint instead.
func (h *AlertingHandler) Notify(c *gin.Context) {
	if K8s == nil {
		model.Error(c, http.StatusServiceUnavailable, model.CodeK8sAPIError, "Kubernetes 客户端未初始化")
		return
	}
	notifications, err := alertingSecrets()
	if err != nil || !validAlertRelayToken(c.GetHeader("Authorization"), notifications.RelayToken) {
		model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "告警回调未授权")
		return
	}
	if !notifications.configured() {
		model.Error(c, http.StatusConflict, model.CodeConflict, "告警通知渠道尚未配置")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAlertPayload)
	var payload alertmanagerNotification
	if err := c.ShouldBindJSON(&payload); err != nil || len(payload.Alerts) == 0 || len(payload.Alerts) > 64 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "告警回调载荷无效")
		return
	}
	payload.PlatformURL = h.platformURL
	if err := h.sendNotifications(c.Request.Context(), notifications, payload); err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "转发告警通知失败: "+err.Error())
		return
	}
	h.recordResolved(payload)
	model.Success(c, gin.H{"delivered": true})
}

func (h *AlertingHandler) recordResolved(payload alertmanagerNotification) {
	h.resolvedMu.Lock()
	defer h.resolvedMu.Unlock()
	for _, alert := range payload.Alerts {
		if payload.Status != "resolved" && alert.Status.State != "resolved" {
			continue
		}
		if alert.EndsAt.IsZero() {
			alert.EndsAt = time.Now().UTC()
		}
		alert.Status.State = "resolved"
		if alert.Fingerprint != "" {
			filtered := h.resolved[:0]
			for _, existing := range h.resolved {
				if existing.Fingerprint != alert.Fingerprint {
					filtered = append(filtered, existing)
				}
			}
			h.resolved = filtered
		}
		h.resolved = append([]alertmanagerAlert{alert}, h.resolved...)
	}
	if len(h.resolved) > 12 {
		h.resolved = h.resolved[:12]
	}
}

func (h *AlertingHandler) recentResolved() []alertmanagerAlert {
	h.resolvedMu.Lock()
	defer h.resolvedMu.Unlock()
	return append([]alertmanagerAlert(nil), h.resolved...)
}

func (h *AlertingHandler) readyForAlertmanager(c *gin.Context) bool {
	if K8s == nil {
		k8sUnavailable(c)
		return false
	}
	if status := K8s.AlertingStatus(); status.State != k8s.AlertingStateReady {
		model.Error(c, http.StatusConflict, model.CodeConflict, "Alertmanager 尚未就绪: "+status.Message)
		return false
	}
	return true
}

type alertingNotificationSecrets struct {
	FeishuWebhookURL string
	RelayToken       string
	Email            k8s.EmailConfig
}

func (s alertingNotificationSecrets) configured() bool {
	return s.FeishuWebhookURL != "" || s.Email.Enabled
}

func alertingSecrets() (alertingNotificationSecrets, error) {
	secret, err := K8s.Clientset.CoreV1().Secrets(alertingNamespace).Get(K8s.Ctx(), alertingSecretName, metav1.GetOptions{})
	if err != nil {
		return alertingNotificationSecrets{}, fmt.Errorf("读取告警通知配置失败")
	}
	settings := alertingNotificationSecrets{FeishuWebhookURL: strings.TrimSpace(string(secret.Data["feishu-webhook-url"])), RelayToken: strings.TrimSpace(string(secret.Data["relay-token"]))}
	if settings.RelayToken == "" {
		return alertingNotificationSecrets{}, fmt.Errorf("告警回调令牌尚未配置")
	}
	if settings.FeishuWebhookURL != "" && !validFeishuWebhookURL(settings.FeishuWebhookURL) {
		return alertingNotificationSecrets{}, fmt.Errorf("飞书通知地址无效")
	}
	if to := strings.TrimSpace(string(secret.Data["email-to"])); to != "" {
		port, parseErr := strconv.Atoi(strings.TrimSpace(string(secret.Data["email-smtp-port"])))
		settings.Email = k8s.EmailConfig{Enabled: true, SMTPHost: strings.TrimSpace(string(secret.Data["email-smtp-host"])), SMTPPort: port, Username: strings.TrimSpace(string(secret.Data["email-username"])), Password: string(secret.Data["email-password"]), From: strings.TrimSpace(string(secret.Data["email-from"])), To: to, TLSMode: strings.TrimSpace(string(secret.Data["email-tls-mode"]))}
		if parseErr != nil || settings.Email.SMTPHost == "" || settings.Email.SMTPPort < 1 || settings.Email.From == "" || settings.Email.TLSMode == "" {
			return alertingNotificationSecrets{}, fmt.Errorf("邮件通知配置无效")
		}
	}
	return settings, nil
}

func (h *AlertingHandler) sendNotifications(ctx context.Context, settings alertingNotificationSecrets, payload alertmanagerNotification) error {
	var failures []string
	if settings.FeishuWebhookURL != "" {
		if err := h.notify(ctx, settings.FeishuWebhookURL, payload); err != nil {
			failures = append(failures, "飞书: "+err.Error())
		}
	}
	if settings.Email.Enabled {
		if err := h.emailNotify(ctx, settings.Email, payload); err != nil {
			failures = append(failures, "邮件: "+err.Error())
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil
}

func (h *AlertingHandler) sendTestNotification(ctx context.Context, settings alertingNotificationSecrets, payload alertmanagerNotification, channel string) error {
	switch channel {
	case "feishu":
		if settings.FeishuWebhookURL == "" {
			return fmt.Errorf("飞书通知渠道尚未配置")
		}
		if err := h.notify(ctx, settings.FeishuWebhookURL, payload); err != nil {
			return fmt.Errorf("飞书: %w", err)
		}
		return nil
	case "email":
		if !settings.Email.Enabled {
			return fmt.Errorf("邮件通知渠道尚未配置")
		}
		if err := h.emailNotify(ctx, settings.Email, payload); err != nil {
			return fmt.Errorf("邮件: %w", err)
		}
		return nil
	default:
		return h.sendNotifications(ctx, settings, payload)
	}
}

func validFeishuWebhookURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() == "open.feishu.cn"
}

func normalizeAlertingPlatformURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil {
		return defaultAlertingPlatformURL + "/#/monitoring?tab=alerts"
	}
	return strings.TrimRight(parsed.Scheme+"://"+parsed.Host, "/") + "/#/monitoring?tab=alerts"
}

func validAlertRelayToken(header, expected string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) || expected == "" {
		return false
	}
	provided := strings.TrimPrefix(header, prefix)
	if len(provided) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func alertmanagerRequest(ctx context.Context, method, path string, input, output interface{}) error {
	var body io.Reader
	if input != nil {
		payload, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("编码请求失败: %w", err)
		}
		body = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, k8s.AlertmanagerServiceURL()+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := (&http.Client{Timeout: 12 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return fmt.Errorf("Alertmanager 返回 %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	if output != nil && response.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(io.LimitReader(response.Body, maxAlertPayload)).Decode(output); err != nil {
			return fmt.Errorf("解析 Alertmanager 响应失败: %w", err)
		}
	}
	return nil
}

func sendFeishuNotification(ctx context.Context, webhookURL string, payload alertmanagerNotification) error {
	message, err := json.Marshal(feishuMessage(payload))
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(message))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("飞书返回 %s", response.Status)
	}
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&result); err == nil && result.Code != 0 {
		return fmt.Errorf("飞书返回 %d: %s", result.Code, result.Msg)
	}
	return nil
}

func sendEmailNotification(ctx context.Context, config k8s.EmailConfig, payload alertmanagerNotification) error {
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
		connection.Close()
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
	for _, address := range addresses {
		if err := client.Rcpt(address); err != nil {
			return fmt.Errorf("SMTP 收件人被拒绝: %w", err)
		}
	}
	plainBody := emailMessage(payload)
	htmlBody := emailHTMLMessage(payload)
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("开始 SMTP 邮件内容失败: %w", err)
	}
	_, writeErr := io.WriteString(writer, alertEmailMIME(from.String(), strings.Join(addresses, ", "), payload, plainBody, htmlBody))
	closeErr := writer.Close()
	if writeErr != nil {
		return fmt.Errorf("写入 SMTP 邮件内容失败: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("提交 SMTP 邮件失败: %w", closeErr)
	}
	return nil
}

func emailMessage(payload alertmanagerNotification) string {
	state := "告警恢复"
	if payload.Status == "firing" {
		state = "告警触发"
	}
	lines := []string{state, ""}
	for _, alert := range payload.Alerts {
		for _, field := range alertNotificationFields(alert) {
			lines = append(lines, field.Label+": "+field.Value)
		}
		lines = append(lines, "")
	}
	if payload.PlatformURL != "" {
		lines = append(lines, "查看平台告警: "+payload.PlatformURL)
	}
	return strings.Join(lines, "\n")
}

func feishuMessage(payload alertmanagerNotification) gin.H {
	state := "告警恢复"
	if payload.Status == "firing" {
		state = "告警触发"
	}
	elements := make([]gin.H, 0, len(payload.Alerts)*2+2)
	for _, alert := range payload.Alerts {
		fields := make([]gin.H, 0, 6)
		for _, field := range alertNotificationFields(alert) {
			fields = append(fields, gin.H{"is_short": field.Label != "告警说明", "text": gin.H{"tag": "lark_md", "content": "**" + escapeLarkMarkdown(field.Label) + "**\n" + escapeLarkMarkdown(field.Value)}})
		}
		elements = append(elements, gin.H{"tag": "div", "fields": fields}, gin.H{"tag": "hr"})
	}
	if len(elements) > 0 {
		elements = elements[:len(elements)-1]
	}
	if payload.PlatformURL != "" {
		elements = append(elements, gin.H{"tag": "action", "actions": []gin.H{{"tag": "button", "text": gin.H{"tag": "plain_text", "content": "查看平台告警"}, "type": "primary", "url": payload.PlatformURL}}})
	}
	return gin.H{"msg_type": "interactive", "card": gin.H{"config": gin.H{"wide_screen_mode": true, "enable_forward": true}, "header": gin.H{"title": gin.H{"tag": "plain_text", "content": fmt.Sprintf("%s · %d 条", state, len(payload.Alerts))}, "template": map[bool]string{true: "red", false: "green"}[payload.Status == "firing"]}, "elements": elements}}
}

type alertNotificationField struct {
	Label string
	Value string
}

func alertNotificationFields(alert alertmanagerAlert) []alertNotificationField {
	name := alert.Labels["alertname"]
	if name == "" {
		name = "集群告警"
	}
	rule := alert.Annotations["rule_name"]
	if rule == "" {
		rule = name
	}
	summary := alert.Annotations["summary"]
	if summary == "" {
		summary = alert.Annotations["description"]
	}
	fields := []alertNotificationField{{Label: "告警规则", Value: rule}, {Label: "告警对象", Value: alertNotificationTarget(alert)}, {Label: "告警说明", Value: summary}}
	if value := strings.TrimSpace(alert.Annotations["current_value"]); value != "" {
		fields = append(fields, alertNotificationField{Label: "当前值", Value: value})
	}
	if threshold := strings.TrimSpace(alert.Annotations["threshold"]); threshold != "" {
		fields = append(fields, alertNotificationField{Label: "阈值", Value: threshold})
	}
	if duration := strings.TrimSpace(alert.Annotations["duration"]); duration != "" {
		fields = append(fields, alertNotificationField{Label: "触发条件", Value: "持续 " + duration})
	}
	if !alert.StartsAt.IsZero() {
		fields = append(fields, alertNotificationField{Label: "开始时间", Value: alert.StartsAt.Local().Format("2006-01-02 15:04:05 MST")})
	}
	return fields
}

func alertNotificationTarget(alert alertmanagerAlert) string {
	if node := strings.TrimSpace(alert.Labels["node"]); node != "" {
		return "节点 " + node
	}
	namespace := strings.TrimSpace(alert.Labels["namespace"])
	for _, key := range []string{"pod", "deployment", "statefulset", "daemonset", "job"} {
		if value := strings.TrimSpace(alert.Labels[key]); value != "" {
			if namespace != "" {
				return namespace + "/" + value
			}
			return value
		}
	}
	if namespace != "" {
		return "命名空间 " + namespace
	}
	return "集群"
}

func emailHTMLMessage(payload alertmanagerNotification) string {
	state := "告警恢复"
	accent := "#198754"
	if payload.Status == "firing" {
		state = "告警触发"
		accent = "#d92d20"
	}
	var body strings.Builder
	body.WriteString(`<!doctype html><html><body style="margin:0;background:#f5f7fa;color:#1f2937;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;"><div style="max-width:680px;margin:24px auto;background:#ffffff;border:1px solid #e5e7eb;border-radius:8px;overflow:hidden;"><div style="padding:18px 22px;background:` + accent + `;color:#ffffff;font-size:18px;font-weight:700;">` + html.EscapeString(state) + ` · ` + strconv.Itoa(len(payload.Alerts)) + ` 条</div><div style="padding:20px 22px;">`)
	for _, alert := range payload.Alerts {
		body.WriteString(`<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="margin:0 0 16px;border:1px solid #e5e7eb;border-radius:6px;border-collapse:separate;overflow:hidden;">`)
		for _, field := range alertNotificationFields(alert) {
			body.WriteString(`<tr><td style="width:112px;padding:10px 12px;background:#f9fafb;color:#6b7280;font-size:12px;border-bottom:1px solid #e5e7eb;vertical-align:top;">` + html.EscapeString(field.Label) + `</td><td style="padding:10px 12px;font-size:14px;border-bottom:1px solid #e5e7eb;word-break:break-word;">` + html.EscapeString(field.Value) + `</td></tr>`)
		}
		body.WriteString(`</table>`)
	}
	if payload.PlatformURL != "" {
		body.WriteString(`<a href="` + html.EscapeString(payload.PlatformURL) + `" style="display:inline-block;padding:10px 15px;background:#2563eb;color:#ffffff;text-decoration:none;border-radius:6px;font-weight:600;">查看平台告警</a>`)
	}
	body.WriteString(`</div></div></body></html>`)
	return body.String()
}

func alertEmailMIME(from, to string, payload alertmanagerNotification, plainBody, htmlBody string) string {
	const boundary = "cylism-alert-message"
	subject := "Cylism 告警恢复"
	if payload.Status == "firing" {
		subject = "Cylism 告警触发"
	}
	return "From: " + from + "\r\nTo: " + to + "\r\nSubject: " + mime.QEncoding.Encode("UTF-8", subject) + "\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n--" + boundary + "\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n" + quotedPrintableEncode(plainBody) + "\r\n--" + boundary + "\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n" + quotedPrintableEncode(htmlBody) + "\r\n--" + boundary + "--\r\n"
}

func quotedPrintableEncode(body string) string {
	var encoded bytes.Buffer
	writer := quotedprintable.NewWriter(&encoded)
	_, _ = writer.Write([]byte(body))
	_ = writer.Close()
	return encoded.String()
}

func escapeLarkMarkdown(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "(", "\\(", ")", "\\)")
	return replacer.Replace(value)
}
