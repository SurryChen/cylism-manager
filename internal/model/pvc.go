package model

import "time"

type PersistentVolumeMigration struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	EnvironmentID      uint       `gorm:"index;not null" json:"environment_id"`
	ApplicationID      uint       `gorm:"index;not null" json:"application_id"`
	SourcePVCName      string     `gorm:"size:253;index;not null" json:"source_pvc_name"`
	TargetPVCName      string     `gorm:"size:253;not null" json:"target_pvc_name"`
	SourceNodeName     string     `gorm:"size:253;not null" json:"source_node_name"`
	TargetNodeName     string     `gorm:"size:253;not null" json:"target_node_name"`
	SourceDeployment   string     `gorm:"size:253;not null" json:"source_deployment"`
	SourceReplicas     int32      `json:"source_replicas"`
	Status             string     `gorm:"size:32;index;not null" json:"status"`
	Detail             string     `gorm:"type:text" json:"detail"`
	BytesCopied        int64      `json:"bytes_copied"`
	SourceTemplateSpec string     `gorm:"type:text" json:"-"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type PersistentVolumeBackup struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	EnvironmentID      uint       `gorm:"index;not null" json:"environment_id"`
	PVCName            string     `gorm:"size:253;not null" json:"pvc_name"`
	SourceNodeName     string     `gorm:"size:253" json:"source_node_name"`
	BackupServerID     uint       `gorm:"index;not null" json:"backup_server_id"`
	BackupPath         string     `gorm:"size:1024;not null" json:"backup_path"`
	Bytes              int64      `json:"bytes"`
	Status             string     `gorm:"size:32;index;not null" json:"status"`
	Detail             string     `gorm:"type:text" json:"detail"`
	RestoreStatus      string     `gorm:"size:32;index" json:"restore_status,omitempty"`
	RestoreDetail      string     `gorm:"type:text" json:"restore_detail,omitempty"`
	CreatedBy          uint       `gorm:"index;not null" json:"created_by"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	RestoreStartedAt   *time.Time `json:"restore_started_at,omitempty"`
	RestoreCompletedAt *time.Time `json:"restore_completed_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type HostDirectoryPVCImport struct {
	ID                   uint       `gorm:"primaryKey" json:"id"`
	EnvironmentID        uint       `gorm:"index;not null" json:"environment_id"`
	PVCName              string     `gorm:"size:253;index;not null" json:"pvc_name"`
	SourceServerID       uint       `gorm:"index;not null" json:"source_server_id"`
	SourceNodeName       string     `gorm:"size:253" json:"source_node_name"`
	SourcePath           string     `gorm:"size:1024;not null" json:"source_path"`
	TargetNodeName       string     `gorm:"size:253;not null" json:"target_node_name"`
	TargetPath           string     `gorm:"size:1024;not null" json:"target_path"`
	BackupPath           string     `gorm:"size:1024;not null" json:"backup_path"`
	BackupChecksum       string     `gorm:"size:128" json:"backup_checksum"`
	TargetBackupPath     string     `gorm:"size:1024" json:"target_backup_path,omitempty"`
	TargetBackupChecksum string     `gorm:"size:128" json:"target_backup_checksum,omitempty"`
	SourceChecksum       string     `gorm:"size:128" json:"source_checksum"`
	TargetChecksum       string     `gorm:"size:128" json:"target_checksum"`
	BytesCopied          int64      `json:"bytes_copied"`
	ApplicationReplicas  string     `gorm:"type:text" json:"-"`
	Status               string     `gorm:"size:32;index;not null" json:"status"`
	Detail               string     `gorm:"type:text" json:"detail"`
	VerifiedAt           *time.Time `json:"verified_at,omitempty"`
	BackupDeletedAt      *time.Time `json:"backup_deleted_at,omitempty"`
	CreatedBy            uint       `gorm:"index;not null" json:"created_by"`
	StartedAt            *time.Time `json:"started_at,omitempty"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}
