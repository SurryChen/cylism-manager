package model

import "time"

// ApplicationDeployWebhook holds one independently rotatable signing key per application.
type ApplicationDeployWebhook struct {
	ApplicationID   uint   `gorm:"primaryKey"`
	EncryptedSecret string `gorm:"type:text;not null"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ApplicationDeployWebhookNonce struct {
	ID            uint      `gorm:"primaryKey"`
	ApplicationID uint      `gorm:"uniqueIndex:idx_application_webhook_nonce;not null"`
	Nonce         string    `gorm:"size:256;uniqueIndex:idx_application_webhook_nonce;not null"`
	ExpiresAt     time.Time `gorm:"index;not null"`
}
