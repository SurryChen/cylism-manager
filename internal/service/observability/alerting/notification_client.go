package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SendFeishuJSON sends a pre-rendered Feishu card through the validated
// webhook. Card rendering remains a presentation concern of the API layer;
// network transport is owned by this service.
func SendFeishuJSON(ctx context.Context, webhook string, card interface{}) error {
	body, err := json.Marshal(card)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("飞书返回 %s", resp.Status)
	}
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result); err == nil && result.Code != 0 {
		return fmt.Errorf("飞书返回 %d: %s", result.Code, result.Msg)
	}
	return nil
}
