package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var (
	scenePattern = regexp.MustCompile(`^[a-zA-Z0-9!#$&'()*+,/:;=?@._~-]{1,32}$`)
	ErrScene     = errors.New("请输入 1 到 32 个合法的推广场景字符")
)

func (s *Service) PromotionCode(ctx context.Context, owner, product, channel uint, scene string) ([]byte, string, error) {
	item, err := s.repo.WeChatConnection(owner, product, channel)
	if err != nil {
		return nil, "", err
	}
	if item.EncryptedAppSecret == "" {
		return nil, "", ErrAnalyticsNotConfigured
	}
	scene = strings.TrimSpace(scene)
	if !scenePattern.MatchString(scene) {
		return nil, "", ErrScene
	}
	var image []byte
	var mime string
	key, load := s.directToken(item)
	err = s.client.doWithToken(ctx, key, load, func(access string) error {
		target := s.client.base + "/wxa/getwxacodeunlimit?" + url.Values{"access_token": {access}}.Encode()
		payload, _ := json.Marshal(map[string]any{"scene": scene, "env_version": "release", "width": 430})
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
		if err != nil {
			return errors.New("无法创建微信码请求")
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := s.client.http.Do(request)
		if err != nil {
			return errors.New("无法连接微信 API")
		}
		defer response.Body.Close()
		body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
		if err != nil || len(body) > 1<<20 {
			return errors.New("微信码响应读取失败")
		}
		var result struct {
			Code int `json:"errcode"`
		}
		if json.Unmarshal(body, &result) == nil && result.Code != 0 {
			return APIError(result.Code)
		}
		mime = http.DetectContentType(body)
		if response.StatusCode != http.StatusOK || (mime != "image/png" && mime != "image/jpeg") {
			return errors.New("微信未返回有效的推广码图片")
		}
		image = body
		return nil
	})
	return image, mime, err
}
