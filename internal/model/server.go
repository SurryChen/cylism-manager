package model

import (
	"gorm.io/gorm"
	"time"
)

// Server 服务器模型。
type Server struct {
	ID               uint           `gorm:"primaryKey" json:"id"`
	Name             string         `gorm:"size:128;not null" json:"name"`
	Host             string         `gorm:"size:256;uniqueIndex;not null" json:"host"`
	SSHPort          int            `gorm:"default:22" json:"ssh_port"`
	SSHUser          string         `gorm:"size:128" json:"ssh_user"`
	SSHAuthType      string         `gorm:"size:32" json:"ssh_auth_type"`
	SSHPassword      string         `gorm:"type:text" json:"-"`
	SSHKey           string         `gorm:"type:text" json:"-"`
	SSHKeyPassphrase string         `gorm:"type:text" json:"-"`
	SSHKeyHash       string         `gorm:"size:64" json:"-"`
	ClusterRole      string         `gorm:"size:32" json:"cluster_role"`
	K8sNodeName      string         `gorm:"size:256" json:"k8s_node_name"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}
