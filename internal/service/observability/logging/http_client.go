package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type HTTPClient struct {
	BaseURL string
	Client  *http.Client
}

func (c HTTPClient) Query(ctx context.Context, path string, values url.Values) (Response, error) {
	base := strings.TrimRight(c.BaseURL, "/") + path
	if len(values) > 0 {
		base += "?" + values.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base, nil)
	if err != nil {
		return Response{}, err
	}
	client := c.Client
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("Loki 返回 %s", resp.Status)
	}
	var payload struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Stream map[string]string `json:"stream"`
				Values [][]string        `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Response{}, err
	}
	out := Response{Streams: make([]Stream, 0, len(payload.Data.Result))}
	for _, s := range payload.Data.Result {
		out.Streams = append(out.Streams, Stream{Labels: s.Stream, Values: s.Values})
	}
	return out, nil
}
