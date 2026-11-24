package registry

import (
	"context"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

// ManagedRegistryStatusReader is the narrow live-cluster capability needed to
// keep persisted Registry status snapshots current outside browser requests.
type ManagedRegistryStatusReader interface {
	Available() bool
	ManagedRegistryStatus(context.Context, *model.ManagedOCIRegistry) (string, string, string)
}

// ManagedRegistryStatusReconciler refreshes Registry status in the background
// so list endpoints remain a fast database read.
type ManagedRegistryStatusReconciler struct {
	service *ManagedRegistryService
	status  ManagedRegistryStatusReader
	now     func() time.Time
}

func NewManagedRegistryStatusReconciler(service *ManagedRegistryService, status ManagedRegistryStatusReader) *ManagedRegistryStatusReconciler {
	return &ManagedRegistryStatusReconciler{service: service, status: status, now: time.Now}
}

func (r *ManagedRegistryStatusReconciler) Reconcile(ctx context.Context) error {
	if r == nil || r.service == nil || r.status == nil || !r.status.Available() {
		return nil
	}
	registries, err := r.service.List()
	if err != nil {
		return err
	}
	for index := range registries {
		if err := r.Refresh(ctx, &registries[index]); err != nil {
			return err
		}
	}
	return nil
}

func (r *ManagedRegistryStatusReconciler) Refresh(ctx context.Context, registry *model.ManagedOCIRegistry) error {
	if r == nil || registry == nil || r.service == nil || r.status == nil || !r.status.Available() {
		return nil
	}
	phase, status, detail := r.status.ManagedRegistryStatus(ctx, registry)
	now := r.nowTime()
	registry.PVCPhase, registry.Status, registry.LastError, registry.LastCheckedAt = phase, status, detail, &now
	return r.service.Save(registry)
}

func (r *ManagedRegistryStatusReconciler) nowTime() time.Time {
	if r.now == nil {
		return time.Now()
	}
	return r.now()
}
