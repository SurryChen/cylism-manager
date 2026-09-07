package store

import (
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestAuthRepositoryUserCRUD(t *testing.T) {
	s := setupTestDB(t)
	user := &model.User{Username: "owner", PasswordHash: "hash"}
	if err := s.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if user.ID == 0 {
		t.Fatal("expected generated user id")
	}
	byName, err := s.GetUserByUsername(user.Username)
	if err != nil || byName.ID != user.ID {
		t.Fatalf("get by username = %#v, %v", byName, err)
	}
	byID, err := s.GetUserByID(user.ID)
	if err != nil || byID.Username != user.Username {
		t.Fatalf("get by id = %#v, %v", byID, err)
	}
	count, err := s.CountUsers()
	if err != nil || count != 1 {
		t.Fatalf("count = %d, %v", count, err)
	}
}

func TestAuthRepositoryTemporaryLoginTokenLifecycle(t *testing.T) {
	s := setupTestDB(t)
	user := &model.User{Username: "owner", PasswordHash: "hash"}
	if err := s.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	token := &model.TemporaryLoginToken{TokenHash: "active-token", CreatedBy: user.ID, Label: "review", ExpiresAt: now.Add(time.Hour)}
	if err := s.CreateTemporaryLoginToken(token); err != nil {
		t.Fatal(err)
	}
	if got, err := s.FindActiveTemporaryLoginToken(token.TokenHash, now); err != nil || got.ID != token.ID {
		t.Fatalf("active token = %#v, %v", got, err)
	}
	usedAt := now.Add(time.Minute)
	if err := s.MarkTemporaryLoginTokenUsed(token.TokenHash, usedAt); err != nil {
		t.Fatal(err)
	}
	items, err := s.ListTemporaryLoginTokens(user.ID)
	if err != nil || len(items) != 1 || items[0].LastUsedAt == nil || !items[0].LastUsedAt.Equal(usedAt) {
		t.Fatalf("tokens = %#v, %v", items, err)
	}
	if err := s.RevokeTemporaryLoginToken(user.ID, token.ID, usedAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindActiveTemporaryLoginToken(token.TokenHash, now); err == nil {
		t.Fatal("revoked token should not be active")
	}
	expired := &model.TemporaryLoginToken{TokenHash: "expired-token", CreatedBy: user.ID, ExpiresAt: now.Add(-time.Minute)}
	if err := s.CreateTemporaryLoginToken(expired); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindActiveTemporaryLoginToken(expired.TokenHash, now); err == nil {
		t.Fatal("expired token should not be active")
	}
}
