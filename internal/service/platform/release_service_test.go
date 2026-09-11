package platform

import (
	"context"
	"testing"
	"time"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
)

type platformAdapterFake struct {
	status    *k8sclient.PlatformDeploymentStatus
	statusCtx context.Context
	updateCtx context.Context
	updated   string
}

func (f *platformAdapterFake) PlatformDeploymentStatusContext(ctx context.Context) (*k8sclient.PlatformDeploymentStatus, error) {
	f.statusCtx = ctx
	return f.status, nil
}
func (f *platformAdapterFake) UpdatePlatformDeploymentContext(ctx context.Context, image string, _ uint) (string, error) {
	f.updateCtx = ctx
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

func TestReleaseServiceAcceptsDefaultGHCRDevImage(t *testing.T) {
	service := NewReleaseService(nil, nil, nil)
	if err := service.ValidateImage(DefaultImagePrefix + ":dev-abcdef123456"); err != nil {
		t.Fatalf("expected default GHCR dev image to be accepted: %v", err)
	}
	if err := service.ValidateImage("crpi-c5u9bb8i5qxw1m72.cn-guangzhou.personal.cr.aliyuncs.com/surrychen/cylism-manager:dev-abcdef123456"); err == nil {
		t.Fatal("expected old ACR platform image prefix to be rejected by default")
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
	release, err := service.CreateRelease(context.Background(), "registry.example.com/cylism-manager:1.0.0", "manual", "abc", "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if release.PreviousImage != "registry.example.com/cylism-manager:0.9" {
		t.Fatalf("expected previous image from adapter, got %q", release.PreviousImage)
	}
	service.Apply(context.Background(), release.ID)
	if adapter.updated != release.Image {
		t.Fatalf("expected adapter update for %q, got %q", release.Image, adapter.updated)
	}
}

func TestReleaseServiceReconcileLatestPropagatesLifecycleContext(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	adapter := &platformAdapterFake{status: &k8sclient.PlatformDeploymentStatus{
		Image: "registry.example.com/cylism-manager:0.9", DesiredReplicas: 1,
	}}
	service := NewReleaseService(st, nil, adapter)
	if err := st.CreatePlatformRelease(&model.PlatformRelease{
		Source: "manual", Image: "registry.example.com/cylism-manager:1.0.0", Status: "accepted",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "background-lifecycle")
	service.ReconcileLatest(ctx)
	if adapter.statusCtx == nil || adapter.statusCtx.Value(contextKey{}) != "background-lifecycle" {
		t.Fatal("reconcile did not propagate its lifecycle context to the status adapter")
	}
	if adapter.updateCtx == nil || adapter.updateCtx.Value(contextKey{}) != "background-lifecycle" {
		t.Fatal("reconcile did not propagate its lifecycle context to the update adapter")
	}
}
