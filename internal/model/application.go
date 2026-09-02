package model

import (
	"time"
)

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
	CapabilitiesData            string                `gorm:"column:capabilities;type:text;not null;default:'[]'" json:"-"`
	Capabilities                []string              `gorm:"-" json:"capabilities"`
	DefaultDeploymentTemplateID *uint                 `gorm:"index" json:"default_deployment_template_id,omitempty"`
	CreatedBy                   uint                  `gorm:"index;not null" json:"created_by"`
	CreatedAt                   time.Time             `json:"created_at"`
	UpdatedAt                   time.Time             `json:"updated_at"`
	Project                     Project               `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
	Environment                 Environment           `gorm:"foreignKey:EnvironmentID" json:"environment,omitempty"`
	Endpoints                   []ApplicationEndpoint `gorm:"foreignKey:ApplicationID" json:"endpoints,omitempty"`
}

// IntegrationSession is the server-side authority for an external integration.
// Credential values are never stored, only SHA-256 hashes.

type IntegrationSession struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	HandoffCodeHash  string     `gorm:"size:64;uniqueIndex;not null" json:"-"`
	SessionTokenHash *string    `gorm:"size:64;uniqueIndex" json:"-"`
	UserID           uint       `gorm:"index;not null" json:"user_id"`
	ProjectID        uint       `gorm:"index;not null" json:"project_id"`
	ApplicationID    uint       `gorm:"index;not null" json:"application_id"`
	EnvironmentID    uint       `gorm:"index;not null" json:"environment_id"`
	Capability       string     `gorm:"size:128;not null" json:"capability"`
	ActionsData      string     `gorm:"column:actions;type:text;not null" json:"-"`
	HandoffExpiresAt time.Time  `gorm:"index;not null" json:"handoff_expires_at"`
	HandoffUsedAt    *time.Time `json:"handoff_used_at,omitempty"`
	ExpiresAt        time.Time  `gorm:"index;not null" json:"expires_at"`
	RevokedAt        *time.Time `gorm:"index" json:"revoked_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// TableName preserves the existing schema while the API and code use generic
// integration terminology.

func (IntegrationSession) TableName() string { return "integration_console_sessions" }

// ImageRegistry 是可由项目授权使用的外部 OCI/Docker 镜像仓库。
// Credential 仅保存加密后的值，绝不能通过 API 返回。

const (
	// ApplicationEndpointAccessPublic opens the bound endpoint directly.
	ApplicationEndpointAccessPublic = "public"
	// ApplicationEndpointAccessProtectedConsole creates a short-lived console handoff before opening.
	ApplicationEndpointAccessProtectedConsole = "protected_console"
)

// ApplicationEndpoint 描述一个应用的 Service 暴露方式与可选 TLS 配置。

type ApplicationEndpoint struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	ApplicationID  uint      `gorm:"index;not null" json:"application_id"`
	DomainID       uint      `gorm:"index" json:"domain_id,omitempty"`
	Exposure       string    `gorm:"size:32;default:cluster;not null" json:"exposure"`
	Domain         string    `gorm:"size:256" json:"domain"`
	Path           string    `gorm:"size:256;default:/" json:"path"`
	ServicePort    int32     `gorm:"not null" json:"service_port"`
	Protocol       string    `gorm:"size:16;default:TCP;not null" json:"protocol"`
	TLSEnabled     bool      `gorm:"default:false" json:"tls_enabled"`
	IngressEnabled bool      `gorm:"default:true;not null" json:"ingress_enabled"`
	IngressMode    string    `gorm:"size:16;default:ingress;not null" json:"-"`
	TLSSecretName  string    `gorm:"size:253" json:"tls_secret_name"`
	IssuerRef      string    `gorm:"size:128" json:"issuer_ref"`
	AccessMode     string    `gorm:"size:32;default:public;not null" json:"access_mode"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
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
