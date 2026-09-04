package model

import "time"

type Application struct {
	ID                          uint                  `gorm:"primaryKey" json:"id"`
	ProjectID                   uint                  `gorm:"index;not null" json:"project_id"`
	EnvironmentID               uint                  `gorm:"uniqueIndex:idx_environment_application;not null" json:"environment_id"`
	Name                        string                `gorm:"size:128;uniqueIndex:idx_environment_application;not null" json:"name"`
	WorkloadKind                string                `gorm:"size:32;default:deployment;not null" json:"workload_kind"`
	CapabilitiesData            string                `gorm:"column:capabilities;type:text;not null;default:'[]'" json:"-"`
	Capabilities                []string              `gorm:"-" json:"capabilities"`
	DefaultDeploymentTemplateID *uint                 `gorm:"index" json:"default_deployment_template_id,omitempty"`
	CreatedBy                   uint                  `gorm:"index;not null" json:"created_by"`
	CreatedAt                   time.Time             `json:"created_at"`
	UpdatedAt                   time.Time             `json:"updated_at"`
	Project                     Project               `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
	Environment                 Environment           `gorm:"foreignKey:EnvironmentID" json:"environment,omitempty"`
	Endpoints                   []ApplicationEndpoint `gorm:"foreignKey:ApplicationID" json:"endpoints,omitempty"`
}

const (
	ApplicationEndpointAccessPublic           = "public"
	ApplicationEndpointAccessProtectedConsole = "protected_console"
)

type ApplicationEndpoint struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	ApplicationID  uint      `gorm:"index;not null" json:"application_id"`
	DomainID       uint      `gorm:"index" json:"domain_id,omitempty"`
	Exposure       string    `gorm:"size:32;default:cluster;not null" json:"exposure"`
	Domain         string    `gorm:"size:256" json:"domain"`
	Path           string    `gorm:"size:256;default:/" json:"path"`
	ServicePort    int32     `gorm:"not null" json:"service_port"`
	Protocol       string    `gorm:"size:16;default:TCP;not null" json:"protocol"`
	TLSEnabled     bool      `gorm:"default:false" json:"tls_enabled"`
	IngressEnabled bool      `gorm:"default:true;not null" json:"ingress_enabled"`
	IngressMode    string    `gorm:"size:16;default:ingress;not null" json:"-"`
	TLSSecretName  string    `gorm:"size:253" json:"tls_secret_name"`
	IssuerRef      string    `gorm:"size:128" json:"issuer_ref"`
	AccessMode     string    `gorm:"size:32;default:public;not null" json:"access_mode"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ApplicationDeploymentTemplate struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	ApplicationID    uint      `gorm:"uniqueIndex:idx_application_deployment_template_name;not null" json:"application_id"`
	Name             string    `gorm:"size:128;uniqueIndex:idx_application_deployment_template_name;not null" json:"name"`
	Description      string    `gorm:"size:512" json:"description"`
	Enabled          bool      `gorm:"default:true;not null" json:"enabled"`
	Spec             string    `gorm:"type:text;not null" json:"spec"`
	EncryptedSecrets string    `gorm:"type:text" json:"-"`
	Revision         uint      `gorm:"not null" json:"revision"`
	UpdatedBy        uint      `gorm:"index" json:"updated_by"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
