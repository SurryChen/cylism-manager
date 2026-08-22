package auth

import (
	"testing"
	"time"
)

func TestDelegationTokenRejectsBrowserTokenAndHonorsScope(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	browser, err := GenerateAccessToken(secret, 7, "alice", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseDelegationToken(secret, browser); err != ErrTokenInvalid {
		t.Fatalf("browser JWT accepted as delegation: %v", err)
	}
	token, err := GenerateDelegationToken(secret, DelegationClaims{UserID: 7, Username: "alice", ProjectID: 9, EnvironmentIDs: []uint{4, 2, 4}, ApplicationIDs: []uint{12}, Capability: "hysteria2", Actions: []string{"application:read", "managed_file:write"}}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseDelegationToken(secret, token)
	if err != nil {
		t.Fatal(err)
	}
	if !claims.Allows("managed_file:write") || claims.Allows("application:restart") || !claims.AllowsEnvironment(2) || claims.AllowsEnvironment(3) || !claims.AllowsApplication(12) || claims.AllowsApplication(13) {
		t.Fatalf("unexpected claims scope: %#v", claims)
	}
}
