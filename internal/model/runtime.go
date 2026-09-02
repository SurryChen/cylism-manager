package model

import (
	"time"
)

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
	AgentCapabilityAlertRead             = "alert.read"
	AgentCapabilityMonitoringRead        = "monitoring.read"
	AgentCapabilityMaintenanceInspect    = "maintenance.inspect"
	AgentCapabilityMaintenanceCleanup    = "maintenance.cleanup"
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
	AgentCapabilityAlertRead:             {},
	AgentCapabilityMonitoringRead:        {},
	AgentCapabilityMaintenanceInspect:    {},
	AgentCapabilityMaintenanceCleanup:    {},
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
