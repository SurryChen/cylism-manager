package model

import "time"

// CloudConnection owns provider credentials used only by the platform backend.
type CloudConnection struct {
	ID                   uint   `gorm:"primaryKey" json:"id"`
	Name                 string `gorm:"size:128;uniqueIndex;not null" json:"name"`
	Provider             string `gorm:"size:64;index;not null" json:"provider"`
	EncryptedCredentials string `gorm:"type:text;not null" json:"-"`
	Configuration        string `gorm:"type:text" json:"configuration,omitempty"`
	// These legacy columns are retained for database compatibility. Capability
	// availability is now derived from the registered provider adapter.
	DNSEnabled           bool       `gorm:"not null;default:false" json:"-"`
	ObjectStorageEnabled bool       `gorm:"not null;default:false" json:"-"`
	DNSStatus            string     `gorm:"size:32" json:"-"`
	ObjectStorageStatus  string     `gorm:"size:32" json:"-"`
	LastValidationAt     *time.Time `json:"-"`
	LastValidationError  string     `gorm:"size:512" json:"-"`
	CreatedBy            uint       `gorm:"index;not null" json:"created_by"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	CredentialConfigured bool       `gorm:"-" json:"credential_configured"`
}
