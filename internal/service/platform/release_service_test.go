package platform

import (
	"testing"

	"github.com/cylism/cylism-manager/internal/store"
)

func TestReleaseServiceValidatesMultipleImagePrefixes(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	service := NewReleaseService(st, nil, nil)
	if _, err := service.SetImagePrefixes("registry.example.com/cylism-manager\noci.example.com/cylism-manager/"); err != nil {
		t.Fatal(err)
	}
	for _, image := range []string{"registry.example.com/cylism-manager:1.0.0", "oci.example.com/cylism-manager:2.0.0"} {
		if err := service.ValidateImage(image); err != nil {
			t.Fatalf("expected image %q to be accepted: %v", image, err)
		}
	}
	if err := service.ValidateImage("other.example.com/cylism-manager:1.0.0"); err == nil {
		t.Fatal("expected image outside configured prefixes to be rejected")
	}
}
