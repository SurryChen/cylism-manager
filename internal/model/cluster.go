package model

import "time"

// Cluster is a registered Kubernetes cluster, independent of its nodes.
type Cluster struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	Name            string    `gorm:"size:128;uniqueIndex;not null" json:"name"`
	Distribution    string    `gorm:"size:32;not null" json:"distribution"`
	NetworkMode     string    `gorm:"size:16;not null;default:public" json:"network_mode"`
	Status          string    `gorm:"size:32;not null;default:draft" json:"status"`
	ControlServerID *uint     `gorm:"index" json:"control_server_id,omitempty"`
	ControlAddress  string    `gorm:"size:256" json:"control_address,omitempty"`
	TailscaleIP     string    `gorm:"size:256" json:"tailscale_ip,omitempty"`
	EncryptedToken  string    `gorm:"type:text" json:"-"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ClusterDeployment records one installation attempt and survives browser reloads.
type ClusterDeployment struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	ClusterID   uint      `gorm:"index;not null" json:"cluster_id"`
	ServerID    uint      `gorm:"index;not null" json:"server_id"`
	Operation   string    `gorm:"size:16;not null;default:deploy" json:"operation"`
	Role        string    `gorm:"size:32;not null" json:"role"`
	Mirror      string    `gorm:"size:16;not null" json:"mirror"`
	NetworkMode string    `gorm:"size:16;not null;default:public" json:"network_mode"`
	Address     string    `gorm:"size:256" json:"address,omitempty"`
	Status      string    `gorm:"size:16;not null" json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ClusterDeploymentLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	DeploymentID uint      `gorm:"index;not null" json:"deployment_id"`
	Message      string    `gorm:"type:text;not null" json:"message"`
	CreatedAt    time.Time `json:"created_at"`
}
