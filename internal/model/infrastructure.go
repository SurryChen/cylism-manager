package model

import (
	"gorm.io/gorm"
	"time"
)

type ResourceReference struct {
	ApplicationID   uint   `json:"application_id"`
	ApplicationName string `json:"application_name"`
	Kind            string `json:"kind"`
	Name            string `json:"name"`
}

// DashboardStats is the persisted-data summary rendered by the dashboard.

type DashboardStats struct {
	TotalServers  int64 `json:"total_servers"`
	TotalSites    int64 `json:"total_sites"`
	ExpiringCerts int64 `json:"expiring_certs"`
}

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
