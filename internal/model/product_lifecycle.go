package model

import (
	"errors"
	"time"
)

var ErrProductLifecycleConflict = errors.New("记录已被更新，请刷新后重试")
var ErrProductChannelInUse = errors.New("渠道已有发行历史，不能移除；可将渠道设为暂停发行")
var ErrProductVersionName = errors.New("该产品已有同名版本")

// ProductTask is a lightweight requirement, defect or action. Versions are optional.
type ProductTask struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ProductID uint      `gorm:"index;not null" json:"product_id"`
	VersionID *uint     `gorm:"index" json:"version_id"`
	Title     string    `gorm:"size:512;not null" json:"title"`
	Kind      string    `gorm:"size:24;not null" json:"kind"`
	Status    string    `gorm:"size:24;index;not null" json:"status"`
	Priority  string    `gorm:"size:16;not null" json:"priority"`
	Details   string    `gorm:"type:text" json:"details"`
	DueDate   string    `gorm:"size:10" json:"due_date"`
	Revision  uint      `gorm:"not null;default:1" json:"revision"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProductVersion groups an iteration; it is deliberately independent of container Releases.
type ProductVersion struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	ProductID          uint      `gorm:"uniqueIndex:idx_product_version_name;not null" json:"product_id"`
	Name               string    `gorm:"size:128;uniqueIndex:idx_product_version_name;not null" json:"name"`
	Goal               string    `gorm:"size:1024" json:"goal"`
	Acceptance         string    `gorm:"type:text" json:"acceptance"`
	Status             string    `gorm:"size:24;index;not null" json:"status"`
	TargetDate         string    `gorm:"size:10" json:"target_date"`
	RepositoryURL      string    `gorm:"size:1024" json:"repository_url"`
	CommitSHA          string    `gorm:"size:128" json:"commit_sha"`
	BuildURL           string    `gorm:"size:1024" json:"build_url"`
	ArtifactURL        string    `gorm:"size:1024" json:"artifact_url"`
	Notes              string    `gorm:"type:text" json:"notes"`
	Revision           uint      `gorm:"not null;default:1" json:"revision"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	DeploymentIDs      []uint    `gorm:"-" json:"deployment_ids"`
	PlatformReleaseIDs []uint    `gorm:"-" json:"platform_release_ids"`
	TaskIDs            []uint    `gorm:"-" json:"task_ids"`
}

type ProductVersionDeployment struct {
	VersionID uint `gorm:"primaryKey" json:"version_id"`
	ReleaseID uint `gorm:"primaryKey" json:"release_id"`
}

// ProductVersionPlatformRelease keeps platform updates separate from application releases.
type ProductVersionPlatformRelease struct {
	VersionID uint `gorm:"primaryKey" json:"version_id"`
	ReleaseID uint `gorm:"primaryKey" json:"release_id"`
}

// ProductDistribution is one channel submission; retries may be recorded separately.
type ProductDistribution struct {
	Audience        string    `gorm:"size:24;index;not null;default:unclassified" json:"audience"`
	ID              uint      `gorm:"primaryKey" json:"id"`
	ProductID       uint      `gorm:"index;not null" json:"product_id"`
	VersionID       uint      `gorm:"index;not null" json:"version_id"`
	ChannelID       uint      `gorm:"index;not null" json:"channel_id"`
	PlatformVersion string    `gorm:"size:128;not null" json:"platform_version"`
	BuildIdentifier string    `gorm:"size:128" json:"build_identifier"`
	Status          string    `gorm:"size:24;index;not null" json:"status"`
	Notes           string    `gorm:"type:text" json:"notes"`
	Revision        uint      `gorm:"not null;default:1" json:"revision"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ProductDistributionEvent is append-only, including the original submission snapshot.
type ProductDistributionEvent struct {
	Audience        string    `gorm:"size:24;not null;default:unclassified" json:"audience"`
	BecamePublished bool      `gorm:"not null;default:false" json:"became_published"`
	ID              uint      `gorm:"primaryKey" json:"id"`
	DistributionID  uint      `gorm:"index;not null" json:"distribution_id"`
	ActorID         uint      `json:"actor_id"`
	Status          string    `gorm:"size:24" json:"status"`
	PlatformVersion string    `gorm:"size:128" json:"platform_version"`
	BuildIdentifier string    `gorm:"size:128" json:"build_identifier"`
	Notes           string    `gorm:"type:text" json:"notes"`
	CreatedAt       time.Time `json:"created_at"`
}

// ProductDeploymentOption never exposes desired specs, logs or credentials.
type ProductDeploymentOption struct {
	Source          string    `json:"source"`
	ID              uint      `json:"id"`
	ApplicationID   uint      `json:"application_id"`
	ApplicationName string    `json:"application_name"`
	EnvironmentKind string    `json:"environment_kind"`
	EnvironmentName string    `json:"environment_name"`
	Version         string    `json:"version"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

type ProductLifecycleFilter struct {
	AssignableTasks bool
	Audience        string
	Status          string
	Keyword         string
	VersionID       uint
	Limit           int
	Offset          int
}

type ProductWorkbench struct {
	Products      []Product             `json:"products"`
	Tasks         []ProductTask         `json:"tasks"`
	Versions      []ProductVersion      `json:"versions"`
	Distributions []ProductDistribution `json:"distributions"`
}
