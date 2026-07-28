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
	ID           uint          `gorm:"primaryKey" json:"id"`
	Name         string        `gorm:"size:128;uniqueIndex;not null" json:"name"`
	Description  string        `gorm:"size:512" json:"description"`
	OwnerID      uint          `gorm:"index;not null" json:"owner_id"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	Environments []Environment `gorm:"foreignKey:ProjectID" json:"environments,omitempty"`
}

// Environment 将应用部署目标映射到当前集群中的 Namespace。
type Environment struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ProjectID uint      `gorm:"uniqueIndex:idx_project_environment;not null" json:"project_id"`
	Name      string    `gorm:"size:64;uniqueIndex:idx_project_environment;not null" json:"name"`
	Namespace string    `gorm:"size:128;not null" json:"namespace"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

// ApplicationEndpoint 描述一个应用的 Service 暴露方式与可选 TLS 配置。
type ApplicationEndpoint struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ApplicationID uint      `gorm:"index;not null" json:"application_id"`
	Exposure      string    `gorm:"size:32;default:cluster;not null" json:"exposure"`
	Domain        string    `gorm:"size:256" json:"domain"`
	Path          string    `gorm:"size:256;default:/" json:"path"`
	ServicePort   int32     `gorm:"not null" json:"service_port"`
	TLSEnabled    bool      `gorm:"default:false" json:"tls_enabled"`
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
