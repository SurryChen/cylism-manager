package model

import (
	"fmt"
	"time"
)

// DashboardStats is the persisted-data summary rendered by the dashboard.
type DashboardStats struct {
	TotalServers  int64 `json:"total_servers"`
	TotalSites    int64 `json:"total_sites"`
	ExpiringCerts int64 `json:"expiring_certs"`
	ExpiredCerts  int64 `json:"expired_certs"`
}

// DashboardApplicationSummary is the current release state aggregated per
// application for the operational dashboard.
type DashboardApplicationSummary struct {
	TotalApplications      int64
	SuccessfulApplications int64
	ReleasingApplications  int64
	FailedApplications     int64
	UnreleasedApplications int64
	LatestRelease          *DashboardLatestRelease
}

type DashboardLatestRelease struct {
	ApplicationName string
	Version         string
	Status          string
	CreatedAt       time.Time
}

type AuditLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Action       string    `gorm:"size:64;index;not null" json:"action"`
	ResourceType string    `gorm:"size:64;index;not null" json:"resource_type"`
	ResourceID   uint      `gorm:"index" json:"resource_id"`
	TargetName   string    `gorm:"size:256;index" json:"target_name"`
	UserID       uint      `gorm:"index"`
	ActorType    string    `gorm:"size:32;index" json:"actor_type"`
	ActorName    string    `gorm:"size:128;index" json:"actor_name"`
	Source       string    `gorm:"size:32;index" json:"source"`
	Outcome      string    `gorm:"size:16;index" json:"outcome"`
	Summary      string    `gorm:"size:512" json:"summary"`
	RequestID    string    `gorm:"size:128;index" json:"request_id"`
	OperationID  string    `gorm:"size:128;index" json:"operation_id"`
	Detail       string    `gorm:"type:text" json:"detail"` // JSON
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

const (
	AuditActorUser   = "user"
	AuditActorAgent  = "agent"
	AuditActorSystem = "system"

	AuditSourceAPI        = "api"
	AuditSourceAgent      = "agent"
	AuditSourceDelegation = "delegation"
	AuditSourceSystem     = "system"
	AuditSourceLegacy     = "legacy"

	AuditOutcomeSucceeded = "succeeded"
	AuditOutcomeFailed    = "failed"
	AuditOutcomeDenied    = "denied"
	AuditOutcomeAccepted  = "accepted"
)

// AuditTargetFallback makes a pre-structured record understandable without
// relying on its legacy JSON detail body.
func AuditTargetFallback(resourceType string, resourceID uint) string {
	if resourceID == 0 {
		return resourceType
	}
	return fmt.Sprintf("%s #%d", resourceType, resourceID)
}

func AuditSummaryFallback(action, resourceType string, resourceID uint) string {
	return fmt.Sprintf("执行 %s：%s", action, AuditTargetFallback(resourceType, resourceID))
}

// AuditLogFilter is the structured query contract for the audit timeline.
type AuditLogFilter struct {
	ResourceType string
	Action       string
	Outcome      string
	Source       string
	ActorType    string
	TargetName   string
	Keyword      string
	CreatedFrom  time.Time
	CreatedTo    time.Time
	Limit        int
	Offset       int
}

// OperationLogFilter is the global operation-history query contract.
type OperationLogFilter struct {
	ResourceType string
	Status       string
	Keyword      string
	Limit        int
	Offset       int
}

// UpstreamConfig 上游代理配置

type OperationLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	ResourceType string    `gorm:"size:64;index:idx_resource;not null" json:"resource_type"`
	ResourceID   uint      `gorm:"index:idx_resource;not null" json:"resource_id"`
	Step         string    `gorm:"size:128;not null" json:"step"`
	Status       string    `gorm:"size:16;default:running" json:"status"` // running / success / failed
	Detail       string    `gorm:"type:text" json:"detail"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

// SystemConfig 系统配置（加密存储敏感配置）

type SystemConfig struct {
	ID    uint   `gorm:"primaryKey" json:"-"`
	Key   string `gorm:"size:128;uniqueIndex" json:"key"`
	Value string `gorm:"type:text" json:"-"` // AES-256 加密
}

// RuntimeInstance describes an externally managed agent runtime. Conversation
// and memory bodies stay inside the runtime workspace and are never persisted
// by the Manager.

const (
	AlertEventFiring           = "firing"
	AlertEventAnalyzing        = "analyzing"
	AlertEventAwaitingApproval = "awaiting_approval"
	AlertEventRemediating      = "remediating"
	AlertEventResolved         = "resolved"
	AlertEventFailed           = "failed"

	AlertAutomationReportOnly = "report_only"
	AlertAutomationApproval   = "diagnose_and_request_approval"
)

// AlertEvent is the durable record of one Alertmanager fingerprint. It holds
// automation state separately from Alertmanager's ephemeral query response.

type AlertEvent struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	Fingerprint       string     `gorm:"size:128;uniqueIndex;not null" json:"fingerprint"`
	AlertName         string     `gorm:"size:128;index;not null" json:"alert_name"`
	Severity          string     `gorm:"size:32;index" json:"severity"`
	NodeName          string     `gorm:"size:253;index" json:"node_name,omitempty"`
	MountPoint        string     `gorm:"size:256" json:"mount_point,omitempty"`
	Labels            string     `gorm:"type:text;not null" json:"labels"`
	Annotations       string     `gorm:"type:text;not null" json:"annotations"`
	Status            string     `gorm:"size:32;index;not null" json:"status"`
	RuntimeID         *uint      `gorm:"index" json:"runtime_id,omitempty"`
	SessionID         string     `gorm:"size:128" json:"session_id,omitempty"`
	Report            string     `gorm:"type:text" json:"report,omitempty"`
	DiagnosticSummary string     `gorm:"size:512" json:"diagnostic_summary,omitempty"`
	OperationID       string     `gorm:"size:64;index" json:"operation_id,omitempty"`
	LastError         string     `gorm:"size:512" json:"last_error,omitempty"`
	StartsAt          time.Time  `gorm:"index;not null" json:"starts_at"`
	EndsAt            *time.Time `gorm:"index" json:"ends_at,omitempty"`
	LastDispatchedAt  *time.Time `gorm:"index" json:"last_dispatched_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// AlertAutomationPolicy controls a single, explicit automation route. A
// disabled policy is the default and no alert can invoke a Runtime implicitly.

type AlertAutomationPolicy struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	RuntimeID       uint      `gorm:"not null" json:"runtime_id"`
	Enabled         bool      `gorm:"not null;default:false" json:"enabled"`
	AlertName       string    `gorm:"size:128" json:"alert_name,omitempty"`
	MinimumSeverity string    `gorm:"size:32;not null;default:warning" json:"minimum_severity"`
	Mode            string    `gorm:"size:64;not null;default:report_only" json:"mode"`
	CooldownMinutes int       `gorm:"not null;default:30" json:"cooldown_minutes"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
