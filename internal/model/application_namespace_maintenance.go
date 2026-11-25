package model

import (
	"errors"
	"time"
)

const (
	NamespaceMovePreparing = "preparing"
	NamespaceMovePrepared  = "prepared"
	NamespaceMoveStopping  = "stopping"
	NamespaceMoveStarting  = "starting"
	NamespaceMoveSwitching = "switching"
	NamespaceMoveRouted    = "routed"
	NamespaceMoveRestoring = "restoring"
	NamespaceMoveCanceling = "canceling"
)

var ErrApplicationNamespaceMaintenanceActive = errors.New("应用正在迁移运行环境，请先完成或取消迁移")

// ApplicationNamespaceMaintenance stores the checkpoint across Manager restarts.
// The plan contains resource identities and observed image IDs, never credentials.
type ApplicationNamespaceMaintenance struct {
	ID                  uint      `gorm:"primaryKey" json:"-"`
	ApplicationID       uint      `gorm:"uniqueIndex;not null" json:"-"`
	SourceEnvironmentID uint      `gorm:"not null" json:"source_environment_id"`
	TargetEnvironmentID uint      `gorm:"not null" json:"target_environment_id"`
	ReleaseID           uint      `gorm:"not null" json:"-"`
	OwnerID             uint      `gorm:"not null" json:"-"`
	Phase               string    `gorm:"not null" json:"phase"`
	Plan                string    `gorm:"type:text;not null" json:"-"`
	Detail              string    `json:"detail,omitempty"`
	CreatedAt           time.Time `json:"-"`
	UpdatedAt           time.Time `json:"-"`
}
