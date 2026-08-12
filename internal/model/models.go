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

// RuntimeInstance describes an externally managed agent runtime. Conversation
// and memory bodies stay inside the runtime workspace and are never persisted
// by the Manager.
type RuntimeInstance struct {
	ID                     uint       `gorm:"primaryKey" json:"id"`
	Name                   string     `gorm:"size:128;uniqueIndex;not null" json:"name"`
	RuntimeType            string     `gorm:"size:32;not null" json:"runtime_type"`
	DeploymentMode         string     `gorm:"size:16;not null;default:managed" json:"deployment_mode"`
	RuntimeVersion         string     `gorm:"size:64" json:"runtime_version,omitempty"`
	Image                  string     `gorm:"size:512;not null" json:"image"`
	Namespace              string     `gorm:"size:128;not null;index" json:"namespace"`
	Port                   int32      `gorm:"not null;default:8080" json:"port"`
	HealthPath             string     `gorm:"size:256;not null;default:/health" json:"health_path"`
	PVCName                string     `gorm:"size:253;not null" json:"pvc_name"`
	Storage                string     `gorm:"size:32;not null;default:10Gi" json:"storage"`
	StorageClassName       string     `gorm:"size:128" json:"storage_class_name,omitempty"`
	NodeName               string     `gorm:"size:256" json:"node_name,omitempty"`
	EndpointURL            string     `gorm:"size:512" json:"endpoint_url,omitempty"`
	ModelName              string     `gorm:"size:128" json:"model_name,omitempty"`
	ModelBaseURL           string     `gorm:"size:512" json:"model_base_url,omitempty"`
	APIStyle               string     `gorm:"size:32;default:responses" json:"api_style"`
	EncryptedAPIKey        string     `gorm:"type:text" json:"-"`
	EncryptedRuntimeAPIKey string     `gorm:"type:text" json:"-"`
	APIKeyConfigured       bool       `gorm:"-" json:"api_key_configured"`
	Config                 string     `gorm:"type:text" json:"config,omitempty"`
	SecretName             string     `gorm:"size:253" json:"secret_name,omitempty"`
	Status                 string     `gorm:"size:32;index;not null" json:"status"`
	DesiredGeneration      uint       `gorm:"not null;default:1" json:"desired_generation"`
	ObservedGeneration     uint       `gorm:"not null;default:0" json:"observed_generation"`
	HealthStatus           string     `gorm:"size:32" json:"health_status,omitempty"`
	HealthDetail           string     `gorm:"size:512" json:"health_detail,omitempty"`
	LastHealthAt           *time.Time `json:"last_health_at,omitempty"`
	AgentToolEnabled       bool       `gorm:"not null;default:false" json:"agent_tool_enabled"`
	CreatedBy              uint       `gorm:"index;not null" json:"created_by"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

const (
	RuntimeTypeNanobot = "nanobot"

	RuntimeDeploymentManaged  = "managed"
	RuntimeDeploymentExternal = "external"

	RuntimeStatusDraft       = "draft"
	RuntimeStatusDeploying   = "deploying"
	RuntimeStatusReady       = "ready"
	RuntimeStatusDegraded    = "degraded"
	RuntimeStatusFailed      = "failed"
	RuntimeStatusUninstalled = "uninstalled"
)

const (
	AgentCapabilityClusterRead           = "cluster.read"
	AgentCapabilityWorkloadRead          = "workload.read"
	AgentCapabilityWorkloadLogs          = "workload.logs"
	AgentCapabilityEventsRead            = "events.read"
	AgentCapabilityStorageRead           = "storage.read"
	AgentCapabilityDeploymentScale       = "deployment.scale"
	AgentCapabilityRegistryRead          = "registry.read"
	AgentCapabilityRegistryVerify        = "registry.verify"
	AgentCapabilityRegistryPullCheck     = "registry.pull_check"
	AgentCapabilityDNSRead               = "dns.read"
	AgentCapabilityRegistryProxyDiagnose = "registry.proxy_diagnose"
)

var AgentCapabilities = map[string]struct{}{
	AgentCapabilityClusterRead:           {},
	AgentCapabilityWorkloadRead:          {},
	AgentCapabilityWorkloadLogs:          {},
	AgentCapabilityEventsRead:            {},
	AgentCapabilityStorageRead:           {},
	AgentCapabilityDeploymentScale:       {},
	AgentCapabilityRegistryRead:          {},
	AgentCapabilityRegistryVerify:        {},
	AgentCapabilityRegistryPullCheck:     {},
	AgentCapabilityDNSRead:               {},
	AgentCapabilityRegistryProxyDiagnose: {},
}

func ValidAgentCapability(capability string) bool {
	_, ok := AgentCapabilities[capability]
	return ok
}

// AgentCapabilityGrant is an administrator-managed permission for a Runtime.
// A namespace of "*" is an explicit cluster-wide scope.
type AgentCapabilityGrant struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	RuntimeID  uint      `gorm:"not null;uniqueIndex:idx_agent_capability_scope" json:"runtime_id"`
	Capability string    `gorm:"size:64;not null;uniqueIndex:idx_agent_capability_scope" json:"capability"`
	Namespace  string    `gorm:"size:253;not null;uniqueIndex:idx_agent_capability_scope" json:"namespace"`
	Enabled    bool      `gorm:"not null;default:false" json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

const (
	AgentOperationPendingApproval = "pending_approval"
	AgentOperationApproved        = "approved"
	AgentOperationRejected        = "rejected"
	AgentOperationSucceeded       = "succeeded"
	AgentOperationFailed          = "failed"
	AgentOperationStale           = "stale"
	AgentOperationExpired         = "expired"
)

// AgentOperation is the immutable request snapshot for a Runtime-issued
// platform action. Parameters are stored as a normalized, non-secret JSON
// document so approval always applies to the exact requested operation.
type AgentOperation struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	OperationID     string     `gorm:"size:64;uniqueIndex;not null" json:"operation_id"`
	RuntimeID       uint       `gorm:"not null;uniqueIndex:idx_agent_operation_request" json:"runtime_id"`
	Capability      string     `gorm:"size:64;not null" json:"capability"`
	RequestID       string     `gorm:"size:128;not null;uniqueIndex:idx_agent_operation_request" json:"request_id"`
	ChatSessionID   string     `gorm:"size:128" json:"chat_session_id,omitempty"`
	Parameters      string     `gorm:"type:text;not null" json:"parameters"`
	ParametersHash  string     `gorm:"size:64;not null" json:"parameters_hash"`
	ResourceVersion string     `gorm:"size:256" json:"resource_version,omitempty"`
	Status          string     `gorm:"size:32;index;not null" json:"status"`
	Summary         string     `gorm:"size:512;not null" json:"summary"`
	ErrorSummary    string     `gorm:"size:512" json:"error_summary,omitempty"`
	ApprovedBy      *uint      `gorm:"index" json:"approved_by,omitempty"`
	ApprovedAt      *time.Time `json:"approved_at,omitempty"`
	ExpiresAt       time.Time  `gorm:"index;not null" json:"expires_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	CreatedAt       time.Time  `gorm:"index" json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func AgentOperationTerminal(status string) bool {
	switch status {
	case AgentOperationRejected, AgentOperationSucceeded, AgentOperationFailed, AgentOperationStale, AgentOperationExpired:
		return true
	default:
		return false
	}
}

// PlatformRelease records a self-update request independently from application releases.
type PlatformRelease struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	Source        string     `gorm:"size:32;not null" json:"source"`
	Image         string     `gorm:"size:512;not null" json:"image"`
	PreviousImage string     `gorm:"size:512" json:"previous_image"`
	Status        string     `gorm:"size:32;index;not null" json:"status"`
	Detail        string     `gorm:"type:text" json:"detail"`
	CommitSHA     string     `gorm:"size:64" json:"commit_sha,omitempty"`
	RunID         string     `gorm:"size:64" json:"run_id,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// PlatformWebhookNonce prevents replay of accepted public deployment webhooks.
type PlatformWebhookNonce struct {
	ID        uint      `gorm:"primaryKey" json:"-"`
	Nonce     string    `gorm:"size:256;uniqueIndex;not null" json:"-"`
	ExpiresAt time.Time `gorm:"index;not null" json:"-"`
	CreatedAt time.Time `json:"-"`
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

	PVCMigrationStatusPending            = "pending"
	PVCMigrationStatusPreflight          = "preflight"
	PVCMigrationStatusProvisioningTarget = "provisioning_target"
	PVCMigrationStatusStoppingSource     = "stopping_source"
	PVCMigrationStatusCopying            = "copying"
	PVCMigrationStatusCutover            = "cutover"
	PVCMigrationStatusWaitingReady       = "waiting_ready"
	PVCMigrationStatusSucceeded          = "succeeded"
	PVCMigrationStatusFailed             = "failed"
	PVCMigrationStatusRollingBack        = "rolling_back"
	PVCMigrationStatusRolledBack         = "rolled_back"
	PVCMigrationStatusCleanupPending     = "cleanup_pending"
	PVCMigrationStatusCleaned            = "cleaned"
)

const (
	PVCImportStatusPending           = "pending"
	PVCImportStatusPreflight         = "preflight"
	PVCImportStatusStoppingWorkload  = "stopping_workload"
	PVCImportStatusBackingUp         = "backing_up"
	PVCImportStatusCopying           = "copying"
	PVCImportStatusVerifying         = "verifying"
	PVCImportStatusRestoringWorkload = "restoring_workload"
	PVCImportStatusSucceeded         = "succeeded"
	PVCImportStatusFailed            = "failed"
)

// PersistentVolumeMigration records a local PVC move independently from release snapshots.
type PersistentVolumeMigration struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	EnvironmentID      uint       `gorm:"index;not null" json:"environment_id"`
	ApplicationID      uint       `gorm:"index;not null" json:"application_id"`
	SourcePVCName      string     `gorm:"size:253;index;not null" json:"source_pvc_name"`
	TargetPVCName      string     `gorm:"size:253;not null" json:"target_pvc_name"`
	SourceNodeName     string     `gorm:"size:253;not null" json:"source_node_name"`
	TargetNodeName     string     `gorm:"size:253;not null" json:"target_node_name"`
	SourceDeployment   string     `gorm:"size:253;not null" json:"source_deployment"`
	SourceReplicas     int32      `json:"source_replicas"`
	Status             string     `gorm:"size:32;index;not null" json:"status"`
	Detail             string     `gorm:"type:text" json:"detail"`
	BytesCopied        int64      `json:"bytes_copied"`
	SourceTemplateSpec string     `gorm:"type:text" json:"-"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// PersistentVolumeBackup records a filesystem-level backup of a local PVC.
type PersistentVolumeBackup struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	EnvironmentID      uint       `gorm:"index;not null" json:"environment_id"`
	PVCName            string     `gorm:"size:253;not null" json:"pvc_name"`
	SourceNodeName     string     `gorm:"size:253" json:"source_node_name"`
	BackupServerID     uint       `gorm:"index;not null" json:"backup_server_id"`
	BackupPath         string     `gorm:"size:1024;not null" json:"backup_path"`
	Bytes              int64      `json:"bytes"`
	Status             string     `gorm:"size:32;index;not null" json:"status"`
	Detail             string     `gorm:"type:text" json:"detail"`
	RestoreStatus      string     `gorm:"size:32;index" json:"restore_status,omitempty"`
	RestoreDetail      string     `gorm:"type:text" json:"restore_detail,omitempty"`
	CreatedBy          uint       `gorm:"index;not null" json:"created_by"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	RestoreStartedAt   *time.Time `json:"restore_started_at,omitempty"`
	RestoreCompletedAt *time.Time `json:"restore_completed_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// HostDirectoryPVCImport records a controlled import from a managed server path
// into a local platform PVC, together with its retained rollback archive.
type HostDirectoryPVCImport struct {
	ID                   uint       `gorm:"primaryKey" json:"id"`
	EnvironmentID        uint       `gorm:"index;not null" json:"environment_id"`
	PVCName              string     `gorm:"size:253;index;not null" json:"pvc_name"`
	SourceServerID       uint       `gorm:"index;not null" json:"source_server_id"`
	SourceNodeName       string     `gorm:"size:253" json:"source_node_name"`
	SourcePath           string     `gorm:"size:1024;not null" json:"source_path"`
	TargetNodeName       string     `gorm:"size:253;not null" json:"target_node_name"`
	TargetPath           string     `gorm:"size:1024;not null" json:"target_path"`
	BackupPath           string     `gorm:"size:1024;not null" json:"backup_path"`
	BackupChecksum       string     `gorm:"size:128" json:"backup_checksum"`
	TargetBackupPath     string     `gorm:"size:1024" json:"target_backup_path,omitempty"`
	TargetBackupChecksum string     `gorm:"size:128" json:"target_backup_checksum,omitempty"`
	SourceChecksum       string     `gorm:"size:128" json:"source_checksum"`
	TargetChecksum       string     `gorm:"size:128" json:"target_checksum"`
	BytesCopied          int64      `json:"bytes_copied"`
	ApplicationReplicas  string     `gorm:"type:text" json:"-"`
	Status               string     `gorm:"size:32;index;not null" json:"status"`
	Detail               string     `gorm:"type:text" json:"detail"`
	VerifiedAt           *time.Time `json:"verified_at,omitempty"`
	BackupDeletedAt      *time.Time `json:"backup_deleted_at,omitempty"`
	CreatedBy            uint       `gorm:"index;not null" json:"created_by"`
	StartedAt            *time.Time `json:"started_at,omitempty"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

func IsPVCMigrationTerminal(status string) bool {
	switch status {
	case PVCMigrationStatusSucceeded, PVCMigrationStatusFailed, PVCMigrationStatusRolledBack, PVCMigrationStatusCleaned:
		return true
	default:
		return false
	}
}

func IsPVCImportTerminal(status string) bool {
	return status == PVCImportStatusSucceeded || status == PVCImportStatusFailed
}

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
	ID                          uint                  `gorm:"primaryKey" json:"id"`
	ProjectID                   uint                  `gorm:"index;not null" json:"project_id"`
	EnvironmentID               uint                  `gorm:"uniqueIndex:idx_environment_application;not null" json:"environment_id"`
	Name                        string                `gorm:"size:128;uniqueIndex:idx_environment_application;not null" json:"name"`
	WorkloadKind                string                `gorm:"size:32;default:deployment;not null" json:"workload_kind"`
	DefaultDeploymentTemplateID *uint                 `gorm:"index" json:"default_deployment_template_id,omitempty"`
	CreatedBy                   uint                  `gorm:"index;not null" json:"created_by"`
	CreatedAt                   time.Time             `json:"created_at"`
	UpdatedAt                   time.Time             `json:"updated_at"`
	Project                     Project               `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
	Environment                 Environment           `gorm:"foreignKey:EnvironmentID" json:"environment,omitempty"`
	Endpoints                   []ApplicationEndpoint `gorm:"foreignKey:ApplicationID" json:"endpoints,omitempty"`
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
type ClusterDNSPolicy struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Resolvers string    `gorm:"type:text;not null" json:"resolvers"`
	Revision  uint      `gorm:"uniqueIndex;not null" json:"revision"`
	Active    bool      `gorm:"not null;default:true" json:"active"`
	CreatedBy uint      `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// SystemComponentConfig 记录 K3s 内置系统组件的持久化期望配置，以及保存时
// 运行时探测到的控制模式。控制模式仅是快照，重放前仍需重新探测。
type SystemComponentConfig struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	ChartName      string     `gorm:"size:64;uniqueIndex;not null" json:"chart_name"`
	Namespace      string     `gorm:"size:128;not null;default:kube-system" json:"namespace"`
	ControllerMode string     `gorm:"size:32" json:"controller_mode"`
	ValuesContent  string     `gorm:"type:text" json:"values_content"`
	Enabled        bool       `gorm:"not null;default:true" json:"enabled"`
	LastAppliedAt  *time.Time `json:"last_applied_at,omitempty"`
	ApplyStatus    string     `gorm:"size:32" json:"apply_status"`
	ApplyError     string     `gorm:"type:text" json:"apply_error,omitempty"`
	CreatedBy      uint       `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
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
	ID                   uint      `gorm:"primaryKey" json:"id"`
	Hostname             string    `gorm:"size:253;uniqueIndex;not null" json:"hostname"`
	EnvironmentID        uint      `gorm:"index" json:"environment_id,omitempty"`
	Namespace            string    `gorm:"size:128" json:"namespace"`
	CertificateName      string    `gorm:"size:253" json:"certificate_name"`
	TLSSecretName        string    `gorm:"size:253" json:"tls_secret_name"`
	IssuerRef            string    `gorm:"size:128" json:"issuer_ref"`
	IssuerKind           string    `gorm:"size:32;default:ClusterIssuer" json:"issuer_kind"`
	CertificateOwnership string    `gorm:"size:16;default:managed;not null" json:"certificate_ownership"`
	Description          string    `gorm:"size:512" json:"description"`
	Enabled              bool      `gorm:"default:true;not null" json:"enabled"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
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

// ApplicationDeploymentTemplate stores one selectable rollout profile for an application.
// Each release keeps its own immutable resolved snapshot separately.
type ApplicationDeploymentTemplate struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	ApplicationID    uint      `gorm:"uniqueIndex:idx_application_deployment_template_name;not null" json:"application_id"`
	Name             string    `gorm:"size:128;uniqueIndex:idx_application_deployment_template_name;not null" json:"name"`
	Description      string    `gorm:"size:512" json:"description"`
	Enabled          bool      `gorm:"default:true;not null" json:"enabled"`
	Spec             string    `gorm:"type:text;not null" json:"spec"`
	EncryptedSecrets string    `gorm:"type:text" json:"-"`
	Revision         uint      `gorm:"not null" json:"revision"`
	UpdatedBy        uint      `gorm:"index" json:"updated_by"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Release 是应用一次不可变的期望状态快照；DesiredSpec 不得包含 Secret 明文。
type Release struct {
	ID                 uint               `gorm:"primaryKey" json:"id"`
	ApplicationID      uint               `gorm:"uniqueIndex:idx_application_sequence;not null" json:"application_id"`
	Sequence           uint               `gorm:"uniqueIndex:idx_application_sequence;not null" json:"sequence"`
	Image              string             `gorm:"size:512;not null" json:"image"`
	Version            string             `gorm:"size:128" json:"version,omitempty"`
	TemplateID         *uint              `gorm:"index" json:"template_id,omitempty"`
	TemplateRevision   uint               `json:"template_revision,omitempty"`
	ImageDigest        string             `gorm:"size:512" json:"image_digest"`
	ImageRegistryID    *uint              `gorm:"index" json:"image_registry_id,omitempty"`
	PodTrackingEnabled bool               `gorm:"default:false;not null" json:"pod_tracking_enabled"`
	DesiredSpec        string             `gorm:"type:text;not null" json:"desired_spec"`
	Status             string             `gorm:"size:32;index;not null" json:"status"`
	SourceReleaseID    *uint              `gorm:"index" json:"source_release_id,omitempty"`
	CreatedBy          uint               `gorm:"index;not null" json:"created_by"`
	StartedAt          *time.Time         `json:"started_at,omitempty"`
	CompletedAt        *time.Time         `json:"completed_at,omitempty"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
	Operations         []ReleaseOperation `gorm:"foreignKey:ReleaseID" json:"operations,omitempty"`
	Runtime            *ReleaseRuntime    `gorm:"-" json:"runtime,omitempty"`
}

// ReleaseRuntime represents the live Kubernetes state associated with a Release.
// It is intentionally never stored because Pods may be recreated at any time.
type ReleaseRuntime struct {
	Tracking   string              `json:"tracking"`
	Pods       []ReleasePodRuntime `json:"pods"`
	Diagnostic string              `json:"diagnostic,omitempty"`
}

type ReleasePodRuntime struct {
	Name       string                    `json:"name"`
	NodeName   string                    `json:"node_name,omitempty"`
	Phase      string                    `json:"phase"`
	Ready      bool                      `json:"ready"`
	Restarts   int32                     `json:"restarts"`
	CreatedAt  time.Time                 `json:"created_at"`
	Containers []ReleaseContainerRuntime `json:"containers"`
	Diagnostic string                    `json:"diagnostic,omitempty"`
}

type ReleaseContainerRuntime struct {
	Name         string `json:"name"`
	Image        string `json:"image,omitempty"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restart_count"`
	State        string `json:"state"`
	Reason       string `json:"reason,omitempty"`
	Message      string `json:"message,omitempty"`
	ExitCode     *int32 `json:"exit_code,omitempty"`
	LastState    string `json:"last_state,omitempty"`
	LastReason   string `json:"last_reason,omitempty"`
	LastExitCode *int32 `json:"last_exit_code,omitempty"`
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
