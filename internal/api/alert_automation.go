package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/agent"
	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/runtime"
)

// alertRuntimeDispatcher is deliberately small: the webhook handler owns
// persistence and only this interface may invoke a Runtime asynchronously.
type alertRuntimeDispatcher interface {
	Dispatch(context.Context, *model.AlertEvent)
}

// AlertRuntimeDispatcher turns a persisted, matched alert into a bounded
// Nanobot analysis. The prompt only exposes fixed CLI operations; it never
// passes shell, SSH, Kubernetes, or notification credentials to the Runtime.
type AlertRuntimeDispatcher struct {
	store interface {
		GetAlertAutomationPolicy() (*model.AlertAutomationPolicy, error)
		GetRuntime(uint) (*model.RuntimeInstance, error)
		HasAgentCapability(uint, string, string) (bool, error)
		UpdateAlertEvent(*model.AlertEvent) error
	}
	encKey   []byte
	registry *runtime.Registry
}

func NewAlertRuntimeDispatcher(store interface {
	GetAlertAutomationPolicy() (*model.AlertAutomationPolicy, error)
	GetRuntime(uint) (*model.RuntimeInstance, error)
	HasAgentCapability(uint, string, string) (bool, error)
	UpdateAlertEvent(*model.AlertEvent) error
}, encKey []byte, registry *runtime.Registry) *AlertRuntimeDispatcher {
	return &AlertRuntimeDispatcher{store: store, encKey: encKey, registry: registry}
}

func (d *AlertRuntimeDispatcher) Dispatch(ctx context.Context, event *model.AlertEvent) {
	if d == nil || event == nil {
		return
	}
	policy, err := d.store.GetAlertAutomationPolicy()
	if err != nil || !policy.Enabled || !alertPolicyMatches(policy, event) {
		return
	}
	runtimeInstance, err := d.store.GetRuntime(policy.RuntimeID)
	if err != nil || runtimeInstance.Status != model.RuntimeStatusReady || runtimeInstance.RuntimeType != model.RuntimeTypeNanobot || !runtimeInstance.AgentToolEnabled {
		d.fail(event, "告警自动化 Runtime 未就绪或未启用受控工具")
		return
	}
	for _, capability := range []string{model.AgentCapabilityAlertRead, model.AgentCapabilityMonitoringRead} {
		granted, checkErr := d.store.HasAgentCapability(runtimeInstance.ID, capability, "")
		if checkErr != nil || !granted {
			d.fail(event, "告警自动化 Runtime 缺少只读诊断授权")
			return
		}
	}
	adapter, ok := d.registry.Get(runtimeInstance.RuntimeType)
	if !ok {
		d.fail(event, "告警自动化 Runtime 类型不受支持")
		return
	}
	apiKey, err := crypto.Decrypt(d.encKey, runtimeInstance.EncryptedRuntimeAPIKey)
	if err != nil || strings.TrimSpace(apiKey) == "" {
		d.fail(event, "读取告警自动化 Runtime 凭据失败")
		return
	}
	endpoint, err := adapter.ChatEndpoint(runtimeInstance)
	if err != nil {
		d.fail(event, "构建告警自动化 Runtime 端点失败")
		return
	}
	sessionEndpoint, err := adapter.SessionEndpoint(runtimeInstance)
	if err != nil {
		d.fail(event, "构建告警自动化会话端点失败")
		return
	}
	hash := sha256.Sum256([]byte(event.Fingerprint))
	event.SessionID = "alert-" + hex.EncodeToString(hash[:12])
	event.RuntimeID = &runtimeInstance.ID
	now := time.Now().UTC()
	event.LastDispatchedAt = &now
	_ = d.store.UpdateAlertEvent(event)

	prompt := alertAutomationPrompt(event, policy.Mode)
	callCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	client := agent.NewRuntimeChatClient(endpoint, sessionEndpoint, runtimeInstance.ModelName, apiKey)
	response, err := client.StreamChat(callCtx, event.SessionID, prompt)
	if err != nil {
		d.fail(event, "连接告警自动化 Runtime 失败: "+err.Error())
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		d.fail(event, fmt.Sprintf("告警自动化 Runtime 返回 HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body))))
		return
	}
	var report strings.Builder
	err = agent.ForEachChatEvent(response.Body, func(chunk agent.ChatEvent) error {
		if chunk.Type == agent.EventDelta && report.Len()+len(chunk.Content) <= 24*1024 {
			report.WriteString(chunk.Content)
		}
		return nil
	})
	if err != nil {
		d.fail(event, "读取告警自动化报告失败: "+err.Error())
		return
	}
	event.Report = strings.TrimSpace(report.String())
	if event.Report == "" {
		d.fail(event, "告警自动化 Runtime 未生成报告")
		return
	}
	if event.OperationID != "" {
		event.Status = model.AlertEventAwaitingApproval
		event.DiagnosticSummary = "诊断完成，等待管理员审批固定清理配方"
	} else {
		event.Status = model.AlertEventFiring
		event.DiagnosticSummary = "诊断报告已生成"
	}
	event.LastError = ""
	_ = d.store.UpdateAlertEvent(event)
}

func (d *AlertRuntimeDispatcher) fail(event *model.AlertEvent, message string) {
	event.Status = model.AlertEventFailed
	event.LastError = truncateAgentText(message, 512)
	_ = d.store.UpdateAlertEvent(event)
}

func alertPolicyMatches(policy *model.AlertAutomationPolicy, event *model.AlertEvent) bool {
	if policy == nil || event == nil || !policy.Enabled || (policy.AlertName != "" && policy.AlertName != event.AlertName) {
		return false
	}
	return alertSeverityRank(event.Severity) >= alertSeverityRank(policy.MinimumSeverity)
}

func alertSeverityRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical":
		return 2
	case "warning":
		return 1
	default:
		return 0
	}
}

func alertAutomationPrompt(event *model.AlertEvent, mode string) string {
	return fmt.Sprintf(`你正在处理 Cylism 的持久化告警事件 #%d。只能使用 cylism_platform 工具，禁止建议或尝试 shell、SSH、kubectl、网络请求、任意文件操作。

先调用 alert_get（alert_id=%d）和 monitoring_disk_growth（node=%q, range=6h），必要时调用 node_get。基于返回的事实生成简洁中文报告：影响、证据、风险、建议。若告警仍为 firing 且确实适合固定配方，在模式 %q 下只可请求 maintenance_cleanup_request，recipe 仅能为 journal-vacuum 或 container-image-prune；必须说明理由。不要声称已执行清理，任何清理都需要管理员审批。`, event.ID, event.ID, event.NodeName, mode)
}
