// Package repository provides persistence adapters for application services.
package repository

import (
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
)

// ManagedRegistryRepository is the persistence contract required by the
// managed Registry service. It keeps GORM and store implementation details
// outside of service code.
type ManagedRegistryRepository interface {
	ListManagedOCIRegistries() ([]model.ManagedOCIRegistry, error)
	GetManagedOCIRegistry(uint) (*model.ManagedOCIRegistry, error)
	GetManagedOCIRegistryByEndpoint(string) (*model.ManagedOCIRegistry, error)
	CreateManagedOCIRegistry(*model.ManagedOCIRegistry, *model.ImageRegistry, *model.NodeRegistryMirror, []uint) error
	UpdateManagedOCIRegistry(*model.ManagedOCIRegistry) error
	DeleteManagedOCIRegistry(uint) error
	CountManagedOCIRegistryReferences(uint) (int64, int64, error)
	GetImageRegistryByEndpoint(string) (*model.ImageRegistry, error)
	GetNodeRegistryMirrorByRegistry(string) (*model.NodeRegistryMirror, error)
	GetNodeRegistryMirror(uint) (*model.NodeRegistryMirror, error)
	ListNodeRegistryMirrors() ([]model.NodeRegistryMirror, error)
	ListServers() ([]model.Server, error)
	UpsertNodeRegistryMirrorStatus(*model.NodeRegistryMirrorNode) error
	UpdateImageRegistry(*model.ImageRegistry, []uint) error
	UpdateNodeRegistryMirror(*model.NodeRegistryMirror) error
	DeleteImageRegistry(uint) error
	DeleteNodeRegistryMirror(uint) error
}

// StoreManagedRegistryRepository adapts the existing Store without leaking it
// into Registry service APIs.
type StoreManagedRegistryRepository struct {
	store *store.Store
}

func NewManagedRegistryRepository(s *store.Store) *StoreManagedRegistryRepository {
	return &StoreManagedRegistryRepository{store: s}
}

func (r *StoreManagedRegistryRepository) ListManagedOCIRegistries() ([]model.ManagedOCIRegistry, error) {
	return r.store.ListManagedOCIRegistries()
}

func (r *StoreManagedRegistryRepository) GetManagedOCIRegistry(id uint) (*model.ManagedOCIRegistry, error) {
	return r.store.GetManagedOCIRegistry(id)
}

func (r *StoreManagedRegistryRepository) GetManagedOCIRegistryByEndpoint(endpoint string) (*model.ManagedOCIRegistry, error) {
	return r.store.GetManagedOCIRegistryByEndpoint(endpoint)
}

func (r *StoreManagedRegistryRepository) CreateManagedOCIRegistry(registry *model.ManagedOCIRegistry, image *model.ImageRegistry, mirror *model.NodeRegistryMirror, projectIDs []uint) error {
	return r.store.CreateManagedOCIRegistry(registry, image, mirror, projectIDs)
}

func (r *StoreManagedRegistryRepository) UpdateManagedOCIRegistry(registry *model.ManagedOCIRegistry) error {
	return r.store.UpdateManagedOCIRegistry(registry)
}

func (r *StoreManagedRegistryRepository) DeleteManagedOCIRegistry(id uint) error {
	return r.store.DeleteManagedOCIRegistry(id)
}

func (r *StoreManagedRegistryRepository) CountManagedOCIRegistryReferences(id uint) (int64, int64, error) {
	return r.store.CountManagedOCIRegistryReferences(id)
}

func (r *StoreManagedRegistryRepository) GetImageRegistryByEndpoint(endpoint string) (*model.ImageRegistry, error) {
	return r.store.GetImageRegistryByEndpoint(endpoint)
}

func (r *StoreManagedRegistryRepository) GetNodeRegistryMirrorByRegistry(registry string) (*model.NodeRegistryMirror, error) {
	return r.store.GetNodeRegistryMirrorByRegistry(registry)
}

func (r *StoreManagedRegistryRepository) GetNodeRegistryMirror(id uint) (*model.NodeRegistryMirror, error) {
	return r.store.GetNodeRegistryMirror(id)
}

func (r *StoreManagedRegistryRepository) ListNodeRegistryMirrors() ([]model.NodeRegistryMirror, error) {
	return r.store.ListNodeRegistryMirrors()
}

func (r *StoreManagedRegistryRepository) ListServers() ([]model.Server, error) {
	return r.store.ListServers()
}

func (r *StoreManagedRegistryRepository) UpsertNodeRegistryMirrorStatus(status *model.NodeRegistryMirrorNode) error {
	return r.store.UpsertNodeRegistryMirrorStatus(status)
}

func (r *StoreManagedRegistryRepository) UpdateImageRegistry(image *model.ImageRegistry, projectIDs []uint) error {
	return r.store.UpdateImageRegistry(image, projectIDs)
}

func (r *StoreManagedRegistryRepository) UpdateNodeRegistryMirror(mirror *model.NodeRegistryMirror) error {
	return r.store.UpdateNodeRegistryMirror(mirror)
}

func (r *StoreManagedRegistryRepository) DeleteImageRegistry(id uint) error {
	return r.store.DeleteImageRegistry(id)
}

func (r *StoreManagedRegistryRepository) DeleteNodeRegistryMirror(id uint) error {
	return r.store.DeleteNodeRegistryMirror(id)
}
