package wechat

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

const (
	EventPath         = "/api/wechat/component/events"
	AuthorizationPath = "/api/wechat/component/authorized"
)

var (
	appIDPattern              = regexp.MustCompile(`^wx[0-9a-fA-F]{16}$`)
	ErrComponentUnavailable   = errors.New("管理员需先配置微信开放平台第三方应用")
	ErrTicketUnavailable      = errors.New("尚未收到微信授权事件票据，请检查授权事件接收 URL")
	ErrReleaseScope           = errors.New("微信未授予版本管理权限（权限集 18）")
	ErrAppID                  = errors.New("请输入正确的微信 AppID")
	ErrAppSecret              = errors.New("AppSecret 格式无效")
	ErrAnalyticsNotConfigured = errors.New("请先配置微信 AppSecret")
	Beijing                   = time.FixedZone("Asia/Shanghai", 8*3600)
)

type ComponentConfig struct {
	AppID, AppSecret, EventToken, EncodingAESKey string
}

type Repository interface {
	WeChatConnections(uint, uint) ([]model.ProductWeChatConnection, error)
	WeChatConnection(uint, uint, uint) (model.ProductWeChatConnection, error)
	SaveWeChatConnection(uint, uint, uint, string, string) error
	DeleteWeChatConnection(uint, uint, uint) error
	SaveWeChatTicket(*model.WeChatComponentTicket) error
	WeChatTicket(string) (model.WeChatComponentTicket, error)
	StartWeChatAuthorization(model.ProductWeChatConnection, string, time.Time) error
	ClaimWeChatAuthorization(string, time.Time) (model.ProductWeChatConnection, error)
	FinishWeChatAuthorization(model.ProductWeChatConnection, string, string, int64) error
	RevokeWeChatAuthorization(string, string, int64) error
	RefreshWeChatGrant(model.ProductWeChatConnection, string) error
	WeChatReleaseSyncTargets(string, time.Time) ([]model.ProductWeChatConnection, error)
	RecordWeChatReleaseError(model.ProductWeChatConnection, string, time.Time) error
	ImportWeChatRelease(model.ProductWeChatConnection, model.WeChatReleaseSnapshot, time.Time) error
	WeChatMetrics(uint, string) ([]model.WeChatDailyMetric, error)
	SaveWeChatMetrics(model.ProductWeChatConnection, []model.WeChatDailyMetric, string) error
	WeChatAnalyticsTargets() ([]model.ProductWeChatConnection, error)
	WeChatChannelPlatform(model.ProductWeChatConnection) (string, error)
}

type Service struct {
	repo      Repository
	client    *Client
	key       []byte
	publicURL string
	component ComponentConfig
	work      singleflight.Group
}

func New(repo Repository, key []byte, publicURL string, component ComponentConfig) *Service {
	return &Service{repo: repo, key: key, client: newClient(), publicURL: validPublicURL(publicURL), component: component}
}

func validPublicURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Trim(u.Path, "/") != "" {
		return ""
	}
	return strings.TrimRight(u.String(), "/")
}

func (s *Service) ComponentAvailable() bool {
	c := s.component
	key, err := base64.RawStdEncoding.DecodeString(c.EncodingAESKey)
	return s.publicURL != "" && appIDPattern.MatchString(c.AppID) && c.AppSecret != "" && c.EventToken != "" && err == nil && len(key) == 32
}

type Connection struct {
	model.ProductWeChatConnection
	AnalyticsConnected bool `json:"analytics_connected"`
	ReleaseConnected   bool `json:"release_connected"`
}

type Settings struct {
	Connections        []Connection `json:"connections"`
	ComponentAvailable bool         `json:"component_available"`
	EventURL           string       `json:"event_url,omitempty"`
	AuthorizationURL   string       `json:"authorization_url,omitempty"`
}

func (s *Service) Settings(owner, product uint) (Settings, error) {
	items, err := s.repo.WeChatConnections(owner, product)
	settings := Settings{Connections: make([]Connection, 0, len(items)), ComponentAvailable: s.ComponentAvailable()}
	if s.publicURL != "" {
		settings.EventURL = s.publicURL + EventPath
		settings.AuthorizationURL = s.publicURL + AuthorizationPath
	}
	for _, item := range items {
		settings.Connections = append(settings.Connections, Connection{ProductWeChatConnection: item, AnalyticsConnected: item.EncryptedAppSecret != "",
			ReleaseConnected: settings.ComponentAvailable && item.ComponentAppID == s.component.AppID && item.EncryptedRefreshToken != ""})
	}
	return settings, err
}

func (s *Service) Connect(owner, product, channel uint, appID, secret string) error {
	appID, secret = strings.TrimSpace(appID), strings.TrimSpace(secret)
	if !appIDPattern.MatchString(appID) {
		return ErrAppID
	}
	ciphertext := ""
	if secret != "" {
		if len(secret) > 256 || strings.ContainsAny(secret, " \r\n\t") {
			return ErrAppSecret
		}
		var err error
		ciphertext, err = security.Encrypt(s.key, secret)
		if err != nil {
			return err
		}
	}
	return s.repo.SaveWeChatConnection(owner, product, channel, strings.ToLower(appID), ciphertext)
}

func (s *Service) Disconnect(owner, product, channel uint) error {
	return s.repo.DeleteWeChatConnection(owner, product, channel)
}

func (s *Service) ReceiveEvent(body []byte, timestamp, nonce, signature string) error {
	if !s.ComponentAvailable() {
		return ErrComponentUnavailable
	}
	event, err := decryptEvent(body, s.component, timestamp, nonce, signature, time.Now())
	if err != nil {
		return err
	}
	switch event.Type {
	case "component_verify_ticket":
		if event.Ticket == "" || len(event.Ticket) > 4096 {
			return ErrInvalidEvent
		}
		ciphertext, err := security.Encrypt(s.key, event.Ticket)
		if err != nil {
			return err
		}
		return s.repo.SaveWeChatTicket(&model.WeChatComponentTicket{AppID: s.component.AppID, EncryptedTicket: ciphertext, IssuedAt: event.CreatedAt})
	case "unauthorized":
		if !appIDPattern.MatchString(event.AuthorizerAppID) {
			return ErrInvalidEvent
		}
		return s.repo.RevokeWeChatAuthorization(s.component.AppID, event.AuthorizerAppID, event.CreatedAt)
	default:
		return nil
	}
}

func hashState(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Service) componentCall(ctx context.Context, path string, input, output any) error {
	if !s.ComponentAvailable() {
		return ErrComponentUnavailable
	}
	return s.client.doWithToken(ctx, "component:"+s.component.AppID, s.loadComponentToken, func(access string) error {
		return s.client.callWithCredential(ctx, http.MethodPost, path, "component_access_token", access, input, output)
	})
}

func (s *Service) loadComponentToken(ctx context.Context) (string, int, error) {
	item, err := s.repo.WeChatTicket(s.component.AppID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", 0, ErrTicketUnavailable
	}
	if err != nil {
		return "", 0, err
	}
	ticket, err := security.Decrypt(s.key, item.EncryptedTicket)
	if err != nil {
		return "", 0, err
	}
	var output struct {
		Token   string `json:"component_access_token"`
		Expires int    `json:"expires_in"`
	}
	err = s.client.call(ctx, http.MethodPost, "/cgi-bin/component/api_component_token", "", map[string]string{
		"component_appid": s.component.AppID, "component_appsecret": s.component.AppSecret, "component_verify_ticket": ticket,
	}, &output)
	return output.Token, output.Expires, err
}

func (s *Service) StartAuthorization(ctx context.Context, owner, product, channel uint) (string, error) {
	item, err := s.repo.WeChatConnection(owner, product, channel)
	if err != nil {
		return "", err
	}
	var output struct {
		Code string `json:"pre_auth_code"`
	}
	err = s.componentCall(ctx, "/cgi-bin/component/api_create_preauthcode",
		map[string]string{"component_appid": s.component.AppID}, &output)
	if err != nil {
		return "", err
	}
	if output.Code == "" {
		return "", model.ErrWeChatAuthorization
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	state := hex.EncodeToString(random[:])
	if err := s.repo.StartWeChatAuthorization(item, hashState(state), time.Now().Add(15*time.Minute)); err != nil {
		return "", err
	}
	callback := s.publicURL + AuthorizationPath + "?" + url.Values{"state": {state}}.Encode()
	return "https://mp.weixin.qq.com/cgi-bin/componentloginpage?" + url.Values{
		"component_appid": {s.component.AppID}, "pre_auth_code": {output.Code},
		"redirect_uri": {callback}, "auth_type": {"2"}, "biz_appid": {item.AppID},
	}.Encode(), nil
}

type authorization struct {
	AppID   string `json:"authorizer_appid"`
	Access  string `json:"authorizer_access_token"`
	Refresh string `json:"authorizer_refresh_token"`
	Expires int    `json:"expires_in"`
	Scopes  []struct {
		Category struct {
			ID int `json:"id"`
		} `json:"funcscope_category"`
	} `json:"func_info"`
}

func (s *Service) FinishAuthorization(ctx context.Context, state, code string) (model.ProductWeChatConnection, error) {
	if !s.ComponentAvailable() {
		return model.ProductWeChatConnection{}, ErrComponentUnavailable
	}
	if len(state) != 64 {
		return model.ProductWeChatConnection{}, model.ErrWeChatAuthorization
	}
	item, err := s.repo.ClaimWeChatAuthorization(hashState(state), time.Now())
	if err != nil {
		return model.ProductWeChatConnection{}, err
	}
	if code == "" || len(code) > 1024 {
		return item, model.ErrWeChatAuthorization
	}
	var output struct {
		Authorization authorization `json:"authorization_info"`
	}
	err = s.componentCall(ctx, "/cgi-bin/component/api_query_auth", map[string]string{
		"component_appid": s.component.AppID, "authorization_code": code,
	}, &output)
	if err != nil {
		return item, err
	}
	grant := output.Authorization
	if grant.AppID != item.AppID || grant.Refresh == "" {
		return item, model.ErrWeChatAuthorization
	}
	allowed := false
	for _, scope := range grant.Scopes {
		if scope.Category.ID == 18 {
			allowed = true
			break
		}
	}
	if !allowed {
		return item, ErrReleaseScope
	}
	ciphertext, err := security.Encrypt(s.key, grant.Refresh)
	if err != nil {
		return item, err
	}
	grantedAt := time.Now().Unix()
	if err = s.repo.FinishWeChatAuthorization(item, s.component.AppID, ciphertext, grantedAt); err != nil {
		return item, err
	}
	item.ComponentAppID, item.EncryptedRefreshToken, item.AuthorizedAt = s.component.AppID, ciphertext, grantedAt
	if grant.Access != "" && grant.Expires > 60 && grant.Expires <= 86400 {
		s.client.mu.Lock()
		s.client.tokens[grantKey(item)] = token{grant.Access, time.Now().Add(time.Duration(grant.Expires)*time.Second - time.Minute)}
		s.client.mu.Unlock()
	}
	return item, nil
}

func grantKey(item model.ProductWeChatConnection) string {
	return fmt.Sprintf("authorizer:%s:%s:%d", item.ComponentAppID, item.AppID, item.AuthorizedAt)
}

func (s *Service) authorizerToken(item model.ProductWeChatConnection) func(context.Context) (string, int, error) {
	return func(ctx context.Context) (string, int, error) {
		refresh, err := security.Decrypt(s.key, item.EncryptedRefreshToken)
		if err != nil {
			return "", 0, err
		}
		var output authorization
		err = s.componentCall(ctx, "/cgi-bin/component/api_authorizer_token", map[string]string{
			"component_appid": s.component.AppID, "authorizer_appid": item.AppID, "authorizer_refresh_token": refresh,
		}, &output)
		if err != nil {
			return "", 0, err
		}
		if output.Refresh != "" && output.Refresh != refresh {
			encrypted, err := security.Encrypt(s.key, output.Refresh)
			if err != nil {
				return "", 0, err
			}
			if err := s.repo.RefreshWeChatGrant(item, encrypted); err != nil {
				return "", 0, err
			}
			item.EncryptedRefreshToken = encrypted
		}
		return output.Access, output.Expires, nil
	}
}

func (s *Service) SyncRelease(ctx context.Context, owner, product, channel uint) error {
	item, err := s.repo.WeChatConnection(owner, product, channel)
	if err != nil {
		return err
	}
	return s.syncRelease(ctx, item)
}

func (s *Service) syncRelease(ctx context.Context, item model.ProductWeChatConnection) error {
	if !s.ComponentAvailable() {
		return ErrComponentUnavailable
	}
	if item.ComponentAppID != s.component.AppID || item.EncryptedRefreshToken == "" {
		return model.ErrWeChatAuthorization
	}
	_, err, _ := s.work.Do("release:"+grantKey(item), func() (any, error) {
		var output struct {
			Release struct {
				Version string `json:"release_version"`
				Stamp   int64  `json:"release_time"`
				Notes   string `json:"release_desc"`
			} `json:"release_info"`
			Trial struct {
				Version string `json:"exp_version"`
			} `json:"exp_info"`
		}
		var audit struct {
			Status *int   `json:"status"`
			Reason string `json:"reason"`
		}
		key, load := grantKey(item), s.authorizerToken(item)
		err := s.client.doWithToken(ctx, key, load, func(access string) error {
			return s.client.call(ctx, http.MethodPost, "/wxa/getversioninfo", access, struct{}{}, &output)
		})
		if err != nil {
			_ = s.repo.RecordWeChatReleaseError(item, err.Error(), time.Now())
			return nil, err
		}
		snapshot := model.WeChatReleaseSnapshot{
			OnlineVersion: limitText(output.Release.Version, 128), OnlineNotes: limitText(output.Release.Notes, 8000),
			TrialVersion: limitText(output.Trial.Version, 128),
		}
		if err := s.client.doWithToken(ctx, key, load, func(access string) error {
			return s.client.call(ctx, http.MethodGet, "/wxa/get_latest_auditstatus", access, nil, &audit)
		}); err != nil {
			snapshot.AuditError = "最近审核状态查询失败。 " + err.Error()
		} else {
			snapshot.AuditStatus, snapshot.AuditReason = audit.Status, limitText(audit.Reason, 2000)
		}
		now := time.Now()
		if output.Release.Stamp > 0 && output.Release.Stamp <= now.Add(5*time.Minute).Unix() {
			stamp := time.Unix(output.Release.Stamp, 0).UTC()
			snapshot.OnlineAt = &stamp
		}
		if err := s.repo.ImportWeChatRelease(item, snapshot, now); err != nil {
			_ = s.repo.RecordWeChatReleaseError(item, "微信版本已读取，登记发行记录失败", now)
			return nil, err
		}
		return nil, nil
	})
	return err
}

func limitText(value string, max int) string {
	r := []rune(strings.TrimSpace(value))
	if len(r) > max {
		r = r[:max]
	}
	return string(r)
}

func (s *Service) PollReleases(ctx context.Context) {
	if !s.ComponentAvailable() {
		return
	}
	items, err := s.repo.WeChatReleaseSyncTargets(s.component.AppID, time.Now().Add(-4*time.Minute))
	if err != nil {
		log.Print("微信版本同步列表读取失败")
		return
	}
	for _, item := range items {
		if ctx.Err() != nil {
			return
		}
		if err := s.syncRelease(ctx, item); err != nil {
			log.Printf("微信版本同步失败，渠道 %d", item.ChannelID)
		}
	}
}
