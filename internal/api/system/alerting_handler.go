package system

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
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
	publicURL       string
	clusterID       uint
	notifyURL       string
	resolvedCache   *alertingservice.ResolvedCache
	automationStore repository.AlertAutomationRepository
	dispatcher      alertingservice.Dispatcher
	automation      *alertingservice.AutomationService
	queryService    *alertingservice.QueryService
	component       *alertingservice.ComponentService
	ready           func(context.Context) bool
	secrets         alertingservice.SecretReader
	sender          alertingservice.NotificationSender
}

type AlertingDependencies struct {
	Alertmanager     alertingservice.RequestFunc
	Component        alertingservice.ComponentAdapter
	ComponentService *alertingservice.ComponentService
	Ready            func(context.Context) bool
	Secrets          alertingservice.SecretReader
	Sender           alertingservice.NotificationSender
	QueryService     *alertingservice.QueryService
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
	publicURL := strings.TrimSpace(platformURL)
	if platformURL == "" {
		platformURL = defaultAlertingPlatformURL
	}
	return &AlertingHandler{publicURL: publicURL, resolvedCache: alertingservice.NewResolvedCache(12), platformURL: normalizeAlertingPlatformURL(platformURL)}
}

// WithCluster binds callbacks and notification links to a single cluster. Only
// the configured public URL is trusted; request headers cannot select a token destination.
func (h *AlertingHandler) WithCluster(clusterID uint) *AlertingHandler {
	h.clusterID = clusterID
	parsed, err := url.Parse(h.publicURL)
	if err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Hostname() != "" && parsed.User == nil {
		base := parsed.Scheme + "://" + parsed.Host
		h.notifyURL = fmt.Sprintf("%s/api/clusters/%d/monitoring/alerts/notify", base, clusterID)
		h.platformURL = fmt.Sprintf("%s/#/cluster/%d/monitoring?tab=alerts", base, clusterID)
	}
	return h
}

func (h *AlertingHandler) notificationWarning(status *k8s.AlertingStatus) *k8s.AlertingStatus {
	if h.clusterID != 0 && status != nil {
		if h.notifyURL == "" {
			status.NotificationWarning = "请配置平台公开访问地址 server.public_url（PUBLIC_URL），以接收当前集群的告警回调。"
		} else if status.State != k8s.AlertingStateNotInstalled && status.NotifyURL != h.notifyURL {
			status.NotificationWarning = "告警回调仍使用旧地址，请打开告警设置并保存一次，以启用当前集群的通知回调。"
		}
	}
	return status
}

func (h *AlertingHandler) configureNotifyURL(c *gin.Context, config *k8s.AlertingConfig) bool {
	// Callback destinations are server-owned, including for the legacy endpoint.
	config.NotifyURL = h.notifyURL
	if h.clusterID != 0 && h.notifyURL == "" {
		apiShared.ValidationError(c, "请先配置平台公开访问地址 server.public_url（PUBLIC_URL）")
		return false
	}
	return true
}

// NewAlertingHandlerWithComposedDependencies receives the query, component
// and automation services created by Bootstrap without constructing fallbacks.
func NewAlertingHandlerWithComposedDependencies(platformURL string, deps AlertingDependencies, store repository.AlertAutomationRepository, automation *alertingservice.AutomationService, dispatcher alertingservice.Dispatcher) *AlertingHandler {
	handler := NewAlertingHandler(platformURL)
	handler.alertmanager = alertmanagerRequestFunc(deps.Alertmanager)
	handler.component = deps.ComponentService
	handler.ready = deps.Ready
	handler.secrets = deps.Secrets
	handler.sender = deps.Sender
	handler.queryService = deps.QueryService
	handler.automationStore = store
	handler.automation = automation
	handler.dispatcher = dispatcher
	return handler
}

func (h *AlertingHandler) WithDependencies(deps AlertingDependencies) *AlertingHandler {
	if deps.Alertmanager != nil {
		h.alertmanager = alertmanagerRequestFunc(deps.Alertmanager)
	}
	if deps.ComponentService != nil {
		h.component = deps.ComponentService
	} else if deps.Component != nil {
		h.component = alertingservice.NewComponentService(deps.Component)
	}
	h.ready = deps.Ready
	h.secrets = deps.Secrets
	h.sender = deps.Sender
	if deps.QueryService != nil {
		h.queryService = deps.QueryService
	} else if deps.Alertmanager != nil {
		h.queryService = alertingservice.NewQueryService(alertingservice.NewClient(deps.Alertmanager), deps.Ready)
	}
	return h
}

// WithAlertmanager injects a request transport and its pre-composed query
// service for focused tests or alternate embeddings.
func (h *AlertingHandler) WithAlertmanager(request alertingservice.RequestFunc, ready func(context.Context) bool) *AlertingHandler {
	h.alertmanager = alertmanagerRequestFunc(request)
	h.ready = ready
	h.queryService = alertingservice.NewQueryService(alertingservice.NewClient(request), ready)
	return h
}

func (h *AlertingHandler) WithAutomation(store repository.AlertAutomationRepository, dispatcher alertingservice.Dispatcher) *AlertingHandler {
	h.automationStore = store
	h.dispatcher = dispatcher
	h.automation = alertingservice.NewAutomationService(store, dispatcher)
	return h
}

// WithAutomationService injects the pre-composed automation service.
func (h *AlertingHandler) WithAutomationService(store repository.AlertAutomationRepository, service *alertingservice.AutomationService, dispatcher alertingservice.Dispatcher) *AlertingHandler {
	h.automationStore = store
	h.dispatcher = dispatcher
	h.automation = service
	return h
}

func (h *AlertingHandler) workflow() *alertingservice.Workflow {
	return &alertingservice.Workflow{
		Client: alertingservice.NewClient(alertingservice.RequestFunc(h.alertmanager)), Query: h.queryService, Ready: h.ready,
		Automation: h.automation, Store: h.automationStore, Dispatcher: h.dispatcher, Cache: h.resolvedCache, PlatformURL: h.platformURL,
		Secrets: h.secrets, Sender: h.sender,
	}
}

func (h *AlertingHandler) Status(c *gin.Context) {
	if h.component == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	apiShared.Success(c, h.notificationWarning(h.component.Status(c.Request.Context())))
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
	if !h.configureNotifyURL(c, &config) {
		return
	}
	status, err := h.component.Install(c.Request.Context(), config)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, status, "告警组件配置已提交")
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
	if !h.configureNotifyURL(c, &config) {
		return
	}
	status, err := h.component.Update(c.Request.Context(), config)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, status, "告警设置已保存")
}

func (h *AlertingHandler) Uninstall(c *gin.Context) {
	if h.component == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	if err := h.component.Uninstall(c.Request.Context()); err != nil {
		apiShared.Error(c, http.StatusInternalServerError, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, gin.H{"data_retained": true}, "告警工作负载已卸载，通知 Secret 和本地 PVC 已保留")
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
	if apiShared.PaginationRequested(c) {
		items := overview.Active
		view := c.DefaultQuery("view", "active")
		if view == "resolved" {
			items = overview.Resolved
		} else if view != "active" {
			apiShared.BadRequest(c, "告警视图无效")
			return
		}
		sort.SliceStable(items, func(i, j int) bool {
			if view == "active" {
				a, b := items[i].Labels["severity"] == "critical", items[j].Labels["severity"] == "critical"
				if a != b {
					return a
				}
			} else if !items[i].EndsAt.Equal(items[j].EndsAt) {
				return items[i].EndsAt.After(items[j].EndsAt)
			}
			return alertListKey(items[i]) < alertListKey(items[j])
		})
		limit, offset := apiShared.LimitOffset(c, 10, 100)
		start := min(offset, len(items))
		page := append(make([]alertingservice.Alert, 0), items[start:min(start+limit, len(items))]...)
		apiShared.Success(c, gin.H{"items": page, "total": len(items), "active_total": len(overview.Active), "resolved_total": len(overview.Resolved), "firing": overview.Firing, "silenced": overview.Silenced, "limit": limit, "offset": offset})
		return
	}
	apiShared.Success(c, overview)
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
	apiShared.Success(c, silences)
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
	apiShared.Success(c, gin.H{"id": response.SilenceID, "ends_at": endsAt})
}

func (h *AlertingHandler) DeleteSilence(c *gin.Context) {
	if err := h.workflow().EnsureReady(c.Request.Context()); err != nil {
		apiShared.Conflict(c, err.Error())
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	if silenceID := strings.TrimSpace(c.Param("silenceID")); silenceID != "" {
		id = silenceID
	}
	if id == "" || len(id) > 128 {
		apiShared.ValidationError(c, "静默 ID 无效")
		return
	}
	if err := h.workflow().DeleteSilence(c.Request.Context(), id); err != nil {
		apiShared.K8sAPIError(c, "删除 Alertmanager 静默失败: "+err.Error())
		return
	}
	apiShared.Success(c, gin.H{"id": id})
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
	apiShared.SuccessWithMessage(c, gin.H{"configured": true}, message)
}

// Notify receives Alertmanager's authenticated webhook. It deliberately does
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
	apiShared.Success(c, gin.H{"delivered": delivered, "persisted": true})
}

func (h *AlertingHandler) AutomationPolicy(c *gin.Context) {
	policy, err := h.workflow().Policy()
	if err != nil {
		apiShared.Success(c, apiShared.AlertAutomationPolicyDTO(&model.AlertAutomationPolicy{Enabled: false, MinimumSeverity: "warning", Mode: model.AlertAutomationReportOnly, CooldownMinutes: 30}))
		return
	}
	apiShared.Success(c, apiShared.AlertAutomationPolicyDTO(policy))
}

func (h *AlertingHandler) UpdateAutomationPolicy(c *gin.Context) {
	var policy model.AlertAutomationPolicy
	if err := c.ShouldBindJSON(&policy); err != nil || policy.RuntimeID == 0 || !alertingservice.ValidatePolicy(&policy) {
		apiShared.ValidationError(c, "告警自动化策略无效")
		return
	}
	result := gin.H{"policy": apiShared.AlertAutomationPolicyDTO(&policy), "synced": 0, "sync_warning": ""}
	count, err := h.workflow().UpdatePolicy(c.Request.Context(), &policy)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "无效") || strings.Contains(err.Error(), "必须选择") {
			status = http.StatusBadRequest
		}
		apiShared.Error(c, status, apiShared.CodeValidationFail, err.Error())
		return
	}
	result["synced"] = count
	apiShared.SuccessWithMessage(c, result, "告警自动化策略已保存")
}

func (h *AlertingHandler) ListAutomationEvents(c *gin.Context) {
	if apiShared.PaginationRequested(c) {
		limit, offset := apiShared.LimitOffset(c, 10, 100)
		events, total, err := h.workflow().ListEventPage(limit, offset)
		if err != nil {
			apiShared.DBError(c, "读取自动处置事件失败")
			return
		}
		apiShared.Success(c, gin.H{"items": apiShared.AlertEventsDTO(events), "total": total, "limit": limit, "offset": offset})
		return
	}

	events, err := h.workflow().ListEvents(30)
	if err != nil {
		apiShared.DBError(c, "读取告警自动化事件失败")
		return
	}
	apiShared.Success(c, apiShared.AlertEventsDTO(events))
}

func normalizeAlertingPlatformURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil {
		return defaultAlertingPlatformURL + "/#/monitoring?tab=alerts"
	}
	return strings.TrimRight(parsed.Scheme+"://"+parsed.Host, "/") + "/#/monitoring?tab=alerts"
}

func alertListKey(alert alertingservice.Alert) string {
	if alert.Fingerprint != "" {
		return alert.Fingerprint
	}
	labels, _ := json.Marshal(alert.Labels)
	return string(labels) + "|" + alert.StartsAt.UTC().Format(time.RFC3339Nano)
}
