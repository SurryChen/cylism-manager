package registry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

type proxyReconcilerAdapterFake struct {
	available bool
	ready     bool
	missing   bool
	err       error
	ctx       context.Context
	cleared   int
}

func (f *proxyReconcilerAdapterFake) Available() bool { return f.available }

func (f *proxyReconcilerAdapterFake) DeploymentAvailable(ctx context.Context, _ *model.RegistryProxy) (bool, bool, error) {
	f.ctx = ctx
	return f.ready, f.missing, f.err
}

func (f *proxyReconcilerAdapterFake) ClearCache(ctx context.Context, _ *model.RegistryProxy) error {
	f.ctx = ctx
	f.cleared++
	return nil
}

func TestProxyReconcilerRefreshesReadyProxyWithRequestContext(t *testing.T) {
	past := time.Now().Add(-2 * time.Hour)
	repo := &proxyRepositoryFake{proxies: []model.RegistryProxy{{ID: 7, Name: "Docker Hub", CleanupIntervalHours: 1, Status: "ready", LastCleanupAt: &past}}}
	adapter := &proxyReconcilerAdapterFake{available: true, ready: true}
	reconciler := NewProxyReconciler(NewProxyService(repo, nil), adapter, adapter)

	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "background-task")
	if err := reconciler.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if adapter.ctx == nil || adapter.ctx.Value(contextKey{}) != "background-task" {
		t.Fatal("reconciler did not propagate its lifecycle context")
	}
	if adapter.cleared != 1 {
		t.Fatalf("cache clear calls = %d, want 1", adapter.cleared)
	}
	if repo.saved == nil || repo.saved.Status != "deploying" || repo.saved.LastCheckedAt == nil || repo.saved.LastCleanupAt == nil {
		t.Fatalf("unexpected reconciled proxy: %#v", repo.saved)
	}
}

func TestProxyReconcilerPersistsMissingAndDiagnosticFailure(t *testing.T) {
	for name, adapter := range map[string]*proxyReconcilerAdapterFake{
		"missing": {available: true, missing: true},
		"failure": {available: true, err: errors.New("api unavailable")},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &proxyRepositoryFake{proxies: []model.RegistryProxy{{ID: 7, Name: "Docker Hub", CleanupIntervalHours: 24}}}
			reconciler := NewProxyReconciler(NewProxyService(repo, nil), adapter, adapter)
			if err := reconciler.Reconcile(context.Background()); err != nil {
				t.Fatalf("Reconcile: %v", err)
			}
			if repo.saved == nil || repo.saved.LastCheckedAt == nil {
				t.Fatalf("proxy was not persisted: %#v", repo.saved)
			}
			if name == "missing" && repo.saved.Status != "missing" {
				t.Fatalf("status = %q, want missing", repo.saved.Status)
			}
			if name == "failure" && repo.saved.Status != "failed" {
				t.Fatalf("status = %q, want failed", repo.saved.Status)
			}
		})
	}
}
