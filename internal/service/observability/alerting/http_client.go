package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPClient is the Alertmanager transport. Endpoint construction and
// timeouts live here so handlers only coordinate request/response mapping.
type HTTPClient struct {
	BaseURL func() string
	Client  *http.Client
	MaxBody int64
}

func (c *HTTPClient) Request(ctx context.Context, method, path string, input, output interface{}) error {
	if c == nil || c.BaseURL == nil {
		return context.Canceled
	}
	var body io.Reader
	if input != nil {
		payload, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("编码请求失败: %w", err)
		}
		body = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL(), "/")+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return fmt.Errorf("Alertmanager 返回 %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	if output != nil && response.StatusCode != http.StatusNoContent {
		limit := c.MaxBody
		if limit <= 0 {
			limit = 512 << 10
		}
		if err := json.NewDecoder(io.LimitReader(response.Body, limit)).Decode(output); err != nil {
			return fmt.Errorf("解析 Alertmanager 响应失败: %w", err)
		}
	}
	return nil
}
