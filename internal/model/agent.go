package model

import "time"

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
