package repository

import (
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
)

// ManagedRegistryCatalogRepository contains only the persistent application
// definitions required to protect managed Registry content from deletion.
type ManagedRegistryCatalogRepository interface {
	ListManagedRegistryContentReferences() ([]model.ManagedRegistryContentReference, error)
}

var _ ManagedRegistryCatalogRepository = (*store.Store)(nil)
