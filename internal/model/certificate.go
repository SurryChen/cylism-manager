package model

import "time"

// Cert 证书模型。
type Cert struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	SiteID        uint       `gorm:"index;not null" json:"site_id"`
	Domains       string     `gorm:"type:text" json:"domains"`
	Provider      string     `gorm:"size:32;default:acme.sh" json:"provider"`
	Account       string     `gorm:"size:256" json:"account"`
	CertPath      string     `gorm:"size:512" json:"cert_path"`
	KeyPath       string     `gorm:"size:512" json:"key_path"`
	FullchainPath string     `gorm:"size:512" json:"fullchain_path"`
	ValidFrom     time.Time  `json:"valid_from"`
	ValidTo       time.Time  `json:"valid_to"`
	Status        string     `gorm:"size:32" json:"status"`
	Challenge     string     `gorm:"size:16" json:"challenge"`
	LastRenew     *time.Time `json:"last_renew"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}
