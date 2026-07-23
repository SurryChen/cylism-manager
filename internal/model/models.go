package model

import (
	"time"

	"gorm.io/gorm"
)

// Server 服务器模型
type Server struct {
	ID               uint           `gorm:"primaryKey" json:"id"`
	Name             string         `gorm:"size:128;not null" json:"name"`
	Host             string         `gorm:"size:256;uniqueIndex;not null" json:"host"`
	Port             int            `gorm:"default:9527" json:"port"`
	SSHHost          string         `gorm:"size:256" json:"ssh_host"`
	SSHPort          int            `gorm:"default:22" json:"ssh_port"`
	SSHUser          string         `gorm:"size:128" json:"ssh_user"`
	SSHAuthType      string         `gorm:"size:32" json:"ssh_auth_type"` // password / key
	SSHPassword      string         `gorm:"type:text" json:"-"`           // 加密存储，JSON 序列化时隐藏
	SSHKey           string         `gorm:"type:text" json:"-"`           // 加密存储
	SSHKeyPassphrase string         `gorm:"type:text" json:"-"`           // 加密存储
	Status           string         `gorm:"size:32;default:offline" json:"status"` // online / offline / deploying
	LastSeen         *time.Time     `json:"last_seen"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
	Sites            []Site         `gorm:"foreignKey:ServerID" json:"sites,omitempty"`
}

// Site 站点模型
type Site struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	ServerID       uint           `gorm:"index;not null" json:"server_id"`
	Domain         string         `gorm:"size:256;not null" json:"domain"`
	Port           int            `gorm:"default:80" json:"port"`
	SSLEnabled     bool           `gorm:"default:false" json:"ssl_enabled"`
	RootPath       string         `gorm:"size:512" json:"root_path"`
	Managed        bool           `gorm:"default:true" json:"managed"`
	NginxConfPath  string         `gorm:"size:512" json:"nginx_conf_path"`
	Upstream       string         `gorm:"type:text" json:"upstream"`   // JSON
	Locations      string         `gorm:"type:text" json:"locations"`  // JSON
	CertID         *uint          `json:"cert_id"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
	Server         Server         `gorm:"foreignKey:ServerID" json:"-"`
	Cert           *Cert          `gorm:"foreignKey:CertID" json:"cert,omitempty"`
}

// Cert 证书模型
type Cert struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	SiteID         uint      `gorm:"index;not null" json:"site_id"`
	Domains        string    `gorm:"type:text" json:"domains"` // JSON array
	Provider       string    `gorm:"size:32;default:acme.sh" json:"provider"`
	Account        string    `gorm:"size:256" json:"account"`
	CertPath       string    `gorm:"size:512" json:"cert_path"`
	KeyPath        string    `gorm:"size:512" json:"key_path"`
	FullchainPath  string    `gorm:"size:512" json:"fullchain_path"`
	ValidFrom      time.Time `json:"valid_from"`
	ValidTo        time.Time `json:"valid_to"`
	Status         string    `gorm:"size:32" json:"status"` // issued / renewing / expired / revoked
	Challenge      string    `gorm:"size:16" json:"challenge"` // http / dns
	LastRenew      *time.Time `json:"last_renew"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// AuditLog 审计日志模型
type AuditLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Action       string    `gorm:"size:64;index;not null" json:"action"`
	ResourceType string    `gorm:"size:64;index;not null" json:"resource_type"`
	ResourceID   uint      `gorm:"index" json:"resource_id"`
	UserID       uint           `gorm:"index"`
	Detail       string    `gorm:"type:text" json:"detail"` // JSON
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

// UpstreamConfig 上游代理配置
type UpstreamConfig struct {
	Servers []UpstreamServer `json:"servers"`
}

// UpstreamServer 上游服务器
type UpstreamServer struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Weight int    `json:"weight,omitempty"`
}

// LocationConfig 额外的 location 规则
type LocationConfig struct {
	Path       string `json:"path"`
	ProxyPass  string `json:"proxy_pass,omitempty"`
	Root       string `json:"root,omitempty"`
	Extra      string `json:"extra,omitempty"`
}

// User 用户模型
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"size:128;uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"size:256;not null" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
