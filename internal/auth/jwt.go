package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrTokenExpired = errors.New("token expired")
	ErrTokenInvalid = errors.New("token invalid")
)

type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Exp      int64  `json:"exp"`
	Iat      int64  `json:"iat"`
}

// GenerateAccessToken 生成短期 access token (HMAC-SHA256)
func GenerateAccessToken(secret []byte, userID uint, username string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   userID,
		Username: username,
		Iat:      now.Unix(),
		Exp:      now.Add(ttl).Unix(),
	}
	return signToken(secret, claims)
}

// GenerateRefreshToken 生成长期 refresh token
func GenerateRefreshToken(secret []byte, userID uint, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Iat:    now.Unix(),
		Exp:    now.Add(ttl).Unix(),
	}
	return signToken(secret, claims)
}

// ParseToken 解析并验证 JWT
func ParseToken(secret []byte, tokenString string) (*Claims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, ErrTokenInvalid
	}

	if !validSignature(secret, parts) {
		return nil, ErrTokenInvalid
	}

	// 解码 payload
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrTokenInvalid
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrTokenInvalid
	}

	// 检查过期
	if time.Now().Unix() > claims.Exp {
		return nil, ErrTokenExpired
	}

	return &claims, nil
}

func signToken(secret []byte, claims Claims) (string, error) {
	return signPayload(secret, claims)
}

func signPayload(secret []byte, claims interface{}) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)

	signingInput := header + "." + payload
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return signingInput + "." + signature, nil
}

func validSignature(secret []byte, parts []string) bool {
	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(parts[2]), []byte(expectedSig))
}
