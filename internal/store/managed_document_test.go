package store

import (
	"encoding/json"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestManagedDocumentNormalizesPathsAndUsesOptimisticVersion(t *testing.T) {
	s := setupTestDB(t)
	paths, _ := json.Marshal([]string{"/auth/userpass", "/auth/userpass"})
	document := &model.ManagedDocument{ApplicationID: 7, ResourceKind: "secret", ResourceName: "hysteria-config", Key: "config.yaml", Format: "yaml", AllowedPaths: string(paths), CreatedBy: 1}
	if err := s.CreateManagedDocument(document); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.GetManagedDocument(7, document.ID)
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := loaded.Paths()
	if err != nil || len(allowed) != 1 || allowed[0] != "/auth/userpass" || loaded.Version != 1 {
		t.Fatalf("unexpected document: %#v paths=%#v err=%v", loaded, allowed, err)
	}
	if _, err := s.AdvanceManagedDocumentVersion(7, document.ID, 2); err == nil {
		t.Fatal("expected stale document version to be rejected")
	}
	version, err := s.AdvanceManagedDocumentVersion(7, document.ID, 1)
	if err != nil || version != 2 {
		t.Fatalf("advance version=%d err=%v", version, err)
	}
}
