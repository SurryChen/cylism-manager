package model

import "time"

// AccessCA is the platform trust anchor for cluster-agent mTLS identities.
type AccessCA struct {
	ID                  uint      `gorm:"primaryKey" json:"-"`
	CertificatePEM      string    `gorm:"type:text;not null" json:"certificate_pem"`
	EncryptedPrivateKey string    `gorm:"type:text;not null" json:"-"`
	Fingerprint         string    `gorm:"size:64;not null" json:"fingerprint"`
	NotAfter            time.Time `json:"not_after"`
	CreatedAt           time.Time `json:"created_at"`
}

// AccessCertificate records a cluster-bound identity and its revocation state.
type AccessCertificate struct {
	ID                  uint       `gorm:"primaryKey" json:"id"`
	ClusterID           uint       `gorm:"index;not null" json:"cluster_id"`
	Kind                string     `gorm:"size:32;index;not null" json:"kind"`
	Serial              string     `gorm:"size:64;uniqueIndex;not null" json:"serial"`
	Subject             string     `gorm:"size:256;not null" json:"subject"`
	Endpoint            string     `gorm:"size:256" json:"endpoint,omitempty"`
	CertificatePEM      string     `gorm:"type:text;not null" json:"-"`
	EncryptedPrivateKey string     `gorm:"type:text" json:"-"`
	Fingerprint         string     `gorm:"size:64;not null" json:"fingerprint"`
	NotAfter            time.Time  `json:"not_after"`
	InUse               bool       `gorm:"index;not null;default:false" json:"in_use"`
	RevokedAt           *time.Time `json:"revoked_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}
