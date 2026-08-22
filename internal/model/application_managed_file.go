package model

import "time"

// ApplicationManagedFile grants an integration complete read/replace access
// to one file already mounted by an application. The platform does not
// interpret the file content or its schema.
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
