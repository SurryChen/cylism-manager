package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
)

const (
	DefaultTemporaryLoginTTL = time.Hour
	MaxTemporaryLoginTTL     = 7 * 24 * time.Hour
)

var ErrTemporaryLoginInvalid = errors.New("临时登录秘钥无效或已过期")

type TemporaryTokenService struct {
	users  repository.UserRepository
	tokens repository.TemporaryLoginTokenRepository
}

func NewTemporaryTokenService(users repository.UserRepository, tokens repository.TemporaryLoginTokenRepository) *TemporaryTokenService {
	return &TemporaryTokenService{users: users, tokens: tokens}
}

type TemporaryTokenView struct {
	ID         uint       `json:"id"`
	Label      string     `json:"label"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	Status     string     `json:"status"`
}

type CreatedTemporaryToken struct {
	TemporaryTokenView
	Token string `json:"token"`
}

func (s *TemporaryTokenService) Create(userID uint, label string, ttl time.Duration) (*CreatedTemporaryToken, error) {
	if userID == 0 {
		return nil, errors.New("用户未认证")
	}
	if ttl <= 0 {
		ttl = DefaultTemporaryLoginTTL
	}
	if ttl > MaxTemporaryLoginTTL {
		ttl = MaxTemporaryLoginTTL
	}
	plain, err := randomToken()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	entity := &model.TemporaryLoginToken{
		TokenHash: hashToken(plain), CreatedBy: userID,
		Label: strings.TrimSpace(label), ExpiresAt: now.Add(ttl),
	}
	if err := s.tokens.CreateTemporaryLoginToken(entity); err != nil {
		return nil, err
	}
	return &CreatedTemporaryToken{TemporaryTokenView: toView(*entity, now), Token: plain}, nil
}

func (s *TemporaryTokenService) List(userID uint) ([]TemporaryTokenView, error) {
	items, err := s.tokens.ListTemporaryLoginTokens(userID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	result := make([]TemporaryTokenView, 0, len(items))
	for _, item := range items {
		result = append(result, toView(item, now))
	}
	return result, nil
}

func (s *TemporaryTokenService) Revoke(userID, tokenID uint) error {
	return s.tokens.RevokeTemporaryLoginToken(userID, tokenID, time.Now().UTC())
}

func (s *TemporaryTokenService) Redeem(token string) (*model.User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrTemporaryLoginInvalid
	}
	now := time.Now().UTC()
	entity, err := s.tokens.FindActiveTemporaryLoginToken(hashToken(token), now)
	if err != nil {
		return nil, ErrTemporaryLoginInvalid
	}
	user, err := s.users.GetUserByID(entity.CreatedBy)
	if err != nil || user == nil {
		return nil, ErrTemporaryLoginInvalid
	}
	_ = s.tokens.MarkTemporaryLoginTokenUsed(entity.TokenHash, now)
	return user, nil
}

func GenerateSessionTokens(secret []byte, user *model.User, accessTTL, refreshTTL time.Duration) (string, string, error) {
	access, err := GenerateAccessToken(secret, user.ID, user.Username, accessTTL)
	if err != nil {
		return "", "", err
	}
	refresh, err := GenerateRefreshToken(secret, user.ID, refreshTTL)
	if err != nil {
		return "", "", err
	}
	return access, refresh, nil
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "cylism_tmp_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func toView(item model.TemporaryLoginToken, now time.Time) TemporaryTokenView {
	status := "active"
	if item.RevokedAt != nil {
		status = "revoked"
	} else if !item.ExpiresAt.After(now) {
		status = "expired"
	}
	return TemporaryTokenView{ID: item.ID, Label: item.Label, ExpiresAt: item.ExpiresAt, RevokedAt: item.RevokedAt, LastUsedAt: item.LastUsedAt, CreatedAt: item.CreatedAt, Status: status}
}
