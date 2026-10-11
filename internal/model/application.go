package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

var ErrApplicationRequiresUninstall = errors.New("已有发布记录或访问入口，请先卸载集群资源")

// Project is the business and authorization boundary for applications.
type Project struct {
	ID                     uint           `gorm:"primaryKey" json:"id"`
	Name                   string         `gorm:"size:128;uniqueIndex;not null" json:"name"`
	Slug                   string         `gorm:"size:40;uniqueIndex:idx_projects_slug,where:slug <> ''" json:"slug"`
	Description            string         `gorm:"size:512" json:"description"`
	DefaultImageRegistryID *uint          `gorm:"index" json:"default_image_registry_id,omitempty"`
	OwnerID                uint           `gorm:"index;not null" json:"owner_id"`
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
	Environments           []Environment  `gorm:"foreignKey:ProjectID" json:"environments,omitempty"`
	DefaultImageRegistry   *ImageRegistry `gorm:"foreignKey:DefaultImageRegistryID" json:"default_image_registry,omitempty"`
}

// Environment maps a deployment target to a Kubernetes namespace.
type Environment struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	ProjectID         uint      `gorm:"uniqueIndex:idx_project_environment;not null" json:"project_id"`
	ClusterID         uint      `gorm:"index;not null;default:0" json:"cluster_id"`
	Name              string    `gorm:"size:64;uniqueIndex:idx_project_environment;not null" json:"name"`
	Namespace         string    `gorm:"size:128;not null" json:"namespace"`
	NamespaceStatus   string    `gorm:"-" json:"namespace_status,omitempty"`
	NamespaceConflict bool      `gorm:"-" json:"namespace_conflict,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// ResourceReference identifies an application-owned Kubernetes resource.
type ResourceReference struct {
	ApplicationID   uint   `json:"application_id"`
	ApplicationName string `json:"application_name"`
	Kind            string `json:"kind"`
	Name            string `json:"name"`
}

// NamespaceConflictError indicates that another environment already owns a Namespace.
type NamespaceConflictError struct {
	ClusterID     uint
	Namespace     string
	EnvironmentID uint
	ProjectID     uint
}

func (e *NamespaceConflictError) Error() string {
	return fmt.Sprintf("集群 %d 的命名空间 %q 已被项目 %d 的环境 %d 绑定", e.ClusterID, e.Namespace, e.ProjectID, e.EnvironmentID)
}

// NamespaceConflict groups legacy duplicate bindings for an operator-directed migration.
type NamespaceConflict struct {
	ClusterID    uint          `json:"cluster_id"`
	Namespace    string        `json:"namespace"`
	Environments []Environment `json:"environments"`
}

// TemplateRevisionConflictError indicates that a template changed after a caller read it.
type TemplateRevisionConflictError struct {
	Current  uint
	Expected uint
}

func (e *TemplateRevisionConflictError) Error() string {
	return fmt.Sprintf("模板版本冲突，当前版本为 %d，期望版本为 %d", e.Current, e.Expected)
}

// Application is a platform-managed stateless service.
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

// ApplicationEndpoint describes how an application Service is exposed.
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
	DefaultVersion   string    `gorm:"size:128" json:"default_version"`
	Spec             string    `gorm:"type:text;not null" json:"spec"`
	EncryptedSecrets string    `gorm:"type:text" json:"-"`
	Revision         uint      `gorm:"not null" json:"revision"`
	UpdatedBy        uint      `gorm:"index" json:"updated_by"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ApplicationCatalogItem is a reusable package made of deployment components.
type ApplicationCatalogItem struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Name           string    `gorm:"size:128;not null" json:"name"`
	Slug           string    `gorm:"column:application_name;size:63;uniqueIndex;not null" json:"slug"`
	Description    string    `gorm:"size:512" json:"description"`
	Revision       uint      `gorm:"not null;default:1" json:"revision"`
	Enabled        bool      `gorm:"not null;default:false" json:"enabled"`
	EntryComponent string    `gorm:"size:63" json:"entry_component"`
	ComponentsData string    `gorm:"column:components;type:text" json:"-"`
	LegacySpec     string    `gorm:"column:spec;type:text;not null" json:"-"`
	CreatedBy      uint      `gorm:"index;not null" json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ApplicationCatalogComponent struct {
	Name            string          `json:"name"`
	ApplicationName string          `json:"application_name"`
	DefaultVersion  string          `json:"default_version,omitempty"`
	Spec            json.RawMessage `json:"spec"`
}

func (item ApplicationCatalogItem) CatalogComponents() ([]ApplicationCatalogComponent, error) {
	if item.ComponentsData != "" {
		var components []ApplicationCatalogComponent
		if err := json.Unmarshal([]byte(item.ComponentsData), &components); err != nil {
			return nil, err
		}
		return components, nil
	}
	// Entries created before packages existed remain importable as one component.
	return []ApplicationCatalogComponent{{Name: item.Name, ApplicationName: item.Slug, Spec: json.RawMessage(item.LegacySpec)}}, nil
}

func (item ApplicationCatalogItem) CatalogEntryComponent() string {
	if item.EntryComponent != "" {
		return item.EntryComponent
	}
	return item.Slug
}

// ApplicationCatalogInstallation keeps the imported package identity after its
// components become ordinary project applications.
type ApplicationCatalogInstallation struct {
	ID                 uint                                      `gorm:"primaryKey" json:"id"`
	CatalogItemID      uint                                      `gorm:"index;not null" json:"catalog_item_id"`
	CatalogRevision    uint                                      `gorm:"not null" json:"catalog_revision"`
	Name               string                                    `gorm:"size:128;not null" json:"name"`
	ProjectID          uint                                      `gorm:"index;not null" json:"project_id"`
	EnvironmentID      uint                                      `gorm:"index;not null" json:"environment_id"`
	EntryApplicationID uint                                      `gorm:"index;not null" json:"entry_application_id"`
	CreatedAt          time.Time                                 `json:"created_at"`
	Components         []ApplicationCatalogInstallationComponent `gorm:"-" json:"components"`
}

type ApplicationCatalogInstallationComponent struct {
	ID             uint   `gorm:"primaryKey" json:"id"`
	InstallationID uint   `gorm:"index;not null" json:"installation_id"`
	ApplicationID  uint   `gorm:"index;not null" json:"application_id"`
	Name           string `gorm:"size:128;not null" json:"name"`
	IsEntry        bool   `json:"is_entry"`
}

// ApplicationManagedFile identifies a ConfigMap key exposed through an application template.
type ApplicationManagedFile struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ApplicationID uint      `gorm:"uniqueIndex:idx_application_managed_file_binding;not null" json:"application_id"`
	ResourceKind  string    `gorm:"size:16;uniqueIndex:idx_application_managed_file_binding;not null" json:"resource_kind"`
	ResourceName  string    `gorm:"size:253;uniqueIndex:idx_application_managed_file_binding;not null" json:"resource_name"`
	Key           string    `gorm:"size:253;uniqueIndex:idx_application_managed_file_binding;not null" json:"key"`
	MountPath     string    `gorm:"size:4096;not null" json:"mount_path"`
	Format        string    `gorm:"size:16;not null;default:text" json:"format"`
	Version       uint      `gorm:"not null;default:1" json:"version"`
	Enabled       bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedBy     uint      `gorm:"index;not null" json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// IntegrationSession is the server-side authority for an external integration.
type IntegrationSession struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	HandoffCodeHash  string     `gorm:"size:64;uniqueIndex;not null" json:"-"`
	SessionTokenHash *string    `gorm:"size:64;uniqueIndex" json:"-"`
	UserID           uint       `gorm:"index;not null" json:"user_id"`
	ProjectID        uint       `gorm:"index;not null" json:"project_id"`
	ApplicationID    uint       `gorm:"index;not null" json:"application_id"`
	EnvironmentID    uint       `gorm:"index;not null" json:"environment_id"`
	Capability       string     `gorm:"size:128;not null" json:"capability"`
	ActionsData      string     `gorm:"column:actions;type:text;not null" json:"-"`
	HandoffExpiresAt time.Time  `gorm:"index;not null" json:"handoff_expires_at"`
	HandoffUsedAt    *time.Time `json:"handoff_used_at,omitempty"`
	ExpiresAt        time.Time  `gorm:"index;not null" json:"expires_at"`
	RevokedAt        *time.Time `gorm:"index" json:"revoked_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (IntegrationSession) TableName() string { return "integration_console_sessions" }

const maxApplicationCapabilities = 16

var applicationCapabilityPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)

func NormalizeApplicationCapabilities(values []string) ([]string, error) {
	if len(values) > maxApplicationCapabilities {
		return nil, fmt.Errorf("能力标签最多 %d 项", maxApplicationCapabilities)
	}
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		capability := strings.ToLower(strings.TrimSpace(value))
		if !applicationCapabilityPattern.MatchString(capability) {
			return nil, fmt.Errorf("能力标签 %q 无效，只能使用小写字母、数字和连字符，长度不超过 64", value)
		}
		unique[capability] = struct{}{}
	}
	result := make([]string, 0, len(unique))
	for capability := range unique {
		result = append(result, capability)
	}
	sort.Strings(result)
	return result, nil
}

func (application *Application) SetCapabilities(values []string) error {
	normalized, err := NormalizeApplicationCapabilities(values)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	application.Capabilities = normalized
	application.CapabilitiesData = string(encoded)
	return nil
}

func (application *Application) LoadCapabilities() {
	application.Capabilities = []string{}
	if strings.TrimSpace(application.CapabilitiesData) == "" {
		return
	}
	var values []string
	if err := json.Unmarshal([]byte(application.CapabilitiesData), &values); err != nil {
		return
	}
	normalized, err := NormalizeApplicationCapabilities(values)
	if err == nil {
		application.Capabilities = normalized
	}
}
