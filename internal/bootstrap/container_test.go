package bootstrap

import (
	"testing"
	"time"
)

func TestNewContainerInitializesStoreAndAuth(t *testing.T) {
	container, err := NewContainer(Config{
		DBPath:          ":memory:",
		EncryptionKey:   []byte("01234567890123456789012345678901"),
		JWTSecret:       []byte("secret"),
		AccessTokenTTL:  time.Minute,
		RefreshTokenTTL: time.Hour,
		PlatformURL:     "https://example.test",
	})
	if err != nil {
		t.Fatalf("NewContainer: %v", err)
	}
	if container.Store == nil || container.Auth == nil {
		t.Fatal("expected store and auth dependencies")
	}
	if container.Auth.PlatformURL != "https://example.test" || container.Auth.AccessTokenTTL != time.Minute {
		t.Fatalf("unexpected auth config: %+v", container.Auth)
	}
	if string(container.configEncryptionKey()) != "01234567890123456789012345678901" {
		t.Fatal("encryption key was not retained")
	}
}
