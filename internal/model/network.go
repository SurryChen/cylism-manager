package model

import (
	"time"

	"gorm.io/gorm"
)

// UpstreamConfig describes the upstream servers for a managed site.
type UpstreamConfig struct {
	Servers []UpstreamServer `json:"servers"`
}

// UpstreamServer is an upstream endpoint.
type UpstreamServer struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Weight int    `json:"weight,omitempty"`
}

// LocationConfig describes an additional location rule.
type LocationConfig struct {
	Path      string `json:"path"`
	ProxyPass string `json:"proxy_pass,omitempty"`
	Root      string `json:"root,omitempty"`
	Extra     string `json:"extra,omitempty"`
}

// ClusterDNSPolicy stores a versioned cluster DNS resolver policy.
type ClusterDNSPolicy struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Resolvers string    `gorm:"type:text;not null" json:"resolvers"`
	Revision  uint      `gorm:"uniqueIndex;not null" json:"revision"`
	Active    bool      `gorm:"not null;default:true" json:"active"`
	CreatedBy uint      `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// DNSCredential stores encrypted provider credentials for DNS challenges.
type DNSCredential struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	Name             string    `gorm:"size:128;uniqueIndex;not null" json:"name"`
	Provider         string    `gorm:"size:32;not null" json:"provider"`
	Namespace        string    `gorm:"size:128;not null" json:"namespace"`
	EncryptedValues  string    `gorm:"type:text" json:"-"`
	SecretName       string    `gorm:"size:253;uniqueIndex;not null" json:"secret_name"`
	Enabled          bool      `gorm:"default:true;not null" json:"enabled"`
	CreatedBy        uint      `gorm:"index;not null" json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	SecretConfigured bool      `gorm:"-" json:"secret_configured"`
	ConfiguredFields []string  `gorm:"-" json:"configured_fields"`
}

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

// Cert stores certificate material and lifecycle metadata for a site.
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

// Site is a managed web site served from a server.
type Site struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	ServerID      uint           `gorm:"index;not null" json:"server_id"`
	Domain        string         `gorm:"size:256;not null" json:"domain"`
	Port          int            `gorm:"default:80" json:"port"`
	SSLEnabled    bool           `gorm:"default:false" json:"ssl_enabled"`
	RootPath      string         `gorm:"size:512" json:"root_path"`
	Managed       bool           `gorm:"default:true" json:"managed"`
	NginxConfPath string         `gorm:"size:512" json:"nginx_conf_path"`
	Upstream      string         `gorm:"type:text" json:"upstream"`
	Locations     string         `gorm:"type:text" json:"locations"`
	CertID        *uint          `json:"cert_id"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
	Server        Server         `gorm:"foreignKey:ServerID" json:"-"`
	Cert          *Cert          `gorm:"foreignKey:CertID" json:"cert,omitempty"`
}
