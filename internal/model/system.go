package model

import "time"

// SystemComponentConfig records the desired configuration for a K3s system component.
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
