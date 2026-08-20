package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

const (
	DelegationTokenType     = "delegation"
	DelegationTokenAudience = "cylism-integration"
	MaxDelegationTTL        = 10 * time.Minute
)

// DelegationClaims describes the narrow authority granted to an external
// management UI. It deliberately has a different type and audience from a
// browser JWT so the two credentials cannot be substituted for each other.
type DelegationClaims struct {
	Type           string   `json:"typ"`
	Audience       string   `json:"aud"`
	UserID         uint     `json:"user_id"`
	Username       string   `json:"username"`
	ProjectID      uint     `json:"project_id"`
	EnvironmentIDs []uint   `json:"environment_ids"`
	Capability     string   `json:"capability"`
	Actions        []string `json:"actions"`
	JTI            string   `json:"jti"`
	Exp            int64    `json:"exp"`
	Iat            int64    `json:"iat"`
}

func GenerateDelegationToken(secret []byte, claims DelegationClaims, ttl time.Duration) (string, error) {
	if ttl <= 0 || ttl > MaxDelegationTTL {
		ttl = MaxDelegationTTL
	}
	claims.Type = DelegationTokenType
	claims.Audience = DelegationTokenAudience
	claims.Capability = strings.ToLower(strings.TrimSpace(claims.Capability))
	claims.Actions = normalizedDelegationActions(claims.Actions)
	claims.EnvironmentIDs = normalizedEnvironmentIDs(claims.EnvironmentIDs)
	if claims.JTI == "" {
		jti, err := newDelegationID()
		if err != nil {
			return "", err
		}
		claims.JTI = jti
	}
	now := time.Now()
	claims.Iat = now.Unix()
	claims.Exp = now.Add(ttl).Unix()
	if !claims.valid(now) {
		return "", ErrTokenInvalid
	}
	return signPayload(secret, claims)
}

func ParseDelegationToken(secret []byte, tokenString string) (*DelegationClaims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 || !validSignature(secret, parts) {
		return nil, ErrTokenInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrTokenInvalid
	}
	var claims DelegationClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrTokenInvalid
	}
	if time.Now().Unix() > claims.Exp {
		return nil, ErrTokenExpired
	}
	if !claims.valid(time.Now()) {
		return nil, ErrTokenInvalid
	}
	return &claims, nil
}

func (c DelegationClaims) Allows(action string) bool {
	for _, candidate := range c.Actions {
		if candidate == action {
			return true
		}
	}
	return false
}

func (c DelegationClaims) AllowsEnvironment(id uint) bool {
	if len(c.EnvironmentIDs) == 0 {
		return true
	}
	index := sort.Search(len(c.EnvironmentIDs), func(index int) bool { return c.EnvironmentIDs[index] >= id })
	return index < len(c.EnvironmentIDs) && c.EnvironmentIDs[index] == id
}

func (c DelegationClaims) valid(now time.Time) bool {
	return c.Type == DelegationTokenType && c.Audience == DelegationTokenAudience && c.UserID != 0 && c.ProjectID != 0 && c.Capability != "" && c.JTI != "" && len(c.Actions) > 0 && c.Exp > now.Unix() && c.Iat <= now.Unix() && c.Exp-c.Iat <= int64(MaxDelegationTTL/time.Second)
}

func normalizedDelegationActions(actions []string) []string {
	seen := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		action = strings.TrimSpace(action)
		if action != "" {
			seen[action] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for action := range seen {
		result = append(result, action)
	}
	sort.Strings(result)
	return result
}

func normalizedEnvironmentIDs(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if id != 0 {
			seen[id] = struct{}{}
		}
	}
	result := make([]uint, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func newDelegationID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
