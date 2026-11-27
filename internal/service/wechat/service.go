package wechat

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	"golang.org/x/sync/singleflight"
)

var (
	appIDPattern              = regexp.MustCompile(`^wx[0-9a-fA-F]{16}$`)
	ErrAppID                  = errors.New("请输入正确的微信 AppID")
	ErrAppSecret              = errors.New("AppSecret 格式无效")
	ErrAnalyticsNotConfigured = errors.New("请先配置微信 AppSecret")
	Beijing                   = time.FixedZone("Asia/Shanghai", 8*3600)
)

type Repository interface {
	WeChatConnections(uint, uint) ([]model.ProductWeChatConnection, error)
	WeChatConnection(uint, uint, uint) (model.ProductWeChatConnection, error)
	SaveWeChatConnection(uint, uint, uint, string, string) error
	DeleteWeChatConnection(uint, uint, uint) error
	WeChatMetrics(uint, string) ([]model.WeChatDailyMetric, error)
	SaveWeChatMetrics(model.ProductWeChatConnection, []model.WeChatDailyMetric, string) error
	WeChatAnalyticsTargets() ([]model.ProductWeChatConnection, error)
	WeChatChannelPlatform(model.ProductWeChatConnection) (string, error)
}

type Service struct {
	repo   Repository
	client *Client
	key    []byte
	work   singleflight.Group
}

func New(repo Repository, key []byte) *Service {
	return &Service{repo: repo, key: key, client: newClient()}
}

type Connection struct {
	model.ProductWeChatConnection
	AnalyticsConnected bool `json:"analytics_connected"`
}

type Settings struct {
	Connections []Connection `json:"connections"`
}

func (s *Service) Settings(owner, product uint) (Settings, error) {
	items, err := s.repo.WeChatConnections(owner, product)
	settings := Settings{Connections: make([]Connection, 0, len(items))}
	for _, item := range items {
		settings.Connections = append(settings.Connections, Connection{ProductWeChatConnection: item, AnalyticsConnected: item.EncryptedAppSecret != ""})
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
