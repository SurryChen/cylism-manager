package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPClient struct {
	BaseURL func() string
	Client  *http.Client
	Timeout time.Duration
}

func (c *HTTPClient) Query(ctx context.Context, path string, values url.Values) (interface{}, error) {
	if c == nil || c.BaseURL == nil {
		return nil, fmt.Errorf("VictoriaMetrics 查询不可用")
	}
	u := strings.TrimRight(c.BaseURL(), "/") + path
	if len(values) > 0 {
		u += "?" + values.Encode()
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	client := c.Client
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("VictoriaMetrics 返回 %s", resp.Status)
	}
	var payload struct {
		Status string      `json:"status"`
		Data   interface{} `json:"data"`
		Error  string      `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析 VictoriaMetrics 响应失败: %w", err)
	}
	if payload.Status != "success" {
		if payload.Error == "" {
			payload.Error = "VictoriaMetrics 查询未成功"
		}
		return nil, fmt.Errorf("%s", payload.Error)
	}
	return payload.Data, nil
}
