package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type APIError int

func (e APIError) Error() string { return fmt.Sprintf("微信 API 返回错误码 %d", e) }

type token struct {
	value  string
	expiry time.Time
}

type Client struct {
	base    string
	http    *http.Client
	mu      sync.Mutex
	tokens  map[string]token
	refresh singleflight.Group
}

func newClient() *Client {
	return &Client{
		base:   "https://api.weixin.qq.com",
		http:   &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }},
		tokens: make(map[string]token),
	}
}

func (c *Client) call(ctx context.Context, method, path, access string, input, output any) error {
	return c.callWithCredential(ctx, method, path, "access_token", access, input, output)
}

func (c *Client) callWithCredential(ctx context.Context, method, path, credential, access string, input, output any) error {
	target := c.base + path
	if access != "" {
		target += "?" + url.Values{credential: {access}}.Encode()
	}
	var data []byte
	if input != nil {
		var err error
		data, err = json.Marshal(input)
		if err != nil {
			return err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(data))
	if err != nil {
		return errors.New("无法创建微信 API 请求")
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return errors.New("无法连接微信 API")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("微信 API 返回 HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 {
		return errors.New("微信 API 响应读取失败")
	}
	var result struct {
		Code int `json:"errcode"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return errors.New("微信 API 返回无效 JSON")
	}
	if result.Code != 0 {
		return APIError(result.Code)
	}
	if json.Unmarshal(raw, output) != nil {
		return errors.New("微信 API 返回无效数据")
	}
	return nil
}

func (c *Client) cachedToken(ctx context.Context, key string, load func(context.Context) (string, int, error)) (string, error) {
	c.mu.Lock()
	current := c.tokens[key]
	c.mu.Unlock()
	if time.Now().Before(current.expiry) {
		return current.value, nil
	}
	ch := c.refresh.DoChan(key, func() (any, error) {
		c.mu.Lock()
		current := c.tokens[key]
		c.mu.Unlock()
		if time.Now().Before(current.expiry) {
			return current.value, nil
		}
		// Use a bounded context independent of an individual page refresh.
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 25*time.Second)
		defer cancel()
		value, expires, err := load(loadCtx)
		if err != nil {
			return "", err
		}
		if value == "" || expires <= 0 || expires > 86400 {
			return "", errors.New("微信未返回有效调用凭证")
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		for k, item := range c.tokens {
			if time.Now().After(item.expiry) {
				delete(c.tokens, k)
			}
		}
		c.tokens[key] = token{value, time.Now().Add(time.Duration(expires)*time.Second - time.Minute)}
		return value, nil
	})
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-ch:
		if r.Err != nil {
			return "", r.Err
		}
		return r.Val.(string), nil
	}
}

func invalidToken(err error) bool {
	return errors.Is(err, APIError(40001)) || errors.Is(err, APIError(40014)) || errors.Is(err, APIError(42001))
}

func (c *Client) doWithToken(ctx context.Context, key string, load func(context.Context) (string, int, error), request func(string) error) error {
	access, err := c.cachedToken(ctx, key, load)
	if err != nil {
		return err
	}
	err = request(access)
	if !invalidToken(err) {
		return err
	}
	c.mu.Lock()
	if c.tokens[key].value == access {
		delete(c.tokens, key)
	}
	c.mu.Unlock()
	access, err = c.cachedToken(ctx, key, load)
	if err != nil {
		return err
	}
	return request(access)
}
