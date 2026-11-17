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

func TestPlatformReleasesPagination(t *testing.T) {
	s := setupTestDB(t)
	for i := 0; i < 25; i++ {
		if err := s.CreatePlatformRelease(&model.PlatformRelease{Source: "manual", Image: "registry.example.com/platform:test", Status: "succeeded"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		offset int
		count  int
		first  uint
	}{
		{0, 10, 25},
		{10, 10, 15},
		{20, 5, 5},
		{30, 0, 0},
	} {
		releases, total, err := s.ListPlatformReleasesPage(10, test.offset)
		if err != nil || total != 25 || len(releases) != test.count {
			t.Fatalf("offset %d: releases=%d total=%d err=%v", test.offset, len(releases), total, err)
		}
		if len(releases) > 0 && releases[0].ID != test.first {
			t.Fatalf("offset %d: first ID=%d, want %d", test.offset, releases[0].ID, test.first)
		}
	}
}
