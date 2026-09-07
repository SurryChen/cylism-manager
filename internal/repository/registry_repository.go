// Package repository provides persistence adapters for application services.
package repository

import (
	"time"

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

// NodeRegistryMirrorRepository is the persistence contract used by the node
// mirror workflow. Keeping it separate from ManagedRegistryRepository avoids
// coupling standalone mirror operations to the managed OCI Registry use case.
type NodeRegistryMirrorRepository interface {
	CreateNodeRegistryMirror(*model.NodeRegistryMirror) error
	GetNodeRegistryMirror(uint) (*model.NodeRegistryMirror, error)
	ListNodeRegistryMirrors() ([]model.NodeRegistryMirror, error)
	UpdateNodeRegistryMirror(*model.NodeRegistryMirror) error
	UpdateNodeRegistryMirrorVerification(uint, string, string, time.Time) error
	DeleteNodeRegistryMirror(uint) error
	UpsertNodeRegistryMirrorStatus(*model.NodeRegistryMirrorNode) error
	ListServers() ([]model.Server, error)
}

// RegistryProxyRepository is the persistence contract for proxy configuration.
type RegistryProxyRepository interface {
	GetRegistryProxy() (*model.RegistryProxy, error)
	GetRegistryProxyByID(uint) (*model.RegistryProxy, error)
	ListRegistryProxies() ([]model.RegistryProxy, error)
	SaveRegistryProxy(*model.RegistryProxy) error
}

// ImageRegistryRepository owns externally configured image Registry records.
type ImageRegistryRepository interface {
	CreateImageRegistry(*model.ImageRegistry, []uint) error
	ListImageRegistries(uint) ([]model.ImageRegistry, error)
	GetImageRegistry(uint) (*model.ImageRegistry, error)
	GetImageRegistryByEndpoint(string) (*model.ImageRegistry, error)
	GetImageRegistryForProject(uint, uint) (*model.ImageRegistry, error)
	UpdateImageRegistry(*model.ImageRegistry, []uint) error
	UpdateImageRegistryVerification(uint, string, string, time.Time) error
	CountImageRegistryReleases(uint) (int64, error)
	DeleteImageRegistry(uint) error
}

// ChartRepositoryRepository persists Helm chart repository configuration.
type ChartRepositoryRepository interface {
	CreateChartRepository(*model.ChartRepository) error
	ListChartRepositories() ([]model.ChartRepository, error)
	GetChartRepository(uint) (*model.ChartRepository, error)
	UpdateChartRepository(*model.ChartRepository) error
	DeleteChartRepository(uint) error
	GetVerifiedCertManagerChartRepository() (*model.ChartRepository, error)
}

var (
	_ ManagedRegistryRepository    = (*store.Store)(nil)
	_ NodeRegistryMirrorRepository = (*store.Store)(nil)
	_ RegistryProxyRepository      = (*store.Store)(nil)
	_ ImageRegistryRepository      = (*store.Store)(nil)
	_ ChartRepositoryRepository    = (*store.Store)(nil)
)
