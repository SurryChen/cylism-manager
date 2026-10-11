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

// CrossClusterService publishes one target Service to a source cluster over Tailscale.
type CrossClusterService struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	SourceClusterID   uint      `gorm:"index;uniqueIndex:idx_cross_service_name;not null" json:"source_cluster_id"`
	TargetClusterID   uint      `gorm:"index;not null" json:"target_cluster_id"`
	SourceNamespace   string    `gorm:"size:63;uniqueIndex:idx_cross_service_name;not null" json:"source_namespace"`
	SourceName        string    `gorm:"size:63;uniqueIndex:idx_cross_service_name;not null" json:"source_name"`
	TargetNamespace   string    `gorm:"size:63;not null" json:"target_namespace"`
	TargetName        string    `gorm:"size:63;not null" json:"target_name"`
	TargetServicePort int32     `gorm:"not null" json:"target_service_port"`
	SourcePort        int32     `gorm:"not null" json:"source_port"`
	GatewayPort       int32     `gorm:"not null" json:"gateway_port"`
	TargetServerID    uint      `gorm:"not null" json:"target_server_id"`
	TailscaleIP       string    `gorm:"size:64;not null" json:"tailscale_ip"`
	Status            string    `gorm:"size:16;not null" json:"status"`
	LastError         string    `gorm:"type:text" json:"last_error,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
