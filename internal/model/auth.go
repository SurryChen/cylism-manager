package model

import (
	"time"
)

type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"size:128;uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"size:256;not null" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TemporaryLoginToken stores only the digest of a short-lived login secret.
type TemporaryLoginToken struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	TokenHash  string     `gorm:"size:64;uniqueIndex;not null" json:"-"`
	CreatedBy  uint       `gorm:"index;not null" json:"created_by"`
	Label      string     `gorm:"size:128" json:"label"`
	ExpiresAt  time.Time  `gorm:"index;not null" json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// OperationLog 操作日志模型（通用，适用于所有长流程操作）
