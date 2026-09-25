package registry

import (
	"context"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

type managedRegistryStatusRepositoryFake struct {
	managedRegistryRepositoryFake
	saved *model.ManagedOCIRegistry
}

func (f *managedRegistryStatusRepositoryFake) UpdateManagedOCIRegistry(registry *model.ManagedOCIRegistry) error {
	copy := *registry
	f.saved = &copy
	return nil
}

type managedRegistryStatusReaderFake struct {
	ctx context.Context
}

func (f *managedRegistryStatusReaderFake) Available() bool { return true }

func (f *managedRegistryStatusReaderFake) ManagedRegistryStatus(ctx context.Context, _ *model.ManagedOCIRegistry) (string, string, string) {
	f.ctx = ctx
	return "Bound", "ready", ""
}

func TestManagedRegistryStatusReconcilerPersistsLiveStatusOutsideListRequests(t *testing.T) {
	repo := &managedRegistryStatusRepositoryFake{managedRegistryRepositoryFake: managedRegistryRepositoryFake{registries: []model.ManagedOCIRegistry{{ID: 7, Name: "platform"}}}}
	reader := &managedRegistryStatusReaderFake{}
	reconciler := NewManagedRegistryStatusReconciler(NewManagedRegistryService(repo, nil), reader)

	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "background-task")
	if err := reconciler.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if reader.ctx == nil || reader.ctx.Value(contextKey{}) != "background-task" {
		t.Fatal("reconciler did not propagate its lifecycle context")
	}
	if repo.saved == nil || repo.saved.Status != "ready" || repo.saved.PVCPhase != "Bound" || repo.saved.LastCheckedAt == nil {
		t.Fatalf("unexpected persisted Registry snapshot: %#v", repo.saved)
	}
}
