package auth

import (
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
)

func TestTemporaryTokenCreateRedeemAndRevoke(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	user := &model.User{Username: "owner", PasswordHash: "hash"}
	if err := db.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	service := NewTemporaryTokenService(db, db)
	created, err := service.Create(user.ID, "review", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if created.Token == "" || created.ID == 0 {
		t.Fatalf("unexpected token view: %#v", created)
	}
	redeemed, err := service.Redeem(created.Token)
	if err != nil || redeemed.ID != user.ID {
		t.Fatalf("redeem = %#v, %v", redeemed, err)
	}
	items, err := service.List(user.ID)
	if err != nil || len(items) != 1 || items[0].LastUsedAt == nil {
		t.Fatalf("list = %#v, %v", items, err)
	}
	if err := service.Revoke(user.ID, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Redeem(created.Token); err == nil {
		t.Fatal("revoked token was accepted")
	}
}

func TestTemporaryTokenExpiresAndSessionTokensAreNormalJWT(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	user := &model.User{Username: "owner", PasswordHash: "hash"}
	if err := db.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	service := NewTemporaryTokenService(db, db)
	expired := "expired-token"
	if err := db.CreateTemporaryLoginToken(&model.TemporaryLoginToken{TokenHash: hashToken(expired), CreatedBy: user.ID, ExpiresAt: time.Now().UTC().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Redeem(expired); err == nil {
		t.Fatal("expired token was accepted")
	}
	access, refresh, err := GenerateSessionTokens([]byte("secret"), user, time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseToken([]byte("secret"), access)
	if err != nil || claims.UserID != user.ID {
		t.Fatalf("access claims = %#v, %v", claims, err)
	}
	if refresh == "" {
		t.Fatal("refresh token is empty")
	}
}
