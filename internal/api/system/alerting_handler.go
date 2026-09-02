package system

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	alertingservice "github.com/cylism/cylism-manager/internal/service/observability/alerting"
	"github.com/gin-gonic/gin"
)

const (
	alertingNamespace          = "monitoring"
	alertingSecretName         = "cylism-alerting-secret"
	maxAlertPayload            = 512 << 10
	defaultAlertingPlatformURL = "https://cylism.crazycoding.top"
)

type alertmanagerRequestFunc func(context.Context, string, string, interface{}, interface{}) error
type AlertingHandler struct {
	alertmanager    alertmanagerRequestFunc
	platformURL     string
	resolvedCache   *alertingservice.ResolvedCache
	automationStore repository.AlertAutomationRepository
	dispatcher      alertRuntimeDispatcher
	automation      *alertingservice.AutomationService
	component       *alertingservice.ComponentService
	ready           func(context.Context) bool
	secrets         alertingservice.SecretReader
	sender          alertingservice.NotificationSender
}

type AlertingDependencies struct {
	Alertmanager alertingservice.RequestFunc
	Component    alertingservice.ComponentAdapter
	Ready        func(context.Context) bool
	Secrets      alertingservice.SecretReader
	Sender       alertingservice.NotificationSender
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

type alertmanagerAlert = alertingservice.Alert
type alertStatus = alertingservice.AlertStatus

type alertSilenceRequest struct {
	Matchers        []alertMatcher `json:"matchers"`
	DurationMinutes int            `json:"duration_minutes"`
	Comment         string         `json:"comment"`
}

type alertMatcher = alertingservice.Matcher

func NewAlertingHandler(platformURL string) *AlertingHandler {
	if platformURL == "" {
		platformURL = defaultAlertingPlatformURL
	}
	return &AlertingHandler{resolvedCache: alertingservice.NewResolvedCache(12), platformURL: normalizeAlertingPlatformURL(platformURL)}
}

func (h *AlertingHandler) WithDependencies(deps AlertingDependencies) *AlertingHandler {
	if deps.Alertmanager != nil {
		h.alertmanager = alertmanagerRequestFunc(deps.Alertmanager)
	}
	if deps.Component != nil {
		h.component = alertingservice.NewComponentService(deps.Component)
	}
	h.ready = deps.Ready
	h.secrets = deps.Secrets
	h.sender = deps.Sender
	return h
}

func (h *AlertingHandler) WithAutomation(store repository.AlertAutomationRepository, dispatcher alertRuntimeDispatcher) *AlertingHandler {
	h.automationStore = store
	h.dispatcher = dispatcher
	h.automation = alertingservice.NewAutomationService(store, dispatcher)
	return h
}

func (h *AlertingHandler) workflow() *alertingservice.Workflow {
	return &alertingservice.Workflow{
		Client: alertingservice.NewClient(alertingservice.RequestFunc(h.alertmanager)), Ready: h.ready,
		Automation: h.automation, Store: h.automationStore, Dispatcher: h.dispatcher, Cache: h.resolvedCache, PlatformURL: h.platformURL,
		Secrets: h.secrets, Sender: h.sender,
	}
}

func (h *AlertingHandler) Status(c *gin.Context) {
	if h.component == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	model.Success(c, h.component.Status(c.Request.Context()))
}

func (h *AlertingHandler) Install(c *gin.Context) {
	if h.component == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	var config k8s.AlertingConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		apiShared.BadRequest(c, "告警配置无效")
		return
	}
	status, err := h.component.Install(c.Request.Context(), config)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	model.SuccessWithMessage(c, status, "告警组件配置已提交")
}

func (h *AlertingHandler) Update(c *gin.Context) {
	if h.component == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	var config k8s.AlertingConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		apiShared.BadRequest(c, "告警配置无效")
		return
	}
	status, err := h.component.Update(c.Request.Context(), config)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	model.SuccessWithMessage(c, status, "告警设置已保存")
}

func (h *AlertingHandler) Uninstall(c *gin.Context) {
	if h.component == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	if err := h.component.Uninstall(c.Request.Context()); err != nil {
		apiShared.Error(c, http.StatusInternalServerError, model.CodeK8sAPIError, err.Error())
		return
	}
	model.SuccessWithMessage(c, gin.H{"data_retained": true}, "告警工作负载已卸载，通知 Secret 和本地 PVC 已保留")
}

func (h *AlertingHandler) Overview(c *gin.Context) {
	if err := h.workflow().EnsureReady(c.Request.Context()); err != nil {
		apiShared.Conflict(c, err.Error())
		return
	}
	overview, err := h.workflow().Overview(c.Request.Context())
	if err != nil {
		apiShared.K8sAPIError(c, "读取 Alertmanager 告警失败: "+err.Error())
		return
	}
	model.Success(c, overview)
}

func (h *AlertingHandler) ListSilences(c *gin.Context) {
	if err := h.workflow().EnsureReady(c.Request.Context()); err != nil {
		apiShared.Conflict(c, err.Error())
		return
	}
	silences, err := h.workflow().Silences(c.Request.Context())
	if err != nil {
		apiShared.K8sAPIError(c, "读取 Alertmanager 静默失败: "+err.Error())
		return
	}
	model.Success(c, silences)
}

func (h *AlertingHandler) CreateSilence(c *gin.Context) {
	if err := h.workflow().EnsureReady(c.Request.Context()); err != nil {
		apiShared.Conflict(c, err.Error())
		return
	}
	var request alertSilenceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "静默配置无效")
		return
	}
	serviceRequest := alertingservice.SilenceRequest{DurationMinutes: request.DurationMinutes, Comment: request.Comment}
	for _, matcher := range request.Matchers {
		serviceRequest.Matchers = append(serviceRequest.Matchers, alertingservice.Matcher{Name: matcher.Name, Value: matcher.Value, IsRegex: matcher.IsRegex, IsEqual: matcher.IsEqual})
	}
	response, endsAt, err := h.workflow().CreateSilence(c.Request.Context(), serviceRequest, time.Now().UTC())
	if err != nil {
		if strings.Contains(err.Error(), "静默需") || strings.Contains(err.Error(), "静默匹配") {
			apiShared.ValidationError(c, err.Error())
			return
		}
		apiShared.K8sAPIError(c, "创建 Alertmanager 静默失败: "+err.Error())
		return
	}
	model.Success(c, gin.H{"id": response.SilenceID, "ends_at": endsAt})
}

func (h *AlertingHandler) DeleteSilence(c *gin.Context) {
	if err := h.workflow().EnsureReady(c.Request.Context()); err != nil {
		apiShared.Conflict(c, err.Error())
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" || len(id) > 128 {
		apiShared.ValidationError(c, "静默 ID 无效")
		return
	}
	if err := h.workflow().DeleteSilence(c.Request.Context(), id); err != nil {
		apiShared.K8sAPIError(c, "删除 Alertmanager 静默失败: "+err.Error())
		return
	}
	model.Success(c, gin.H{"id": id})
}

func (h *AlertingHandler) TestNotification(c *gin.Context) {
	if h.secrets == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	channel := strings.ToLower(strings.TrimSpace(c.Query("channel")))
	if channel != "" && channel != "feishu" && channel != "email" {
		apiShared.ValidationError(c, "测试通知渠道必须为 feishu 或 email")
		return
	}
	notifications, err := h.workflow().LoadConfiguredSecrets(c.Request.Context(), alertingNamespace, alertingSecretName)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	payload := alertingservice.TestPayload(h.platformURL, time.Now())
	if err := h.workflow().SendTest(c.Request.Context(), notifications, payload, channel); err != nil {
		apiShared.K8sAPIError(c, "发送测试通知失败: "+err.Error())
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
	notifications, err := h.workflow().LoadConfiguredSecrets(c.Request.Context(), alertingNamespace, alertingSecretName)
	if err != nil || !alertingservice.ValidateRelayToken(c.GetHeader("Authorization"), notifications.RelayToken) {
		apiShared.Unauthorized(c, "告警回调未授权")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAlertPayload)
	var payload alertmanagerNotification
	if err := c.ShouldBindJSON(&payload); err != nil {
		apiShared.BadRequest(c, "告警回调载荷无效")
		return
	}
	payload.PlatformURL = h.platformURL
	servicePayload := alertingservice.AlertNotification{Status: payload.Status, Alerts: payload.Alerts, PlatformURL: payload.PlatformURL}
	workflow := h.workflow()
	if err := workflow.ValidateNotification(servicePayload); err != nil {
		apiShared.BadRequest(c, "告警回调载荷无效")
		return
	}
	if _, err := workflow.Persist(c.Request.Context(), servicePayload, false); err != nil {
		apiShared.DBError(c, "持久化告警事件失败")
		return
	}
	delivered := false
	if notifications.Configured() {
		if err := workflow.Send(c.Request.Context(), notifications, servicePayload); err != nil {
			apiShared.K8sAPIError(c, "转发告警通知失败: "+err.Error())
			return
		}
		delivered = true
	}
	workflow.RecordResolved(servicePayload)
	model.Success(c, gin.H{"delivered": delivered, "persisted": true})
}

func (h *AlertingHandler) AutomationPolicy(c *gin.Context) {
	policy, err := h.workflow().Policy()
	if err != nil {
		model.Success(c, model.AlertAutomationPolicy{Enabled: false, MinimumSeverity: "warning", Mode: model.AlertAutomationReportOnly, CooldownMinutes: 30})
		return
	}
	model.Success(c, policy)
}

func (h *AlertingHandler) UpdateAutomationPolicy(c *gin.Context) {
	var policy model.AlertAutomationPolicy
	if err := c.ShouldBindJSON(&policy); err != nil || policy.RuntimeID == 0 || !alertingservice.ValidatePolicy(&policy) {
		apiShared.ValidationError(c, "告警自动化策略无效")
		return
	}
	result := gin.H{"policy": policy, "synced": 0, "sync_warning": ""}
	count, err := h.workflow().UpdatePolicy(c.Request.Context(), &policy)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "无效") || strings.Contains(err.Error(), "必须选择") {
			status = http.StatusBadRequest
		}
		apiShared.Error(c, status, model.CodeValidationFail, err.Error())
		return
	}
	result["synced"] = count
	model.SuccessWithMessage(c, result, "告警自动化策略已保存")
}

func (h *AlertingHandler) ListAutomationEvents(c *gin.Context) {
	events, err := h.workflow().ListEvents(30)
	if err != nil {
		apiShared.DBError(c, "读取告警自动化事件失败")
		return
	}
	model.Success(c, events)
}

func normalizeAlertingPlatformURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil {
		return defaultAlertingPlatformURL + "/#/monitoring?tab=alerts"
	}
	return strings.TrimRight(parsed.Scheme+"://"+parsed.Host, "/") + "/#/monitoring?tab=alerts"
}
