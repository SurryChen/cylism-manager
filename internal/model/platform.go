package model

import (
	"time"
)

type PlatformRelease struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	Source        string     `gorm:"size:32;not null" json:"source"`
	Image         string     `gorm:"size:512;not null" json:"image"`
	PreviousImage string     `gorm:"size:512" json:"previous_image"`
	Status        string     `gorm:"size:32;index;not null" json:"status"`
	Detail        string     `gorm:"type:text" json:"detail"`
	CommitSHA     string     `gorm:"size:64" json:"commit_sha,omitempty"`
	RunID         string     `gorm:"size:64" json:"run_id,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// PlatformEndpoint stores the one public management entry owned by Cylism
// Manager itself. It is intentionally separate from project application
// endpoints because the platform runs in the control-plane namespace.

type PlatformEndpoint struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	Hostname        string    `gorm:"size:253;not null" json:"hostname"`
	IngressName     string    `gorm:"size:253" json:"ingress_name"`
	CertificateName string    `gorm:"size:253" json:"certificate_name"`
	TLSSecretName   string    `gorm:"size:253" json:"tls_secret_name"`
	Enabled         bool      `gorm:"default:false;not null" json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// PlatformWebhookNonce prevents replay of accepted public deployment webhooks.

type PlatformWebhookNonce struct {
	ID        uint      `gorm:"primaryKey" json:"-"`
	Nonce     string    `gorm:"size:256;uniqueIndex;not null" json:"-"`
	ExpiresAt time.Time `gorm:"index;not null" json:"-"`
	CreatedAt time.Time `json:"-"`
}
