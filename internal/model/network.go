package model

import "time"

// ResourceReference identifies an application-owned Kubernetes resource.
type ResourceReference struct {
	ApplicationID   uint   `json:"application_id"`
	ApplicationName string `json:"application_name"`
	Kind            string `json:"kind"`
	Name            string `json:"name"`
}

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
