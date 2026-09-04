package model

import "time"

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

func (IntegrationSession) TableName() string { return "integration_console_sessions" }
