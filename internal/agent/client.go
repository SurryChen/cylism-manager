package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ChatClient is the contract the chat handler relies on.
type ChatClient interface {
	StreamChat(ctx context.Context, sessionID, message string) (*http.Response, error)
	ListSessions(ctx context.Context) ([]Session, error)
	ReadSession(ctx context.Context, sessionID string, options SessionHistoryOptions) (*SessionDetail, error)
}

// RuntimeChatClient talks to one runtime's chat and session endpoints with the
// runtime API key. The key never leaves the Manager process.
type RuntimeChatClient struct {
	ChatURL    string
	SessionURL string
	Model      string
	APIKey     string
	HTTPClient *http.Client
}

func NewRuntimeChatClient(chatURL, sessionURL, model, apiKey string) *RuntimeChatClient {
	return &RuntimeChatClient{
		ChatURL:    strings.TrimRight(chatURL, "/"),
		SessionURL: strings.TrimRight(sessionURL, "/"),
		Model:      model,
		APIKey:     apiKey,
		HTTPClient: &http.Client{},
	}
}

func (c *RuntimeChatClient) StreamChat(ctx context.Context, sessionID, message string) (*http.Response, error) {
	payload := map[string]any{
		"model":    c.Model,
		"messages": []map[string]string{{"role": "user", "content": message}},
		"stream":   true,
	}
	if sessionID != "" {
		payload["session_id"] = sessionID
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("编码聊天请求: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ChatURL, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("构建聊天请求: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.APIKey)
	return c.HTTPClient.Do(request)
}

func (c *RuntimeChatClient) ListSessions(ctx context.Context) ([]Session, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.SessionURL+"/v1/sessions", nil)
	if err != nil {
		return nil, fmt.Errorf("构建会话列表请求: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.APIKey)
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("读取会话列表: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("会话列表返回 HTTP %d", response.StatusCode)
	}
	var payload struct {
		Sessions []Session `json:"sessions"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析会话列表: %w", err)
	}
	return payload.Sessions, nil
}

func (c *RuntimeChatClient) ReadSession(ctx context.Context, sessionID string, options SessionHistoryOptions) (*SessionDetail, error) {
	endpoint := c.SessionURL + "/v1/sessions/" + url.PathEscape(sessionID) + "/messages"
	query := url.Values{}
	if options.Limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", options.Limit))
	}
	if options.Before != "" {
		query.Set("before", options.Before)
	}
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("构建会话消息请求: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.APIKey)
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("读取会话消息: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("会话消息返回 HTTP %d", response.StatusCode)
	}
	var detail SessionDetail
	if err := json.NewDecoder(response.Body).Decode(&detail); err != nil {
		return nil, fmt.Errorf("解析会话消息: %w", err)
	}
	return &detail, nil
}
