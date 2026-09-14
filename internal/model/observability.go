package model

import (
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
	UserID       uint      `gorm:"index"`
	Detail       string    `gorm:"type:text" json:"detail"` // JSON
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
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

// SystemConfig 系统配置（加密存储敏感信息如 Tailscale Auth Key）

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
