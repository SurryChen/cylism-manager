package model

import "time"

// Project is the business and authorization boundary for applications.
type Project struct {
	ID                     uint           `gorm:"primaryKey" json:"id"`
	Name                   string         `gorm:"size:128;uniqueIndex;not null" json:"name"`
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
	Name              string    `gorm:"size:64;uniqueIndex:idx_project_environment;not null" json:"name"`
	Namespace         string    `gorm:"size:128;not null" json:"namespace"`
	NamespaceStatus   string    `gorm:"-" json:"namespace_status,omitempty"`
	NamespaceConflict bool      `gorm:"-" json:"namespace_conflict,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
