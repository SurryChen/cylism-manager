package store

import (
	"path/filepath"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type legacyDNSCredential struct {
	ID         uint   `gorm:"primaryKey"`
	Name       string `gorm:"size:128;uniqueIndex;not null"`
	SecretName string `gorm:"size:253;uniqueIndex;not null"`
}

func (legacyDNSCredential) TableName() string { return "dns_credentials" }

func TestDNSCredentialIndexMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.AutoMigrate(&legacyDNSCredential{}); err != nil {
		t.Fatal(err)
	}
	st, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ClusterDNSCredentials(1).CreateDNSCredential(&model.DNSCredential{Name: "same", Provider: "alidns", Namespace: "cert-manager", SecretName: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := st.ClusterDNSCredentials(2).CreateDNSCredential(&model.DNSCredential{Name: "same", Provider: "alidns", Namespace: "cert-manager", SecretName: "second"}); err != nil {
		t.Fatalf("legacy unique index still applies: %v", err)
	}
}

func TestClusterDNSCredentialsAreIsolated(t *testing.T) {
	st := setupTestDB(t)
	first := st.ClusterDNSCredentials(1)
	second := st.ClusterDNSCredentials(2)
	credential := &model.DNSCredential{Name: "first-cluster", Provider: "alidns", Namespace: "cert-manager", SecretName: "cylism-dns-test", Enabled: true}
	if err := first.CreateDNSCredential(credential); err != nil {
		t.Fatal(err)
	}
	if credential.ClusterID != 1 {
		t.Fatalf("cluster ID = %d, want 1", credential.ClusterID)
	}
	if err := second.CreateDNSCredential(&model.DNSCredential{Name: "first-cluster", Provider: "alidns", Namespace: "cert-manager", SecretName: "cylism-dns-other", Enabled: true}); err != nil {
		t.Fatalf("same credential name in another cluster: %v", err)
	}
	items, err := second.ListDNSCredentials()
	if err != nil || len(items) != 1 || items[0].ClusterID != 2 {
		t.Fatalf("other cluster credentials = %v, err = %v", items, err)
	}
	if _, err := second.GetDNSCredential(credential.ID); err == nil {
		t.Fatal("other cluster read credential")
	}
	credential.Name = "changed"
	credential.ClusterID = 2
	if err := second.UpdateDNSCredential(credential); err == nil {
		t.Fatal("other cluster updated credential")
	}
	if err := second.DeleteDNSCredential(credential.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := first.GetDNSCredential(credential.ID)
	if err != nil || stored.Name != "first-cluster" {
		t.Fatalf("original credential = %v, err = %v", stored, err)
	}
}
