package model

import (
	"time"

	"gorm.io/gorm"
)

// Server 服务器模型
type Server struct {
	ID               uint           `gorm:"primaryKey" json:"id"`
	Name             string         `gorm:"size:128;not null" json:"name"`
	Host             string         `gorm:"size:256;uniqueIndex;not null" json:"host"`
	SSHPort          int            `gorm:"default:22" json:"ssh_port"`
	SSHUser          string         `gorm:"size:128" json:"ssh_user"`
	SSHAuthType      string         `gorm:"size:32" json:"ssh_auth_type"` // password / key
	SSHPassword      string         `gorm:"type:text" json:"-"`           // 加密存储
	SSHKey           string         `gorm:"type:text" json:"-"`           // 加密存储
	SSHKeyPassphrase string         `gorm:"type:text" json:"-"`           // 加密存储
	SSHKeyHash       string         `gorm:"size:64" json:"-"`             // 原始明文密钥的 MD5（加密前），用于解密后校验
	ClusterRole      string         `gorm:"size:32" json:"cluster_role"`  // "" | control-plane | worker
	K8sNodeName      string         `gorm:"size:256" json:"k8s_node_name"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

// Site 站点模型
type Site struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	ServerID      uint           `gorm:"index;not null" json:"server_id"`
	Domain        string         `gorm:"size:256;not null" json:"domain"`
	Port          int            `gorm:"default:80" json:"port"`
	SSLEnabled    bool           `gorm:"default:false" json:"ssl_enabled"`
	RootPath      string         `gorm:"size:512" json:"root_path"`
	Managed       bool           `gorm:"default:true" json:"managed"`
	NginxConfPath string         `gorm:"size:512" json:"nginx_conf_path"`
	Upstream      string         `gorm:"type:text" json:"upstream"`  // JSON
	Locations     string         `gorm:"type:text" json:"locations"` // JSON
	CertID        *uint          `json:"cert_id"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
	Server        Server         `gorm:"foreignKey:ServerID" json:"-"`
	Cert          *Cert          `gorm:"foreignKey:CertID" json:"cert,omitempty"`
}

// Cert 证书模型
type Cert struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	SiteID        uint       `gorm:"index;not null" json:"site_id"`
	Domains       string     `gorm:"type:text" json:"domains"` // JSON array
	Provider      string     `gorm:"size:32;default:acme.sh" json:"provider"`
	Account       string     `gorm:"size:256" json:"account"`
	CertPath      string     `gorm:"size:512" json:"cert_path"`
	KeyPath       string     `gorm:"size:512" json:"key_path"`
	FullchainPath string     `gorm:"size:512" json:"fullchain_path"`
	ValidFrom     time.Time  `json:"valid_from"`
	ValidTo       time.Time  `json:"valid_to"`
	Status        string     `gorm:"size:32" json:"status"`    // issued / renewing / expired / revoked
	Challenge     string     `gorm:"size:16" json:"challenge"` // http / dns
	LastRenew     *time.Time `json:"last_renew"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// AuditLog 审计日志模型
type AuditLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Action       string    `gorm:"size:64;index;not null" json:"action"`
	ResourceType string    `gorm:"size:64;index;not null" json:"resource_type"`
	ResourceID   uint      `gorm:"index" json:"resource_id"`
	UserID       uint      `gorm:"index"`
	Detail       string    `gorm:"type:text" json:"detail"` // JSON
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

// UpstreamConfig 上游代理配置
type UpstreamConfig struct {
	Servers []UpstreamServer `json:"servers"`
}

// UpstreamServer 上游服务器
type UpstreamServer struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Weight int    `json:"weight,omitempty"`
}

// LocationConfig 额外的 location 规则
type LocationConfig struct {
	Path      string `json:"path"`
	ProxyPass string `json:"proxy_pass,omitempty"`
	Root      string `json:"root,omitempty"`
	Extra     string `json:"extra,omitempty"`
}

// User 用户模型
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"size:128;uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"size:256;not null" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// OperationLog 操作日志模型（通用，适用于所有长流程操作）
type OperationLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	ResourceType string    `gorm:"size:64;index:idx_resource;not null" json:"resource_type"`
	ResourceID   uint      `gorm:"index:idx_resource;not null" json:"resource_id"`
	Step         string    `gorm:"size:128;not null" json:"step"`
	Status       string    `gorm:"size:16;default:running" json:"status"` // running / success / failed
	Detail       string    `gorm:"type:text" json:"detail"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

// SystemConfig 系统配置（加密存储敏感信息如 Tailscale Auth Key）
type SystemConfig struct {
	ID    uint   `gorm:"primaryKey" json:"-"`
	Key   string `gorm:"size:128;uniqueIndex" json:"key"`
	Value string `gorm:"type:text" json:"-"` // AES-256 加密
}

const (
	ReleaseStatusDraft        = "draft"
	ReleaseStatusValidating   = "validating"
	ReleaseStatusApplying     = "applying"
	ReleaseStatusWaitingReady = "waiting_ready"
	ReleaseStatusVerifying    = "verifying"
	ReleaseStatusSucceeded    = "succeeded"
	ReleaseStatusFailed       = "failed"
	ReleaseStatusRollingBack  = "rolling_back"
	ReleaseStatusRolledBack   = "rolled_back"

	ReleaseOperationRunning = "running"
	ReleaseOperationSuccess = "success"
	ReleaseOperationFailed  = "failed"
)

// Project 应用所属的业务与授权边界。
type Project struct {
	ID                     uint           `gorm:"primaryKey" json:"id"`
	Name                   string         `gorm:"size:128;uniqueIndex;not null" json:"name"`
	Description            string         `gorm:"size:512" json:"description"`
	DefaultImageRegistryID *uint          `gorm:"index" json:"default_image_registry_id,omitempty"`
	OwnerID                uint           `gorm:"index;not null" json:"owner_id"`
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
	Environments           []Environment  `gorm:"foreignKey:ProjectID" json:"environments,omitempty"`
	DefaultImageRegistry   *ImageRegistry `gorm:"foreignKey:DefaultImageRegistryID" json:"default_image_registry,omitempty"`
}

// Environment 将应用部署目标映射到当前集群中的 Namespace。
type Environment struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	ProjectID         uint      `gorm:"uniqueIndex:idx_project_environment;not null" json:"project_id"`
	Name              string    `gorm:"size:64;uniqueIndex:idx_project_environment;not null" json:"name"`
	Namespace         string    `gorm:"size:128;not null" json:"namespace"`
	NamespaceStatus   string    `gorm:"-" json:"namespace_status,omitempty"`
	NamespaceConflict bool      `gorm:"-" json:"namespace_conflict,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Application 是平台托管的单个无状态服务。
type Application struct {
	ID            uint                  `gorm:"primaryKey" json:"id"`
	ProjectID     uint                  `gorm:"index;not null" json:"project_id"`
	EnvironmentID uint                  `gorm:"uniqueIndex:idx_environment_application;not null" json:"environment_id"`
	Name          string                `gorm:"size:128;uniqueIndex:idx_environment_application;not null" json:"name"`
	WorkloadKind  string                `gorm:"size:32;default:deployment;not null" json:"workload_kind"`
	CreatedBy     uint                  `gorm:"index;not null" json:"created_by"`
	CreatedAt     time.Time             `json:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at"`
	Project       Project               `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
	Environment   Environment           `gorm:"foreignKey:EnvironmentID" json:"environment,omitempty"`
	Endpoints     []ApplicationEndpoint `gorm:"foreignKey:ApplicationID" json:"endpoints,omitempty"`
}

// ImageRegistry 是可由项目授权使用的外部 OCI/Docker 镜像仓库。
// Credential 仅保存加密后的值，绝不能通过 API 返回。
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

// ChartRepository is a vetted Helm chart source for platform-managed extensions.
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
type DNSCredential struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	Name             string    `gorm:"size:128;uniqueIndex;not null" json:"name"`
	Provider         string    `gorm:"size:32;not null" json:"provider"`
	Namespace        string    `gorm:"size:128;not null" json:"namespace"`
	EncryptedValues  string    `gorm:"type:text" json:"-"`
	SecretName       string    `gorm:"size:253;uniqueIndex;not null" json:"secret_name"`
	Enabled          bool      `gorm:"default:true;not null" json:"enabled"`
	CreatedBy        uint      `gorm:"index;not null" json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	SecretConfigured bool      `gorm:"-" json:"secret_configured"`
	ConfiguredFields []string  `gorm:"-" json:"configured_fields"`
}

// ManagedDomain 是可供应用发布选择的域名资产。
type ManagedDomain struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	Hostname        string    `gorm:"size:253;uniqueIndex;not null" json:"hostname"`
	EnvironmentID   uint      `gorm:"index" json:"environment_id,omitempty"`
	Namespace       string    `gorm:"size:128" json:"namespace"`
	CertificateName string    `gorm:"size:253" json:"certificate_name"`
	TLSSecretName   string    `gorm:"size:253" json:"tls_secret_name"`
	IssuerRef       string    `gorm:"size:128" json:"issuer_ref"`
	IssuerKind      string    `gorm:"size:32;default:ClusterIssuer" json:"issuer_kind"`
	Description     string    `gorm:"size:512" json:"description"`
	Enabled         bool      `gorm:"default:true;not null" json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ApplicationEndpoint 描述一个应用的 Service 暴露方式与可选 TLS 配置。
type ApplicationEndpoint struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ApplicationID uint      `gorm:"index;not null" json:"application_id"`
	DomainID      uint      `gorm:"index" json:"domain_id,omitempty"`
	Exposure      string    `gorm:"size:32;default:cluster;not null" json:"exposure"`
	Domain        string    `gorm:"size:256" json:"domain"`
	Path          string    `gorm:"size:256;default:/" json:"path"`
	ServicePort   int32     `gorm:"not null" json:"service_port"`
	TLSEnabled    bool      `gorm:"default:false" json:"tls_enabled"`
	TLSSecretName string    `gorm:"size:253" json:"tls_secret_name"`
	IssuerRef     string    `gorm:"size:128" json:"issuer_ref"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Release 是应用一次不可变的期望状态快照；DesiredSpec 不得包含 Secret 明文。
type Release struct {
	ID              uint               `gorm:"primaryKey" json:"id"`
	ApplicationID   uint               `gorm:"uniqueIndex:idx_application_sequence;not null" json:"application_id"`
	Sequence        uint               `gorm:"uniqueIndex:idx_application_sequence;not null" json:"sequence"`
	Image           string             `gorm:"size:512;not null" json:"image"`
	ImageDigest     string             `gorm:"size:512" json:"image_digest"`
	ImageRegistryID *uint              `gorm:"index" json:"image_registry_id,omitempty"`
	DesiredSpec     string             `gorm:"type:text;not null" json:"desired_spec"`
	Status          string             `gorm:"size:32;index;not null" json:"status"`
	SourceReleaseID *uint              `gorm:"index" json:"source_release_id,omitempty"`
	CreatedBy       uint               `gorm:"index;not null" json:"created_by"`
	StartedAt       *time.Time         `json:"started_at,omitempty"`
	CompletedAt     *time.Time         `json:"completed_at,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
	Operations      []ReleaseOperation `gorm:"foreignKey:ReleaseID" json:"operations,omitempty"`
}

// ReleaseOperation 记录单次发布的步骤级进度，Detail 仅允许保存脱敏诊断信息。
type ReleaseOperation struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	ReleaseID   uint       `gorm:"index;not null" json:"release_id"`
	Step        string     `gorm:"size:128;not null" json:"step"`
	Status      string     `gorm:"size:16;not null" json:"status"`
	Detail      string     `gorm:"type:text" json:"detail"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func IsReleaseStatusValid(status string) bool {
	switch status {
	case ReleaseStatusDraft, ReleaseStatusValidating, ReleaseStatusApplying, ReleaseStatusWaitingReady,
		ReleaseStatusVerifying, ReleaseStatusSucceeded, ReleaseStatusFailed, ReleaseStatusRollingBack, ReleaseStatusRolledBack:
		return true
	default:
		return false
	}
}
