package repository

import (
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
)

// ServerRepository contains only the server records managed alongside cluster nodes.
type ServerRepository interface {
	CreateServer(*model.Server) error
	GetServer(uint) (*model.Server, error)
	ListServers() ([]model.Server, error)
	UpdateServer(*model.Server) error
	DeleteServer(uint) error
	UnbindServersFromClusterNode(string) error
}

// NodeJoinRepository is the durable input required by the worker join
// progress stream: server credentials plus the two installation settings.
type NodeJoinRepository interface {
	ServerRepository
	GetSystemConfig(string) (string, error)
}

// SystemConfigRepository is the durable boundary for platform-wide settings.
type SystemConfigRepository interface {
	GetSystemConfig(string) (string, error)
	SetSystemConfig(string, string) error
}

// ClusterDNSRepository persists the active CoreDNS forwarding policy and its
// revision history.
type ClusterDNSRepository interface {
	GetActiveClusterDNSPolicy() (*model.ClusterDNSPolicy, error)
	ListClusterDNSPolicies(int) ([]model.ClusterDNSPolicy, error)
	CreateClusterDNSPolicy(*model.ClusterDNSPolicy) error
}

// SiteRepository owns legacy server-backed site and certificate records.
type SiteRepository interface {
	CreateSite(*model.Site) error
	GetSite(uint) (*model.Site, error)
	ListSites() ([]model.Site, error)
	ListSitesByServer(uint) ([]model.Site, error)
	UpdateSite(*model.Site) error
	DeleteSite(uint) error
	CreateCert(*model.Cert) error
	GetCert(uint) (*model.Cert, error)
	GetCertBySite(uint) (*model.Cert, error)
	UpdateCert(*model.Cert) error
	DeleteCert(uint) error
}

// NetworkRepository is the durable boundary for environment-scoped domains
// and DNS credentials.
type NetworkRepository interface {
	GetEnvironmentByID(uint) (*model.Environment, error)
	GetManagedDomain(uint) (*model.ManagedDomain, error)
	GetManagedDomainByHostname(string) (*model.ManagedDomain, error)
	ListManagedDomains(...uint) ([]model.ManagedDomain, error)
	ListUnassignedManagedDomains() ([]model.ManagedDomain, error)
	ListClaimableManagedDomains(string) ([]model.ManagedDomain, error)
	CreateManagedDomain(*model.ManagedDomain) error
	UpdateManagedDomain(*model.ManagedDomain) error
	DeleteManagedDomain(uint) error
	CountApplicationEndpointsByDomain(uint) (int64, error)
	ListDNSCredentials() ([]model.DNSCredential, error)
	GetDNSCredential(uint) (*model.DNSCredential, error)
	CreateDNSCredential(*model.DNSCredential) error
	UpdateDNSCredential(*model.DNSCredential) error
	DeleteDNSCredential(uint) error
}

// CertificateRepository combines DNS/domain records with the verified chart
// source required by the cert-manager installation flow.
type CertificateRepository interface {
	NetworkRepository
	GetVerifiedCertManagerChartRepository() (*model.ChartRepository, error)
}

// StorageRepository owns the durable task records for PVC migration, backup,
// and host-directory import workflows.
type StorageRepository interface {
	GetEnvironmentByID(uint) (*model.Environment, error)
	CreatePersistentVolumeMigration(*model.PersistentVolumeMigration) error
	GetPersistentVolumeMigration(uint) (*model.PersistentVolumeMigration, error)
	ListPersistentVolumeMigrations(uint) ([]model.PersistentVolumeMigration, error)
	FindActivePVCMigration(uint, string) (*model.PersistentVolumeMigration, error)
	UpdatePersistentVolumeMigration(*model.PersistentVolumeMigration, string, string) error
	CreatePersistentVolumeBackup(*model.PersistentVolumeBackup) error
	GetPersistentVolumeBackup(uint) (*model.PersistentVolumeBackup, error)
	ListPersistentVolumeBackups(uint, string) ([]model.PersistentVolumeBackup, error)
	UpdatePersistentVolumeBackup(*model.PersistentVolumeBackup) error
	CreateHostDirectoryPVCImport(*model.HostDirectoryPVCImport) error
	GetHostDirectoryPVCImport(uint) (*model.HostDirectoryPVCImport, error)
	ListHostDirectoryPVCImports(uint, string) ([]model.HostDirectoryPVCImport, error)
	FindActiveHostDirectoryPVCImport(uint, string) (*model.HostDirectoryPVCImport, error)
	UpdateHostDirectoryPVCImport(*model.HostDirectoryPVCImport, string, string) error
}

// PVCRepository is the complete durable boundary used by PVC HTTP workflows:
// task records plus the small set of server/application lookups needed to
// validate and annotate a request.
type PVCRepository interface {
	StorageRepository
	ServerRepository
	GetProject(uint) (*model.Project, error)
	ListApplications(...uint) ([]model.Application, error)
	ListApplicationDeploymentTemplates(uint) ([]model.ApplicationDeploymentTemplate, error)
	UpdateApplicationDeploymentTemplate(*model.ApplicationDeploymentTemplate) error
}

// ResourceReferenceRepository protects Kubernetes resources from deletion
// while application templates or release snapshots still reference them.
type ResourceReferenceRepository interface {
	ListResourceReferences(string, string, string) ([]model.ResourceReference, error)
}

// PlatformReleaseRepository persists self-update configuration, replay
// protection, and release state transitions.
type PlatformReleaseRepository interface {
	GetSystemConfig(string) (string, error)
	SetSystemConfig(string, string) error
	CreatePlatformWebhookNonce(string, time.Time) error
	DeleteExpiredPlatformWebhookNonces(time.Time) error
	CreatePlatformRelease(*model.PlatformRelease) error
	GetPlatformRelease(uint) (*model.PlatformRelease, error)
	LatestIncompletePlatformRelease() (*model.PlatformRelease, error)
	UpdatePlatformRelease(*model.PlatformRelease) error
}

// PlatformEndpointRepository extends self-update records with the single
// platform ingress endpoint and managed-domain ownership check.
type PlatformEndpointRepository interface {
	PlatformReleaseRepository
	ListPlatformReleases(int) ([]model.PlatformRelease, error)
	GetPlatformEndpoint() (*model.PlatformEndpoint, error)
	SavePlatformEndpoint(*model.PlatformEndpoint) error
	GetManagedDomainByHostname(string) (*model.ManagedDomain, error)
}

var (
	_ ServerRepository            = (*store.Store)(nil)
	_ NodeJoinRepository          = (*store.Store)(nil)
	_ SystemConfigRepository      = (*store.Store)(nil)
	_ ClusterDNSRepository        = (*store.Store)(nil)
	_ SiteRepository              = (*store.Store)(nil)
	_ NetworkRepository           = (*store.Store)(nil)
	_ CertificateRepository       = (*store.Store)(nil)
	_ StorageRepository           = (*store.Store)(nil)
	_ PVCRepository               = (*store.Store)(nil)
	_ ResourceReferenceRepository = (*store.Store)(nil)
	_ PlatformReleaseRepository   = (*store.Store)(nil)
	_ PlatformEndpointRepository  = (*store.Store)(nil)
)
