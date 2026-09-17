package bootstrap

import (
	"github.com/cylism/cylism-manager/internal/repository"
	networkservice "github.com/cylism/cylism-manager/internal/service/network"
	alertingservice "github.com/cylism/cylism-manager/internal/service/observability/alerting"
	storageservice "github.com/cylism/cylism-manager/internal/service/storage"
	"github.com/cylism/cylism-manager/internal/store"
)

// Repositories is the repository-side composition graph. Keeping the
// concrete Store behind these narrow ports prevents service wiring from
// becoming coupled to the persistence implementation.
type Repositories struct {
	Store            *store.Store
	Users            repository.UserRepository
	TemporaryToken   repository.TemporaryLoginTokenRepository
	Cluster          repository.ServerRepository
	Network          networkservice.DomainStore
	DNSCredentials   networkservice.DNSCredentialStore
	StorageEnv       storageservice.EnvironmentStore
	StorageRecords   storageservice.RecordStore
	Application      repository.ApplicationQueryRepository
	Platform         repository.PlatformReleaseRepository
	Mirror           repository.NodeRegistryMirrorRepository
	Managed          repository.ManagedRegistryRepository
	RegistryCatalog  repository.ManagedRegistryCatalogRepository
	Proxy            repository.RegistryProxyRepository
	Alerting         alertingservice.AutomationRepository
	LoggingScope     repository.LoggingScopeRepository
	SystemComponents repository.SystemComponentRepository
}

func BuildRepositories(db *store.Store) Repositories {
	return Repositories{
		Store: db, Users: db, TemporaryToken: db, Cluster: db, Network: db,
		DNSCredentials: db, StorageEnv: db, StorageRecords: db, Application: db,
		Platform: db, Mirror: db, Managed: db, RegistryCatalog: db, Proxy: db, Alerting: db,
		LoggingScope: db, SystemComponents: db,
	}
}

// NewRepositories opens the platform persistence implementation. Repository
// interfaces are consumed by services; the bootstrap layer only owns the
// concrete Store lifetime.
func NewRepositories(path string) (*store.Store, error) { return store.New(path) }
