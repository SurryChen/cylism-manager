package model

import "time"

type Release struct {
	ID                 uint               `gorm:"primaryKey" json:"id"`
	ApplicationID      uint               `gorm:"uniqueIndex:idx_application_sequence;not null" json:"application_id"`
	Sequence           uint               `gorm:"uniqueIndex:idx_application_sequence;not null" json:"sequence"`
	Image              string             `gorm:"size:512;not null" json:"image"`
	Version            string             `gorm:"size:128" json:"version,omitempty"`
	TemplateID         *uint              `gorm:"index" json:"template_id,omitempty"`
	TemplateRevision   uint               `json:"template_revision,omitempty"`
	ImageDigest        string             `gorm:"size:512" json:"image_digest"`
	ImageRegistryID    *uint              `gorm:"index" json:"image_registry_id,omitempty"`
	PodTrackingEnabled bool               `gorm:"default:false;not null" json:"pod_tracking_enabled"`
	DesiredSpec        string             `gorm:"type:text;not null" json:"desired_spec"`
	Status             string             `gorm:"size:32;index;not null" json:"status"`
	SourceReleaseID    *uint              `gorm:"index" json:"source_release_id,omitempty"`
	CreatedBy          uint               `gorm:"index;not null" json:"created_by"`
	StartedAt          *time.Time         `json:"started_at,omitempty"`
	CompletedAt        *time.Time         `json:"completed_at,omitempty"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
	Operations         []ReleaseOperation `gorm:"foreignKey:ReleaseID" json:"operations,omitempty"`
	Runtime            *ReleaseRuntime    `gorm:"-" json:"runtime,omitempty"`
}

type ReleaseRuntime struct {
	Tracking   string              `json:"tracking"`
	Pods       []ReleasePodRuntime `json:"pods"`
	Diagnostic string              `json:"diagnostic,omitempty"`
}

type ReleasePodRuntime struct {
	Name       string                    `json:"name"`
	NodeName   string                    `json:"node_name,omitempty"`
	Phase      string                    `json:"phase"`
	Ready      bool                      `json:"ready"`
	Restarts   int32                     `json:"restarts"`
	CreatedAt  time.Time                 `json:"created_at"`
	Containers []ReleaseContainerRuntime `json:"containers"`
	Diagnostic string                    `json:"diagnostic,omitempty"`
}

type ReleaseContainerRuntime struct {
	Name         string `json:"name"`
	Image        string `json:"image,omitempty"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restart_count"`
	State        string `json:"state"`
	Reason       string `json:"reason,omitempty"`
	Message      string `json:"message,omitempty"`
	ExitCode     *int32 `json:"exit_code,omitempty"`
	LastState    string `json:"last_state,omitempty"`
	LastReason   string `json:"last_reason,omitempty"`
	LastExitCode *int32 `json:"last_exit_code,omitempty"`
}

type ReleaseOperation struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	ReleaseID   uint       `gorm:"index;not null" json:"release_id"`
	Step        string     `gorm:"size:128;not null" json:"step"`
	Status      string     `gorm:"size:16;not null" json:"status"`
	Detail      string     `gorm:"type:text" json:"detail"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func IsReleaseStatusValid(status string) bool {
	switch status {
	case ReleaseStatusDraft, ReleaseStatusValidating, ReleaseStatusApplying, ReleaseStatusWaitingReady,
		ReleaseStatusVerifying, ReleaseStatusSucceeded, ReleaseStatusFailed, ReleaseStatusRollingBack, ReleaseStatusRolledBack:
		return true
	default:
		return false
	}
}
