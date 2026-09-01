package platform

import (
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/store"
)

type platformAdapterFake struct {
	status  *k8sclient.PlatformDeploymentStatus
	updated string
}

func (f *platformAdapterFake) PlatformDeploymentStatus() (*k8sclient.PlatformDeploymentStatus, error) {
	return f.status, nil
}
func (f *platformAdapterFake) UpdatePlatformDeployment(image string, _ uint) (string, error) {
	f.updated = image
	return "old-image", nil
}

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

func TestReleaseServiceUsesMinimalPlatformAdapter(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	adapter := &platformAdapterFake{status: &k8sclient.PlatformDeploymentStatus{Image: "registry.example.com/cylism-manager:0.9", DesiredReplicas: 1, ReadyReplicas: 1}}
	service := NewReleaseService(st, nil, adapter)
	if !service.Available() {
		t.Fatal("expected release service to be available with the minimal platform adapter")
	}
	release, err := service.CreateRelease("registry.example.com/cylism-manager:1.0.0", "manual", "abc", "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if release.PreviousImage != "registry.example.com/cylism-manager:0.9" {
		t.Fatalf("expected previous image from adapter, got %q", release.PreviousImage)
	}
	service.Apply(release.ID)
	if adapter.updated != release.Image {
		t.Fatalf("expected adapter update for %q, got %q", release.Image, adapter.updated)
	}
}
