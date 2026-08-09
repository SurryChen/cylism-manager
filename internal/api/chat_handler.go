package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cylism/cylism-manager/internal/agent"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

type chatRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

// chatClientFactory builds a runtime chat client; tests replace it with a fake.
type chatClientFactory func(instance *model.RuntimeInstance, apiKey string) (agent.ChatClient, error)

func (h *RuntimeHandler) chatClient(instance *model.RuntimeInstance, apiKey string) (agent.ChatClient, error) {
	if h.newChatClient != nil {
		return h.newChatClient(instance, apiKey)
	}
	adapter, ok := h.registry.Get(instance.RuntimeType)
	if !ok {
		return nil, fmt.Errorf("不支持的 Runtime 类型: %s", instance.RuntimeType)
	}
	chatURL, err := adapter.ChatEndpoint(instance)
	if err != nil {
		return nil, err
	}
	sessionURL, err := adapter.SessionEndpoint(instance)
	if err != nil {
		return nil, err
	}
	return agent.NewRuntimeChatClient(chatURL, sessionURL, instance.ModelName, apiKey), nil
}

func (h *RuntimeHandler) managedRuntime(c *gin.Context) (*model.RuntimeInstance, bool) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "Runtime ID 无效")
		return nil, false
	}
	instance, err := h.store.GetRuntime(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "Runtime 不存在")
		return nil, false
	}
	if instance.DeploymentMode != model.RuntimeDeploymentManaged {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "仅托管 Runtime 支持聊天")
		return nil, false
	}
	return instance, true
}

func (h *RuntimeHandler) requiresCapability(c *gin.Context, instance *model.RuntimeInstance, capability string) bool {
	adapter, ok := h.registry.Get(instance.RuntimeType)
	if !ok {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "不支持的 Runtime 类型")
		return false
	}
	for _, item := range adapter.Definition().Capabilities {
		if item == capability {
			return true
		}
	}
	model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "该 Runtime 不支持此能力")
	return false
}

// Chat relays the runtime's streaming reply to the browser as SSE. The runtime
// API key stays inside the Manager; client cancellation propagates through the
// request context and stops upstream generation.
func (h *RuntimeHandler) Chat(c *gin.Context) {
	instance, ok := h.managedRuntime(c)
	if !ok || !h.requiresCapability(c, instance, "chat") {
		return
	}
	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Message) == "" {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "聊天消息不能为空")
		return
	}
	apiKey, err := h.runtimeAPIKey(instance)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "读取 Runtime API 凭据失败")
		return
	}
	client, err := h.chatClient(instance, apiKey)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
		return
	}
	upstream, err := client.StreamChat(c.Request.Context(), strings.TrimSpace(req.SessionID), strings.TrimSpace(req.Message))
	if err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "连接 Runtime 聊天接口失败: "+err.Error())
		return
	}
	defer upstream.Body.Close()
	if upstream.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(upstream.Body, 4096))
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, fmt.Sprintf("Runtime 聊天接口返回 HTTP %d: %s", upstream.StatusCode, strings.TrimSpace(string(body))))
		return
	}
	header := c.Writer.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	flusher, _ := c.Writer.(http.Flusher)
	_ = agent.ForEachChatEvent(upstream.Body, func(event agent.ChatEvent) error {
		payload, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			return marshalErr
		}
		if _, writeErr := c.Writer.Write(append(append([]byte("data: "), payload...), '\n', '\n')); writeErr != nil {
			return writeErr
		}
		if flusher != nil {
			flusher.Flush()
		}
		return nil
	})
}

// ChatSessions proxies the runtime's session list.
func (h *RuntimeHandler) ChatSessions(c *gin.Context) {
	instance, ok := h.managedRuntime(c)
	if !ok || !h.requiresCapability(c, instance, "sessions") {
		return
	}
	apiKey, err := h.runtimeAPIKey(instance)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "读取 Runtime API 凭据失败")
		return
	}
	client, err := h.chatClient(instance, apiKey)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
		return
	}
	sessions, err := client.ListSessions(c.Request.Context())
	if err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "读取 Runtime 会话失败: "+err.Error())
		return
	}
	model.Success(c, sessions)
}

// ChatSessionMessages proxies one session's message history from the runtime.
func (h *RuntimeHandler) ChatSessionMessages(c *gin.Context) {
	instance, ok := h.managedRuntime(c)
	if !ok || !h.requiresCapability(c, instance, "sessions") {
		return
	}
	sessionID := strings.TrimSpace(c.Param("sid"))
	if sessionID == "" {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "会话 ID 无效")
		return
	}
	apiKey, err := h.runtimeAPIKey(instance)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "读取 Runtime API 凭据失败")
		return
	}
	client, err := h.chatClient(instance, apiKey)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
		return
	}
	detail, err := client.ReadSession(c.Request.Context(), sessionID)
	if err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "读取 Runtime 会话失败: "+err.Error())
		return
	}
	if detail == nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "会话不存在")
		return
	}
	model.Success(c, detail)
}
