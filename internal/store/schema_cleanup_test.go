package store

import (
	"path/filepath"
	"testing"
)

func TestSchemaCleanupRemovesLegacyDNSColumnsAndManagedDocuments(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "store.db")
	s, err := New(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DB().Exec("ALTER TABLE dns_credentials ADD COLUMN access_key_id text").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DB().Exec("ALTER TABLE dns_credentials ADD COLUMN access_key_secret text").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DB().Exec("CREATE TABLE managed_documents (id integer primary key)").Error; err != nil {
		t.Fatal(err)
	}
	migrated, err := New(dsn)
	if err != nil {
		t.Fatalf("cleanup legacy schema: %v", err)
	}
	if migrated.DB().Migrator().HasColumn("dns_credentials", "access_key_id") || migrated.DB().Migrator().HasColumn("dns_credentials", "access_key_secret") {
		t.Fatal("legacy DNS credential columns must be removed")
	}
	if migrated.DB().Migrator().HasTable("managed_documents") {
		t.Fatal("managed_documents table must be removed")
	}
}

func TestSchemaCleanupIsIdempotent(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "store.db")
	if _, err := New(dsn); err != nil {
		t.Fatal(err)
	}
	if _, err := New(dsn); err != nil {
		t.Fatalf("re-running schema cleanup should succeed: %v", err)
	}
}
