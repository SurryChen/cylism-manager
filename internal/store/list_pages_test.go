package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestTemporaryTokenPageCountsOnlyCurrentUser(t *testing.T) {
	s := setupTestDB(t)
	for i := 0; i < 24; i++ {
		owner := uint(7)
		if i == 23 {
			owner = 8
		}
		item := model.TemporaryLoginToken{CreatedBy: owner, TokenHash: fmt.Sprintf("hash-%d", i), Label: "access", ExpiresAt: time.Now().Add(time.Hour)}
		if err := s.CreateTemporaryLoginToken(&item); err != nil {
			t.Fatal(err)
		}
	}
	items, total, err := s.ListTemporaryLoginTokensPage(7, 10, 20)
	if err != nil || total != 23 || len(items) != 3 || items[0].ID != 3 {
		t.Fatalf("token page: %v %d %v", items, total, err)
	}
	for _, item := range items {
		if item.CreatedBy != 7 {
			t.Fatal("cross-user token")
		}
	}
}

func TestCloudConnectionPageRetainsCredentialFlagWithoutExposingSecret(t *testing.T) {
	s := setupTestDB(t)
	created := time.Now()
	for i := 0; i < 23; i++ {
		item := model.CloudConnection{Name: fmt.Sprintf("cloud-%d", i), Provider: "aliyun", EncryptedCredentials: "encrypted-secret", CreatedBy: 7, CreatedAt: created}
		if err := s.CreateCloudConnection(&item); err != nil {
			t.Fatal(err)
		}
	}
	items, total, err := s.ListCloudConnectionsPage(10, 20)
	if err != nil || total != 23 || len(items) != 3 || items[0].ID != 3 || !items[0].CredentialConfigured {
		t.Fatalf("cloud page: %v %d %v", items, total, err)
	}
	body, err := json.Marshal(items)
	if err != nil || strings.Contains(string(body), "encrypted-secret") {
		t.Fatalf("credentials leaked: %s %v", body, err)
	}
}

func TestProductPageFiltersWithinOwnerScope(t *testing.T) {
	s := setupTestDB(t)
	for i := 0; i < 24; i++ {
		owner := uint(7)
		stage := "developing"
		if i == 23 {
			owner = 8
		}
		if i == 22 {
			stage = "archived"
		}
		p := model.Product{OwnerID: owner, Name: fmt.Sprintf("App %02d", i), Stage: stage, Summary: "offline 100%", Channels: []model.ProductChannel{{Platform: "web", PublishStatus: "draft"}}}
		if err := s.SaveProduct(&p, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	items, total, err := s.ListProductsPage(7, "developing", "current", "100%", 10, 20)
	if err != nil || total != 22 || len(items) != 2 || len(items[0].Channels) != 1 {
		t.Fatalf("page: %v %d %v", items, total, err)
	}
	items, total, err = s.ListProductsPage(7, "", "archived", "APP", 10, 0)
	if err != nil || total != 1 || len(items) != 1 || items[0].OwnerID != 7 {
		t.Fatalf("archive page: %v %d %v", items, total, err)
	}
	items, total, err = s.ListProductsPage(7, "", "current", "no match", 10, 0)
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("empty page: %v %d %v", items, total, err)
	}
}

func TestReleasePageScopeCountAndStableOrder(t *testing.T) {
	s := setupTestDB(t)
	created := time.Now()
	for i := 1; i <= 25; i++ {
		release := model.Release{ApplicationID: uint(i%2 + 1), Sequence: uint(i), Status: "succeeded", CreatedAt: created}
		if err := s.CreateRelease(&release); err != nil {
			t.Fatal(err)
		}
	}
	items, total, err := s.ListReleasesPage([]uint{1}, 10, 10)
	if err != nil || total != 12 || len(items) != 2 || items[0].ID != 4 {
		t.Fatalf("scoped page: %v %d %v", items, total, err)
	}
	for _, item := range items {
		if item.ApplicationID != 1 {
			t.Fatal("cross-application release")
		}
	}
	items, total, err = s.ListReleasesPage(nil, 10, 0)
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("empty scope: %v %d %v", items, total, err)
	}
}

func TestSpeedHistoryIsPersistedAndOwnerScoped(t *testing.T) {
	s := setupTestDB(t)
	for i := 0; i < 23; i++ {
		owner := uint(7)
		if i == 22 {
			owner = 8
		}
		item := model.ServerSpeedTest{OwnerID: owner, SourceID: 1, TargetID: 2, Protocol: "tcp", ReceiverMbps: float64(i)}
		if err := s.SaveServerSpeedTest(&item); err != nil {
			t.Fatal(err)
		}
	}
	items, total, err := s.ListServerSpeedTests(7, 10, 20)
	if err != nil || total != 22 || len(items) != 2 || items[0].ID != 2 || items[0].CreatedAt.IsZero() {
		t.Fatalf("history: %v %d %v", items, total, err)
	}
	items, total, err = s.ListServerSpeedTests(9, 10, 0)
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("owner scope: %v %d %v", items, total, err)
	}
}
