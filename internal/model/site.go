package model

import (
	"gorm.io/gorm"
	"time"
)

// Site 站点模型。
type Site struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	ServerID      uint           `gorm:"index;not null" json:"server_id"`
	Domain        string         `gorm:"size:256;not null" json:"domain"`
	Port          int            `gorm:"default:80" json:"port"`
	SSLEnabled    bool           `gorm:"default:false" json:"ssl_enabled"`
	RootPath      string         `gorm:"size:512" json:"root_path"`
	Managed       bool           `gorm:"default:true" json:"managed"`
	NginxConfPath string         `gorm:"size:512" json:"nginx_conf_path"`
	Upstream      string         `gorm:"type:text" json:"upstream"`
	Locations     string         `gorm:"type:text" json:"locations"`
	CertID        *uint          `json:"cert_id"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
	Server        Server         `gorm:"foreignKey:ServerID" json:"-"`
	Cert          *Cert          `gorm:"foreignKey:CertID" json:"cert,omitempty"`
}
