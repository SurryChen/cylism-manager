package repository

import (
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
)

// AlertRepository contains persistent alert and automation policy state.
type AlertRepository interface {
	UpsertAlertEvent(*model.AlertEvent) (*model.AlertEvent, error)
	GetAlertEvent(uint) (*model.AlertEvent, error)
	ListAlertEvents(int) ([]model.AlertEvent, error)
	UpdateAlertEvent(*model.AlertEvent) error
	GetAlertAutomationPolicy() (*model.AlertAutomationPolicy, error)
	SaveAlertAutomationPolicy(*model.AlertAutomationPolicy) error
}

type AuditRepository interface {
	CreateAuditLog(*model.AuditLog) error
	ListAuditLogs(string, string, string, int, int) ([]model.AuditLog, int64, error)
	ListAuditLogsFiltered(model.AuditLogFilter) ([]model.AuditLog, int64, error)
}

type SystemComponentRepository interface {
	ListSystemComponentConfigs() ([]model.SystemComponentConfig, error)
	GetSystemComponentConfig(string) (*model.SystemComponentConfig, error)
	UpsertSystemComponentConfig(*model.SystemComponentConfig) error
	DeleteSystemComponentConfig(string) error
}

// AlertAutomationRepository is the narrow durable state used by alert
// automation. It intentionally excludes direct event lookup, which belongs to
// Agent read APIs rather than this workflow.
type AlertAutomationRepository interface {
	UpsertAlertEvent(*model.AlertEvent) (*model.AlertEvent, error)
	ListAlertEvents(int) ([]model.AlertEvent, error)
	UpdateAlertEvent(*model.AlertEvent) error
	GetAlertAutomationPolicy() (*model.AlertAutomationPolicy, error)
	SaveAlertAutomationPolicy(*model.AlertAutomationPolicy) error
	GetRuntime(uint) (*model.RuntimeInstance, error)
}

// LoggingScopeRepository validates optional application-related log filters.
type LoggingScopeRepository interface {
	GetApplication(uint) (*model.Application, error)
	GetEnvironmentByID(uint) (*model.Environment, error)
	GetProject(uint) (*model.Project, error)
}

// OperationLogRepository is the durable history used by release workflows
// and the operation history API.
type OperationLogRepository interface {
	CreateOperationLog(*model.OperationLog) error
	UpdateOperationLog(*model.OperationLog) error
	ListOperationsByResource(string, uint) ([]model.OperationLog, error)
	ListOperations(model.OperationLogFilter) ([]model.OperationLog, int64, error)
	DeleteExpiredOperationLogs(int) error
}

// DashboardRepository supplies the small cross-domain read model rendered by
// the management dashboard.
type DashboardRepository interface {
	GetDashboardStats(int) (*model.DashboardStats, error)
	GetDashboardApplicationSummary() (*model.DashboardApplicationSummary, error)
	ListExpiringCerts(int) ([]model.Cert, error)
	ListAuditLogs(string, string, string, int, int) ([]model.AuditLog, int64, error)
}

var (
	_ AlertRepository           = (*store.Store)(nil)
	_ AuditRepository           = (*store.Store)(nil)
	_ SystemComponentRepository = (*store.Store)(nil)
	_ AlertAutomationRepository = (*store.Store)(nil)
	_ LoggingScopeRepository    = (*store.Store)(nil)
	_ OperationLogRepository    = (*store.Store)(nil)
	_ DashboardRepository       = (*store.Store)(nil)
)
