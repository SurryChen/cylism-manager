package model

import (
	"errors"
	"time"
)

var ErrProductResourceAccess = errors.New("关联资源不存在或无权访问，请重新选择")

const (
	ProductStageIdea       = "idea"
	ProductStageDeveloping = "developing"
	ProductStageOperating  = "operating"
	ProductStagePaused     = "paused"
	ProductStageArchived   = "archived"
)

const (
	ProductChannelWeChatMiniProgram = "wechat_mini_program"
	ProductChannelDouyinMiniProgram = "douyin_mini_program"
	ProductChannelMiniGame          = "mini_game"
	ProductChannelGooglePlay        = "google_play"
	ProductChannelAppStore          = "app_store"
	ProductChannelWeb               = "web"
	ProductChannelOther             = "other"
)

// Product is a user-owned product record, independent from deployable applications.
type Product struct {
	ID                  uint                      `gorm:"primaryKey" json:"id"`
	OwnerID             uint                      `gorm:"index;not null" json:"owner_id"`
	Name                string                    `gorm:"size:128;not null" json:"name"`
	Summary             string                    `gorm:"size:1024" json:"summary"`
	IconURL             string                    `gorm:"size:1024" json:"icon_url"`
	Identifier          string                    `gorm:"size:32" json:"identifier"`
	Stage               string                    `gorm:"size:24;index;not null;default:idea" json:"stage"`
	Links               []ProductLink             `gorm:"type:text;serializer:json" json:"links"`
	NextAction          string                    `gorm:"size:512" json:"next_action"`
	NextDueDate         *time.Time                `json:"next_due_date,omitempty"`
	PublicBio           string                    `gorm:"size:1024" json:"public_bio"`
	PublicURL           string                    `gorm:"size:1024" json:"public_url"`
	PublicShowcase      bool                      `gorm:"not null;default:false" json:"public_showcase"`
	CreatedAt           time.Time                 `json:"created_at"`
	UpdatedAt           time.Time                 `json:"updated_at"`
	Channels            []ProductChannel          `gorm:"foreignKey:ProductID" json:"channels,omitempty"`
	Projects            []Project                 `gorm:"-" json:"projects"`
	Applications        []Application             `gorm:"-" json:"applications"`
	RelatedApplications []Application             `gorm:"-" json:"related_applications"`
	DatabaseResources   []ProductDatabaseResource `gorm:"-" json:"database_resources"`
}

// ProductDatabaseResource exposes binding metadata without connection credentials.
type ProductDatabaseResource struct {
	InstanceID    uint   `json:"instance_id"`
	Name          string `json:"name"`
	ApplicationID uint   `json:"application_id"`
}

type ProductLink struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	URL   string `json:"url"`
}

type ProductChannel struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	ProductID      uint      `gorm:"index;not null" json:"product_id"`
	Platform       string    `gorm:"size:32;not null" json:"platform"`
	Name           string    `gorm:"size:128" json:"name"`
	AppIdentifier  string    `gorm:"size:256" json:"app_identifier"`
	AdminURL       string    `gorm:"size:1024" json:"admin_url"`
	PublicURL      string    `gorm:"size:1024" json:"public_url"`
	OnlineVersion  string    `gorm:"size:128" json:"online_version"`
	PendingVersion string    `gorm:"size:128" json:"pending_version"`
	PublishStatus  string    `gorm:"size:32" json:"publish_status"`
	Notes          string    `gorm:"size:1024" json:"notes"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ProductProject struct {
	ProductID uint `gorm:"primaryKey" json:"product_id"`
	ProjectID uint `gorm:"primaryKey" json:"project_id"`
}

type ProductApplication struct {
	ProductID     uint `gorm:"primaryKey" json:"product_id"`
	ApplicationID uint `gorm:"primaryKey" json:"application_id"`
}
