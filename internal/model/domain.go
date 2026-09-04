package model

import "time"

// ManagedDomain is a domain asset available for application releases.
type ManagedDomain struct {
	ID                   uint      `gorm:"primaryKey" json:"id"`
	Hostname             string    `gorm:"size:253;uniqueIndex;not null" json:"hostname"`
	EnvironmentID        uint      `gorm:"index" json:"environment_id,omitempty"`
	Namespace            string    `gorm:"size:128" json:"namespace"`
	CertificateName      string    `gorm:"size:253" json:"certificate_name"`
	TLSSecretName        string    `gorm:"size:253" json:"tls_secret_name"`
	IssuerRef            string    `gorm:"size:128" json:"issuer_ref"`
	IssuerKind           string    `gorm:"size:32;default:ClusterIssuer" json:"issuer_kind"`
	CertificateOwnership string    `gorm:"size:16;default:managed;not null" json:"certificate_ownership"`
	Description          string    `gorm:"size:512" json:"description"`
	Enabled              bool      `gorm:"default:true;not null" json:"enabled"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}
