package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	assistantDefaultProviderConfigKey = "assistant_default_provider_id"
	assistantProviderTypeResponses    = "openai_responses"
)

type AssistantHandler struct {
	store      *store.Store
	encKey     []byte
	runtimeURL string
	client     *http.Client
}

type assistantProviderRequest struct {
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	BaseURL      string `json:"base_url"`
	Model        string `json:"model"`
	APIKey       string `json:"api_key"`
	Enabled      bool   `json:"enabled"`
	IsDefault    bool   `json:"is_default"`
}

type assistantConversationRequest struct {
	Title string `json:"title"`
}
type assistantMessageRequest struct {
	Message     string            `json:"message"`
	PageContext map[string]string `json:"page_context"`
}

type pydanticDiagnosis struct {
	Summary    string `json:"summary"`
	Assessment string `json:"assessment"`
	Evidence   []struct {
		Source      string `json:"source"`
		Observation string `json:"observation"`
		Timestamp   string `json:"timestamp,omitempty"`
	} `json:"evidence"`
	NextSteps             []string `json:"next_steps"`
	RequiresHumanApproval bool     `json:"requires_human_approval"`
	ExecutionStatus       string   `json:"execution_status"`
}

type pydanticResponse struct {
	Diagnosis pydanticDiagnosis `json:"diagnosis"`
}

func NewAssistantHandler(s *store.Store, encKey []byte) *AssistantHandler {
	runtimeURL := strings.TrimRight(os.Getenv("CYLISM_ASSISTANT_RUNTIME_URL"), "/")
	if runtimeURL == "" {
		runtimeURL = "http://cylism-ops-agent.default.svc:8080"
	}
	return &AssistantHandler{store: s, encKey: encKey, runtimeURL: runtimeURL, client: &http.Client{Timeout: 75 * time.Second}}
}

func (h *AssistantHandler) Status(c *gin.Context) {
	providers, err := h.store.ListAssistantProviders()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取助手配置失败")
		return
	}
	defaultID, _ := h.store.GetSystemConfig(assistantDefaultProviderConfigKey)
	configured := false
	for _, provider := range providers {
		if defaultID == strconv.FormatUint(uint64(provider.ID), 10) && provider.Enabled && provider.ProviderType == assistantProviderTypeResponses {
			configured = true
			break
		}
	}
	runtimeStatus := &k8s.OpsAgentStatus{State: "unavailable", Message: "Kubernetes 集群未连接，无法读取 Runtime 状态"}
	if K8s != nil {
		runtimeStatus = K8s.OpsAgentStatus()
	}
	model.Success(c, gin.H{"runtime": "pydanticai", "configured": configured, "provider_count": len(providers), "default_provider_id": defaultID, "runtime_status": runtimeStatus})
}

func (h *AssistantHandler) ListProviders(c *gin.Context) {
	providers, err := h.store.ListAssistantProviders()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取模型提供商失败")
		return
	}
	defaultID, _ := h.store.GetSystemConfig(assistantDefaultProviderConfigKey)
	model.Success(c, gin.H{"providers": providers, "default_provider_id": defaultID})
}

func (h *AssistantHandler) CreateProvider(c *gin.Context) {
	var request assistantProviderRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "模型提供商配置无效")
		return
	}
	provider, err := h.providerFromRequest(request, nil)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.store.CreateAssistantProvider(provider); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "保存模型提供商失败")
		return
	}
	if request.IsDefault {
		_ = h.store.SetSystemConfig(assistantDefaultProviderConfigKey, strconv.FormatUint(uint64(provider.ID), 10))
		if err := h.reconcileRuntimeProvider(provider); err != nil {
			model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "模型提供商已保存，但 Runtime 更新失败: "+err.Error())
			return
		}
	}
	model.SuccessWithMessage(c, provider, "模型提供商已保存")
}

func (h *AssistantHandler) UpdateProvider(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "模型提供商 ID 无效")
		return
	}
	existing, err := h.store.GetAssistantProvider(uint(id))
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "模型提供商不存在")
		return
	}
	var request assistantProviderRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "模型提供商配置无效")
		return
	}
	provider, err := h.providerFromRequest(request, existing)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	wasDefault := h.isDefaultProvider(provider.ID)
	if !provider.Enabled && (wasDefault || request.IsDefault) {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "默认模型提供商不能停用")
		return
	}
	if err := h.store.SaveAssistantProvider(provider); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "更新模型提供商失败")
		return
	}
	if request.IsDefault {
		_ = h.store.SetSystemConfig(assistantDefaultProviderConfigKey, strconv.FormatUint(uint64(provider.ID), 10))
	}
	if wasDefault || request.IsDefault {
		if err := h.reconcileRuntimeProvider(provider); err != nil {
			model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "模型提供商已更新，但 Runtime 更新失败: "+err.Error())
			return
		}
	}
	model.SuccessWithMessage(c, provider, "模型提供商已更新")
}

func (h *AssistantHandler) DeleteProvider(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "模型提供商 ID 无效")
		return
	}
	if h.isDefaultProvider(uint(id)) {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "默认模型提供商正在被 Runtime 使用，请先切换默认模型或卸载 Runtime")
		return
	}
	if err := h.store.DeleteAssistantProvider(uint(id)); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "删除模型提供商失败")
		return
	}
	model.Success(c, nil)
}

func (h *AssistantHandler) ListConversations(c *gin.Context) {
	conversations, err := h.store.ListAssistantConversations(assistantUserID(c), 50)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取助手会话失败")
		return
	}
	model.Success(c, gin.H{"conversations": conversations})
}

func (h *AssistantHandler) CreateConversation(c *gin.Context) {
	var request assistantConversationRequest
	_ = c.ShouldBindJSON(&request)
	conversation := &model.AssistantConversation{UserID: assistantUserID(c), Runtime: "pydanticai", Title: strings.TrimSpace(request.Title)}
	if err := h.store.CreateAssistantConversation(conversation); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "创建助手会话失败")
		return
	}
	model.Success(c, conversation)
}

func (h *AssistantHandler) ListMessages(c *gin.Context) {
	conversation, ok := h.conversation(c)
	if !ok {
		return
	}
	messages, err := h.store.ListAssistantMessages(conversation.ID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取助手消息失败")
		return
	}
	model.Success(c, gin.H{"conversation": conversation, "messages": messages})
}

func (h *AssistantHandler) SendMessage(c *gin.Context) {
	conversation, ok := h.conversation(c)
	if !ok {
		return
	}
	var request assistantMessageRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(strings.TrimSpace(request.Message)) == 0 || len(request.Message) > 4000 {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "消息不能为空且不能超过 4000 个字符")
		return
	}
	userMessage := &model.AssistantMessage{ConversationID: conversation.ID, Role: "user", Content: strings.TrimSpace(request.Message)}
	if err := h.store.CreateAssistantMessage(userMessage); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存用户消息失败")
		return
	}
	diagnosis, err := h.callRuntime(c, conversation.ID, userMessage.Content, request.PageContext)
	if err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "助手诊断失败: "+err.Error())
		return
	}
	metadata, _ := json.Marshal(diagnosis)
	content := diagnosis.Summary
	if diagnosis.Assessment != "" {
		content += "\n\n" + diagnosis.Assessment
	}
	if len(diagnosis.NextSteps) > 0 {
		content += "\n\n建议：\n- " + strings.Join(diagnosis.NextSteps, "\n- ")
	}
	assistantMessage := &model.AssistantMessage{ConversationID: conversation.ID, Role: "assistant", Content: content, Metadata: string(metadata)}
	if err := h.store.CreateAssistantMessage(assistantMessage); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存助手消息失败")
		return
	}
	if conversation.Title == "" {
		conversation.Title = truncateAssistantTitle(userMessage.Content)
		_ = h.store.DB().Save(conversation).Error
	}
	model.Success(c, gin.H{"conversation": conversation, "message": assistantMessage, "diagnosis": diagnosis})
}

func (h *AssistantHandler) providerFromRequest(request assistantProviderRequest, existing *model.AssistantProvider) (*model.AssistantProvider, error) {
	name, kind, modelName := strings.TrimSpace(request.Name), strings.TrimSpace(request.ProviderType), strings.TrimSpace(request.Model)
	if name == "" || modelName == "" {
		return nil, fmt.Errorf("名称和模型不能为空")
	}
	if kind != assistantProviderTypeResponses {
		return nil, fmt.Errorf("当前仅支持 OpenAI Responses API 模型提供商")
	}
	provider := &model.AssistantProvider{Name: name, ProviderType: kind, BaseURL: strings.TrimRight(strings.TrimSpace(request.BaseURL), "/"), Model: modelName, Enabled: request.Enabled}
	if existing != nil {
		provider.ID, provider.APIKeyEncrypted = existing.ID, existing.APIKeyEncrypted
	}
	if strings.TrimSpace(request.APIKey) != "" {
		encrypted, err := crypto.Encrypt(h.encKey, strings.TrimSpace(request.APIKey))
		if err != nil {
			return nil, fmt.Errorf("加密模型密钥失败")
		}
		provider.APIKeyEncrypted = encrypted
	}
	if provider.APIKeyEncrypted == "" {
		return nil, fmt.Errorf("模型 API Key 不能为空")
	}
	return provider, nil
}

func (h *AssistantHandler) conversation(c *gin.Context) (*model.AssistantConversation, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "会话 ID 无效")
		return nil, false
	}
	conversation, err := h.store.GetAssistantConversation(uint(id), assistantUserID(c))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			model.Error(c, http.StatusNotFound, model.CodeNotFound, "助手会话不存在")
		} else {
			model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取助手会话失败")
		}
		return nil, false
	}
	return conversation, true
}

func (h *AssistantHandler) isDefaultProvider(providerID uint) bool {
	defaultID, _ := h.store.GetSystemConfig(assistantDefaultProviderConfigKey)
	return defaultID == strconv.FormatUint(uint64(providerID), 10)
}

func (h *AssistantHandler) reconcileRuntimeProvider(provider *model.AssistantProvider) error {
	if K8s == nil {
		return nil
	}
	status := K8s.OpsAgentStatus()
	if status.State == "not_installed" {
		return nil
	}
	if status.State == "unavailable" || status.NodeName == "" || status.Storage == "" {
		return fmt.Errorf("无法读取已部署 Runtime 的节点和存储配置: %s", status.Message)
	}
	apiKey, err := crypto.Decrypt(h.encKey, provider.APIKeyEncrypted)
	if err != nil {
		return fmt.Errorf("读取模型密钥失败: %w", err)
	}
	_, err = K8s.InstallOpsAgent(k8s.OpsAgentConfig{
		NodeName: status.NodeName,
		Storage:  status.Storage,
		Image:    status.Image,
		Model:    provider.Model,
		BaseURL:  provider.BaseURL,
	}, apiKey)
	return err
}

func (h *AssistantHandler) callRuntime(c *gin.Context, conversationID uint, message string, pageContext map[string]string) (*pydanticDiagnosis, error) {
	if len(pageContext) > 0 {
		encoded, _ := json.Marshal(pageContext)
		message = "当前 Cylism 页面上下文（仅供定位资源）：" + string(encoded) + "\n\n用户问题：" + message
	}
	payload, _ := json.Marshal(gin.H{"message": message, "conversation_id": strconv.FormatUint(uint64(conversationID), 10)})
	request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, h.runtimeURL+"/v1/diagnose", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", c.GetHeader("Authorization"))
	response, err := h.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Runtime 返回 %s", response.Status)
	}
	var result pydanticResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Runtime 响应失败")
	}
	if result.Diagnosis.Summary == "" {
		return nil, fmt.Errorf("Runtime 未返回诊断结果")
	}
	return &result.Diagnosis, nil
}

func assistantUserID(c *gin.Context) uint {
	userID, _ := c.Get("user_id")
	value, _ := userID.(uint)
	return value
}
func truncateAssistantTitle(value string) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) > 48 {
		return string([]rune(value)[:48]) + "..."
	}
	return value
}

type assistantInstallRequest struct {
	ProviderID       uint   `json:"provider_id"`
	NodeName         string `json:"node_name"`
	Storage          string `json:"storage"`
	StorageClassName string `json:"storage_class_name"`
	Image            string `json:"image"`
}

func (h *AssistantHandler) Install(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	var request assistantInstallRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "助手安装配置无效")
		return
	}
	if request.ProviderID == 0 {
		raw, _ := h.store.GetSystemConfig(assistantDefaultProviderConfigKey)
		value, _ := strconv.ParseUint(raw, 10, 64)
		request.ProviderID = uint(value)
	}
	provider, err := h.store.GetAssistantProvider(request.ProviderID)
	if err != nil || !provider.Enabled || provider.ProviderType != assistantProviderTypeResponses {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "请选择已启用的模型提供商")
		return
	}
	apiKey, err := crypto.Decrypt(h.encKey, provider.APIKeyEncrypted)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "读取模型密钥失败")
		return
	}
	status, err := K8s.InstallOpsAgent(k8s.OpsAgentConfig{NodeName: request.NodeName, Storage: request.Storage, StorageClassName: request.StorageClassName, Image: request.Image, Model: provider.Model, BaseURL: provider.BaseURL}, apiKey)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeK8sAPIError, err.Error())
		return
	}
	_ = h.store.SetSystemConfig(assistantDefaultProviderConfigKey, strconv.FormatUint(uint64(provider.ID), 10))
	model.SuccessWithMessage(c, status, "智能助手 Runtime 已提交部署")
}

func (h *AssistantHandler) Uninstall(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	if err := K8s.UninstallOpsAgent(); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeK8sAPIError, "卸载智能助手失败: "+err.Error())
		return
	}
	model.SuccessWithMessage(c, gin.H{"pvc_retained": true}, "智能助手 Runtime 已卸载，审计 PVC 已保留")
}
