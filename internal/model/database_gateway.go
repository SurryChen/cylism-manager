package model

import "time"

// DatabaseGateway owns one private TCP entry for an external database.
// The target and credentials remain in DatabaseInstance; no password is sent to the gateway.
type DatabaseGateway struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	ServiceHost    string     `gorm:"-" json:"service_host"`
	InstanceID     uint       `gorm:"uniqueIndex;not null" json:"instance_id"`
	ServerID       uint       `gorm:"index;uniqueIndex:idx_database_gateway_listener;not null" json:"server_id"`
	ClusterID      uint       `gorm:"index;not null" json:"cluster_id"`
	ListenIP       string     `gorm:"size:45;uniqueIndex:idx_database_gateway_listener;not null" json:"listen_ip"`
	ListenPort     int        `gorm:"uniqueIndex:idx_database_gateway_listener;not null" json:"listen_port"`
	AllowedSources []string   `gorm:"type:text;serializer:json" json:"allowed_sources"`
	ProbeImage     string     `gorm:"size:255" json:"probe_image"`
	Status         string     `gorm:"size:32;not null" json:"status"`
	LastError      string     `gorm:"type:text" json:"last_error"`
	LastVerifiedAt *time.Time `json:"last_verified_at"`
	LastCheckedAt  *time.Time `json:"last_checked_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type DatabaseGatewayTask struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	GatewayID uint      `gorm:"index;uniqueIndex:idx_database_gateway_active,where:status = 'queued' OR status = 'running';not null" json:"gateway_id"`
	Kind      string    `gorm:"size:16;not null" json:"kind"`
	Status    string    `gorm:"size:16;not null" json:"status"`
	Progress  string    `gorm:"type:text" json:"progress"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DatabaseGatewayTaskLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TaskID    uint      `gorm:"index;not null" json:"task_id"`
	Message   string    `gorm:"type:text;not null" json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
