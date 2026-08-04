package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	alertingNamespace  = "monitoring"
	alertingSecretName = "cylism-alerting-secret"
	maxAlertPayload    = 512 << 10
)

type alertmanagerRequestFunc func(context.Context, string, string, interface{}, interface{}) error
type alertNotifyFunc func(context.Context, string, alertmanagerNotification) error

type AlertingHandler struct {
	alertmanager alertmanagerRequestFunc
	notify       alertNotifyFunc
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

func NewAlertingHandler() *AlertingHandler {
	return &AlertingHandler{alertmanager: alertmanagerRequest, notify: sendFeishuNotification}
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
	webhookURL, _, err := alertingSecrets()
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	payload := alertmanagerNotification{Status: "firing", Alerts: []alertmanagerAlert{{Status: alertStatus{State: "firing"}, Labels: map[string]string{"alertname": "CylismAlertingTest", "severity": "info"}, Annotations: map[string]string{"summary": "Cylism 告警通知测试", "description": "飞书通知通道已连通"}, StartsAt: time.Now().UTC()}}}
	if err := h.notify(c.Request.Context(), webhookURL, payload); err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "发送飞书测试通知失败: "+err.Error())
		return
	}
	model.SuccessWithMessage(c, gin.H{"configured": true}, "测试通知已发送")
}

// Notify receives Alertmanager's cluster-internal webhook. It deliberately does
// not use JWT because Alertmanager is not a browser client; the per-install token
// mounted into Alertmanager authenticates this one endpoint instead.
func (h *AlertingHandler) Notify(c *gin.Context) {
	if K8s == nil {
		model.Error(c, http.StatusServiceUnavailable, model.CodeK8sAPIError, "Kubernetes 客户端未初始化")
		return
	}
	webhookURL, relayToken, err := alertingSecrets()
	if err != nil || !validAlertRelayToken(c.GetHeader("Authorization"), relayToken) {
		model.Error(c, http.StatusUnauthorized, model.CodeUnauthorized, "告警回调未授权")
		return
	}
	if webhookURL == "" {
		model.Error(c, http.StatusConflict, model.CodeConflict, "飞书通知渠道尚未配置")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAlertPayload)
	var payload alertmanagerNotification
	if err := c.ShouldBindJSON(&payload); err != nil || len(payload.Alerts) == 0 || len(payload.Alerts) > 64 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "告警回调载荷无效")
		return
	}
	if err := h.notify(c.Request.Context(), webhookURL, payload); err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "转发飞书通知失败: "+err.Error())
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

func alertingSecrets() (string, string, error) {
	secret, err := K8s.Clientset.CoreV1().Secrets(alertingNamespace).Get(K8s.Ctx(), alertingSecretName, metav1.GetOptions{})
	if err != nil {
		return "", "", fmt.Errorf("读取告警通知配置失败")
	}
	webhookURL := strings.TrimSpace(string(secret.Data["feishu-webhook-url"]))
	token := strings.TrimSpace(string(secret.Data["relay-token"]))
	if token == "" {
		return "", "", fmt.Errorf("告警回调令牌尚未配置")
	}
	if webhookURL != "" && !validFeishuWebhookURL(webhookURL) {
		return "", "", fmt.Errorf("飞书通知地址无效")
	}
	return webhookURL, token, nil
}

func validFeishuWebhookURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() == "open.feishu.cn"
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

func feishuMessage(payload alertmanagerNotification) gin.H {
	state := "告警恢复"
	if payload.Status == "firing" {
		state = "告警触发"
	}
	lines := make([]string, 0, len(payload.Alerts))
	for _, alert := range payload.Alerts {
		name := alert.Labels["alertname"]
		if name == "" {
			name = "集群告警"
		}
		summary := alert.Annotations["summary"]
		if summary == "" {
			summary = alert.Annotations["description"]
		}
		lines = append(lines, name+"\n"+summary)
	}
	return gin.H{"msg_type": "interactive", "card": gin.H{"header": gin.H{"title": gin.H{"tag": "plain_text", "content": state}, "template": map[bool]string{true: "red", false: "green"}[payload.Status == "firing"]}, "elements": []gin.H{{"tag": "div", "text": gin.H{"tag": "lark_md", "content": strings.Join(lines, "\n\n")}}}}}
}
