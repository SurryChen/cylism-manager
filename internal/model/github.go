package model

import (
	"errors"
	"time"
)

var ErrGitHubNotConfigured = errors.New("请先在系统设置连接 GitHub App")
var ErrGitHubComponentExists = errors.New("该发布端已绑定同名组件")
var ErrGitHubAlreadyConnected = errors.New("当前账号已连接 GitHub App")
var ErrGitHubRegistrationExpired = errors.New("GitHub 连接请求已过期或已使用")

// GitHubConnection belongs to the Cylism account that registered the App.
type GitHubConnection struct {
	OwnerID      uint   `gorm:"primaryKey;autoIncrement:false" json:"-"`
	AppID        int64  `gorm:"not null" json:"app_id"`
	Slug         string `gorm:"not null" json:"slug"`
	EncryptedKey string `gorm:"not null;type:text" json:"-"`
}

func (GitHubConnection) TableName() string { return "github_connections" }

type GitHubRegistration struct {
	OwnerID      uint   `gorm:"primaryKey;autoIncrement:false"`
	StateHash    string `gorm:"uniqueIndex;not null"`
	BrowserHash  string `gorm:"not null"`
	Organization string
	ExpiresAt    time.Time
	Claimed      bool
}

func (GitHubRegistration) TableName() string { return "github_registrations" }

type ProductGitHubRepository struct {
	ID             uint   `gorm:"primaryKey" json:"id"`
	ProductID      uint   `gorm:"uniqueIndex:idx_product_github_component;not null" json:"-"`
	ReleaseUnitID  uint   `gorm:"uniqueIndex:idx_product_github_component;not null" json:"release_unit_id"`
	Component      string `gorm:"uniqueIndex:idx_product_github_component;not null" json:"component"`
	AppID          int64  `gorm:"not null" json:"-"`
	InstallationID int64  `gorm:"not null" json:"installation_id"`
	RepositoryID   int64  `gorm:"not null" json:"repository_id"`
	Repository     string `gorm:"not null" json:"repository"`
}

func (ProductGitHubRepository) TableName() string { return "product_github_repositories" }

// Public bindings have no app credentials or installation.
func (r ProductGitHubRepository) IsPublic() bool { return r.AppID == 0 && r.InstallationID == 0 }

// ProductGitHubSource saves the resolved commit independently of the current
// installation, repository bindings, deployment environment, and tag position.
type ProductGitHubSource struct {
	BindingID    uint   `json:"binding_id"`
	Component    string `json:"component"`
	Repository   string `json:"repository"`
	RepositoryID int64  `json:"repository_id"`
	Tag          string `json:"tag,omitempty"`
	SHA          string `json:"sha"`
}
