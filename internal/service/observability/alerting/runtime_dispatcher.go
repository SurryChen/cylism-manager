package alerting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/runtime"
	runtimechat "github.com/cylism/cylism-manager/internal/runtime/chat"
	crypto "github.com/cylism/cylism-manager/internal/security"
)

// RuntimeDispatcher turns a persisted, matched alert into a bounded Nanobot
// analysis. The prompt only exposes fixed CLI operations; it never passes
// shell, SSH, Kubernetes, or notification credentials to the Runtime.
type RuntimeDispatcher struct {
	store interface {
		GetAlertAutomationPolicy() (*model.AlertAutomationPolicy, error)
		GetRuntime(uint) (*model.RuntimeInstance, error)
		HasAgentCapability(uint, string, string) (bool, error)
		UpdateAlertEvent(*model.AlertEvent) error
	}
	encKey   []byte
	registry *runtime.Registry
}

func NewRuntimeDispatcher(store interface {
	GetAlertAutomationPolicy() (*model.AlertAutomationPolicy, error)
	GetRuntime(uint) (*model.RuntimeInstance, error)
	HasAgentCapability(uint, string, string) (bool, error)
	UpdateAlertEvent(*model.AlertEvent) error
}, encKey []byte, registry *runtime.Registry) *RuntimeDispatcher {
	return &RuntimeDispatcher{store: store, encKey: encKey, registry: registry}
}

func (d *RuntimeDispatcher) Dispatch(ctx context.Context, event *model.AlertEvent) {
	if d == nil || event == nil {
		return
	}
	policy, err := d.store.GetAlertAutomationPolicy()
	if err != nil || !policy.Enabled || !Matches(policy, event) {
		return
	}
	runtimeInstance, err := d.store.GetRuntime(policy.RuntimeID)
	if err != nil || runtimeInstance.Status != model.RuntimeStatusReady || runtimeInstance.RuntimeType != model.RuntimeTypeNanobot || !runtimeInstance.AgentToolEnabled {
		d.fail(event, "告警自动化 Runtime 未就绪或未启用受控工具")
		return
	}
	for _, capability := range []string{model.AgentCapabilityAlertRead, model.AgentCapabilityMonitoringRead, model.AgentCapabilityMaintenanceInspect} {
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

	prompt := AutomationPrompt(event, policy.Mode)
	callCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	client := runtimechat.NewRuntimeChatClient(endpoint, sessionEndpoint, runtimeInstance.ModelName, apiKey)
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
	err = runtimechat.ForEachChatEvent(response.Body, func(chunk runtimechat.ChatEvent) error {
		if chunk.Type == runtimechat.EventDelta && report.Len()+len(chunk.Content) <= 24*1024 {
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

func (d *RuntimeDispatcher) fail(event *model.AlertEvent, message string) {
	event.Status = model.AlertEventFailed
	event.LastError = truncate(message, 512)
	_ = d.store.UpdateAlertEvent(event)
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max]
}

// AutomationPrompt creates the bounded, read-only diagnostic prompt for an
// alert automation Runtime.
func AutomationPrompt(event *model.AlertEvent, mode string) string {
	return fmt.Sprintf(`你正在处理 Cylism 的持久化告警事件 #%d。只能使用 cylism_platform 工具，禁止建议或尝试 shell、SSH、kubectl、网络请求、任意文件操作。

	工具调用契约：cylism_platform(operation="alert_get", alert_id=%d) 读取本事件；cylism_platform(operation="alert_list") 列出最近事件；cylism_platform(operation="monitoring_disk_growth", node=%q, range="6h") 查询增长趋势；必须调用 cylism_platform(operation="maintenance_disk_inspect", node=%q) 读取固定目录、journal 和文件系统占用，再判断是否适配清理配方。必要时才使用 operation="node_get" 并传 name。alert_get/list 中的 automation_status（以及兼容字段 status）表示自动化生命周期，例如 analyzing、awaiting_approval，绝不能据此判断告警是否恢复；只有 alert_state 才表示真实告警状态。判断节点磁盘压力时，以 node_get 返回的 DiskPressure 条件为准，不要把 KubeletHasNoDiskPressure 指标作为节点条件。仅当 alert_state="firing"、磁盘巡检证据与固定配方匹配时，才可在模式 %q 下调用 cylism_platform(operation="maintenance_cleanup_request", alert_id=%d, recipe="journal-vacuum"、"container-image-prune" 或 "docker-image-prune")；journal 占用高才考虑 journal-vacuum，K3s/containerd 运行时目录或未使用镜像占用高才考虑 container-image-prune，/var/lib/docker（尤其 overlay2）占用高才考虑 docker-image-prune。Docker 配方只清理未使用镜像，不清理容器或卷；业务/PVC/未知数据一律只报告不清理。基于返回的事实生成简洁中文报告：影响、证据、风险、建议。不要声称已执行清理，任何清理都需要管理员审批。`, event.ID, event.ID, event.NodeName, event.NodeName, mode, event.ID)
}
