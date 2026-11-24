package store

import (
	"encoding/json"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestCloudConnectionPersistsEncryptedCredentialWithoutJSONExposure(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	connection := &model.CloudConnection{Name: "production", Provider: "example", EncryptedCredentials: "ciphertext", Configuration: "test-1", DNSEnabled: true, CreatedBy: 7}
	if err := s.CreateCloudConnection(connection); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.GetCloudConnection(connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.CredentialConfigured || loaded.EncryptedCredentials != "ciphertext" {
		t.Fatalf("credential persistence mismatch: %#v", loaded)
	}
	body, err := json.Marshal(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) == "" || contains(string(body), "ciphertext") || contains(string(body), "credentials") {
		t.Fatalf("connection JSON leaked credential: %s", body)
	}
}

func TestCloudConnectionListSetsCredentialConfigured(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateCloudConnection(&model.CloudConnection{Name: "configured", Provider: "example", EncryptedCredentials: "secret", Configuration: "{}", ObjectStorageEnabled: true}); err != nil {
		t.Fatal(err)
	}
	items, err := s.ListCloudConnections()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].CredentialConfigured {
		t.Fatalf("unexpected connection list: %#v", items)
	}
}

func contains(value, fragment string) bool {
	return len(fragment) > 0 && len(value) >= len(fragment) && (value == fragment || index(value, fragment) >= 0)
}

func index(value, fragment string) int {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return i
		}
	}
	return -1
}
