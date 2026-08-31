package repository

import (
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
)

// RuntimeRepository is the durable state required by Runtime management.
type RuntimeRepository interface {
	CreateRuntime(*model.RuntimeInstance) error
	GetRuntime(uint) (*model.RuntimeInstance, error)
	ListRuntimes() ([]model.RuntimeInstance, error)
	UpdateRuntime(*model.RuntimeInstance) error
	UpdateRuntimeHealth(uint, string, string, time.Time) error
	DeleteRuntime(uint) error
}

// AgentCapabilityRepository owns Runtime capability grants and their lookup.
type AgentCapabilityRepository interface {
	ListAgentCapabilityGrants(uint) ([]model.AgentCapabilityGrant, error)
	ReplaceAgentCapabilityGrants(uint, []model.AgentCapabilityGrant) error
	HasAgentCapability(uint, string, string) (bool, error)
}

// AgentOperationRepository records approval-gated Agent operations.
type AgentOperationRepository interface {
	CreateAgentOperation(*model.AgentOperation) (*model.AgentOperation, bool, error)
	GetAgentOperation(string) (*model.AgentOperation, error)
	ListAgentOperations(uint, int, string, string) ([]model.AgentOperation, error)
	UpdateAgentOperationStatus(string, string, string, string, *uint, *time.Time) (bool, error)
}

// AgentReadRepository is the explicitly scoped persistence boundary exposed
// to Runtime-token API reads. Registry, DNS, alert and server methods are
// included only because those are Agent diagnostics, not because they belong
// to Runtime management.
type AgentReadRepository interface {
	HasAgentCapability(uint, string, string) (bool, error)
	ListAgentCapabilityGrants(uint) ([]model.AgentCapabilityGrant, error)
	CreateAgentOperation(*model.AgentOperation) (*model.AgentOperation, bool, error)
	GetAgentOperation(string) (*model.AgentOperation, error)
	GetAlertEvent(uint) (*model.AlertEvent, error)
	ListAlertEvents(int) ([]model.AlertEvent, error)
	UpdateAlertEvent(*model.AlertEvent) error
	GetAlertAutomationPolicy() (*model.AlertAutomationPolicy, error)
	ListNodeRegistryMirrors() ([]model.NodeRegistryMirror, error)
	ListRegistryProxies() ([]model.RegistryProxy, error)
	GetActiveClusterDNSPolicy() (*model.ClusterDNSPolicy, error)
	ListServers() ([]model.Server, error)
	CreateAuditLog(*model.AuditLog) error
}

// AgentOperationManagementRepository contains browser-side Agent approval
// state and the diagnostic records needed by an approved action.
type AgentOperationManagementRepository interface {
	GetRuntime(uint) (*model.RuntimeInstance, error)
	ListAgentCapabilityGrants(uint) ([]model.AgentCapabilityGrant, error)
	ReplaceAgentCapabilityGrants(uint, []model.AgentCapabilityGrant) error
	GetAgentOperation(string) (*model.AgentOperation, error)
	ListAgentOperations(uint, int, string, string) ([]model.AgentOperation, error)
	UpdateAgentOperationStatus(string, string, string, string, *uint, *time.Time) (bool, error)
	CreateAuditLog(*model.AuditLog) error
	ListNodeRegistryMirrors() ([]model.NodeRegistryMirror, error)
	ListRegistryProxies() ([]model.RegistryProxy, error)
	ListServers() ([]model.Server, error)
	GetAlertEvent(uint) (*model.AlertEvent, error)
	UpdateAlertEvent(*model.AlertEvent) error
}

// RuntimeManagementRepository combines the Runtime record and capability
// grants needed by the management API without exposing Agent operation state.
type RuntimeManagementRepository interface {
	RuntimeRepository
	AgentCapabilityRepository
}

var (
	_ RuntimeRepository                  = (*store.Store)(nil)
	_ AgentCapabilityRepository          = (*store.Store)(nil)
	_ AgentOperationRepository           = (*store.Store)(nil)
	_ RuntimeManagementRepository        = (*store.Store)(nil)
	_ AgentReadRepository                = (*store.Store)(nil)
	_ AgentOperationManagementRepository = (*store.Store)(nil)
)
