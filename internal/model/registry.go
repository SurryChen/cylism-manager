package model

import (
	"time"
)

type ImageRegistry struct {
	ID                   uint       `gorm:"primaryKey" json:"id"`
	Name                 string     `gorm:"size:128;uniqueIndex;not null" json:"name"`
	Endpoint             string     `gorm:"size:256;uniqueIndex;not null" json:"endpoint"`
	VerificationImage    string     `gorm:"size:512" json:"verification_image"`
	AuthType             string     `gorm:"size:32;not null" json:"auth_type"`
	Username             string     `gorm:"size:256" json:"username"`
	Credential           string     `gorm:"type:text" json:"-"`
	Enabled              bool       `gorm:"default:true;not null" json:"enabled"`
	LastVerifiedAt       *time.Time `json:"last_verified_at,omitempty"`
	LastVerifyStatus     string     `gorm:"size:32" json:"last_verify_status"`
	LastVerifyError      string     `gorm:"size:512" json:"last_verify_error,omitempty"`
	CreatedBy            uint       `gorm:"index" json:"created_by"`
	ManagedRegistryID    *uint      `gorm:"uniqueIndex" json:"managed_registry_id,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	Projects             []Project  `gorm:"many2many:image_registry_projects;" json:"projects,omitempty"`
	CredentialConfigured bool       `gorm:"-" json:"credential_configured"`
}

// NodeRegistryMirror defines one registry entry written to K3s registries.yaml on cluster nodes.

type NodeRegistryMirror struct {
	ID                   uint                     `gorm:"primaryKey" json:"id"`
	Name                 string                   `gorm:"size:128;uniqueIndex;not null" json:"name"`
	Registry             string                   `gorm:"size:256;uniqueIndex;not null" json:"registry"`
	Endpoints            string                   `gorm:"type:text;not null" json:"endpoints"`
	VerificationImage    string                   `gorm:"size:512" json:"verification_image"`
	Username             string                   `gorm:"size:256" json:"username"`
	Credential           string                   `gorm:"type:text" json:"-"`
	InsecureSkipVerify   bool                     `gorm:"default:false" json:"insecure_skip_verify"`
	Enabled              bool                     `gorm:"default:true;not null" json:"enabled"`
	CreatedBy            uint                     `gorm:"index" json:"created_by"`
	ManagedRegistryID    *uint                    `gorm:"uniqueIndex" json:"managed_registry_id,omitempty"`
	LastVerifiedAt       *time.Time               `json:"last_verified_at,omitempty"`
	LastVerifyStatus     string                   `gorm:"size:32" json:"last_verify_status"`
	LastVerifyError      string                   `gorm:"size:512" json:"last_verify_error,omitempty"`
	LastAppliedAt        *time.Time               `json:"last_applied_at,omitempty"`
	LastApplyStatus      string                   `gorm:"size:32" json:"last_apply_status"`
	LastApplyError       string                   `gorm:"size:512" json:"last_apply_error,omitempty"`
	CreatedAt            time.Time                `json:"created_at"`
	UpdatedAt            time.Time                `json:"updated_at"`
	CredentialConfigured bool                     `gorm:"-" json:"credential_configured"`
	NodeStatuses         []NodeRegistryMirrorNode `gorm:"foreignKey:MirrorID" json:"node_statuses,omitempty"`
}

// NodeRegistryMirrorNode records the last deployment result for one cluster node.

type NodeRegistryMirrorNode struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	MirrorID  uint       `gorm:"uniqueIndex:idx_mirror_node;not null" json:"mirror_id"`
	ServerID  uint       `gorm:"uniqueIndex:idx_mirror_node;not null" json:"server_id"`
	Status    string     `gorm:"size:32;not null" json:"status"`
	Detail    string     `gorm:"size:512" json:"detail,omitempty"`
	AppliedAt *time.Time `json:"applied_at,omitempty"`
	Server    Server     `gorm:"foreignKey:ServerID" json:"server,omitempty"`
}

// ManagedOCIRegistry owns the platform configuration for one persistent
// Docker Distribution Registry. Credentials are encrypted and never exposed.

type ManagedOCIRegistry struct {
	ID                   uint       `gorm:"primaryKey" json:"id"`
	Name                 string     `gorm:"size:128;uniqueIndex;not null" json:"name"`
	Namespace            string     `gorm:"size:253;not null" json:"namespace"`
	ResourceName         string     `gorm:"size:253;uniqueIndex;not null" json:"resource_name"`
	Endpoint             string     `gorm:"size:253;uniqueIndex;not null" json:"endpoint"`
	VerificationImage    string     `gorm:"size:512" json:"verification_image"`
	RegistryImage        string     `gorm:"size:512;not null" json:"registry_image"`
	DataNode             string     `gorm:"size:253;not null" json:"data_node"`
	StorageClassName     string     `gorm:"size:253;not null;default:local-path" json:"storage_class_name"`
	PVCName              string     `gorm:"size:253;not null;default:cylism-oci-registry-data" json:"pvc_name"`
	StorageSize          string     `gorm:"size:64;not null;default:100Gi" json:"storage_size"`
	CPURequest           string     `gorm:"size:64;not null;default:100m" json:"cpu_request"`
	CPULimit             string     `gorm:"size:64;not null;default:500m" json:"cpu_limit"`
	MemoryRequest        string     `gorm:"size:64;not null;default:256Mi" json:"memory_request"`
	MemoryLimit          string     `gorm:"size:64;not null;default:1Gi" json:"memory_limit"`
	InsecureHTTP         bool       `gorm:"not null;default:false" json:"insecure_http"`
	CertificateName      string     `gorm:"size:253" json:"certificate_name,omitempty"`
	TLSSecretName        string     `gorm:"size:253" json:"tls_secret_name,omitempty"`
	PullUsername         string     `gorm:"size:256;not null" json:"pull_username"`
	EncryptedCredential  string     `gorm:"type:text" json:"-"`
	ImageRegistryID      *uint      `gorm:"uniqueIndex" json:"image_registry_id,omitempty"`
	NodeRegistryMirrorID *uint      `gorm:"uniqueIndex" json:"node_registry_mirror_id,omitempty"`
	Status               string     `gorm:"size:32;not null;default:pending" json:"status"`
	LastError            string     `gorm:"size:512" json:"last_error,omitempty"`
	LastCheckedAt        *time.Time `json:"last_checked_at,omitempty"`
	CreatedBy            uint       `gorm:"index;not null" json:"created_by"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	CredentialConfigured bool       `gorm:"-" json:"credential_configured"`
	PVCPhase             string     `gorm:"-" json:"pvc_phase,omitempty"`
}

// RegistryProxy defines one platform-managed, non-persistent registry proxy.

type RegistryProxy struct {
	ID                      uint       `gorm:"primaryKey" json:"id"`
	Name                    string     `gorm:"size:128" json:"name"`
	Registry                string     `gorm:"size:256" json:"registry"`
	UpstreamURL             string     `gorm:"size:512" json:"upstream_url"`
	ResourceName            string     `gorm:"size:128" json:"resource_name"`
	NodeName                string     `gorm:"size:256;not null" json:"node_name"`
	EndpointHost            string     `gorm:"size:256;not null" json:"endpoint_host"`
	NodePort                int32      `gorm:"not null" json:"node_port"`
	CacheLimitGi            int32      `gorm:"not null" json:"cache_limit_gi"`
	CleanupIntervalHours    int32      `gorm:"not null" json:"cleanup_interval_hours"`
	LastCleanupAt           *time.Time `json:"last_cleanup_at,omitempty"`
	LastCheckedAt           *time.Time `json:"last_checked_at,omitempty"`
	Status                  string     `gorm:"size:32;not null" json:"status"`
	LastError               string     `gorm:"size:512" json:"last_error,omitempty"`
	EncryptedHTTPProxy      string     `gorm:"type:text" json:"-"`
	EncryptedHTTPSProxy     string     `gorm:"type:text" json:"-"`
	NoProxy                 string     `gorm:"size:1024" json:"no_proxy,omitempty"`
	DNSResolvers            string     `gorm:"type:text" json:"-"`
	DNSServers              []string   `gorm:"-" json:"dns_servers,omitempty"`
	LastDiagnosticStatus    string     `gorm:"size:64" json:"last_diagnostic_status,omitempty"`
	LastDiagnosticError     string     `gorm:"size:512" json:"last_diagnostic_error,omitempty"`
	LastDiagnosticAt        *time.Time `json:"last_diagnostic_at,omitempty"`
	OutboundProxyConfigured bool       `gorm:"-" json:"outbound_proxy_configured"`
	CreatedBy               uint       `gorm:"index;not null" json:"created_by"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

// ClusterDNSPolicy stores the platform-managed external CoreDNS forward
// targets. CoreDNS's remaining Corefile stays K3s-owned.

type ChartRepository struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	Name             string     `gorm:"size:128;uniqueIndex;not null" json:"name"`
	Endpoint         string     `gorm:"size:512;uniqueIndex;not null" json:"endpoint"`
	ChartName        string     `gorm:"size:256;not null" json:"chart_name"`
	ChartVersion     string     `gorm:"size:128;not null" json:"chart_version"`
	Enabled          bool       `gorm:"default:true;not null" json:"enabled"`
	LastVerifiedAt   *time.Time `json:"last_verified_at,omitempty"`
	LastVerifyStatus string     `gorm:"size:32" json:"last_verify_status"`
	LastVerifyError  string     `gorm:"size:512" json:"last_verify_error,omitempty"`
	CreatedBy        uint       `gorm:"index" json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// DNSCredential stores an encrypted DNS provider credential. Secret values are never exposed by APIs.
