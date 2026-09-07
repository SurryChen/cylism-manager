package store

import (
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestPlatformEndpointIsPersistedAsSingleton(t *testing.T) {
	s := setupTestDB(t)
	endpoint := &model.PlatformEndpoint{Hostname: "console.example.com", IngressName: "cylism-ingress", CertificateName: "console-example-com", TLSSecretName: "console-example-com-tls", Enabled: true}
	if err := s.SavePlatformEndpoint(endpoint); err != nil {
		t.Fatal(err)
	}
	endpoint.Hostname = "admin.example.com"
	if err := s.SavePlatformEndpoint(endpoint); err != nil {
		t.Fatal(err)
	}
	stored, err := s.GetPlatformEndpoint()
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != 1 || stored.Hostname != "admin.example.com" || stored.IngressName != "cylism-ingress" || !stored.Enabled {
		t.Fatalf("unexpected platform endpoint: %#v", stored)
	}
}
