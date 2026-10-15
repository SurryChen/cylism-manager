package model

import "time"

// DatabaseInstance describes either a MySQL service deployed by the platform
// or an existing MySQL endpoint. Credentials are never serialized to clients.
type DatabaseInstance struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	Name              string    `gorm:"size:128;uniqueIndex;not null" json:"name"`
	Mode              string    `gorm:"size:16;not null" json:"mode"`
	ClusterID         uint      `gorm:"index" json:"cluster_id"`
	Namespace         string    `gorm:"size:63" json:"namespace"`
	ResourceName      string    `gorm:"size:63" json:"resource_name"`
	Host              string    `gorm:"size:255;not null" json:"host"`
	Port              int       `gorm:"not null;default:3306" json:"port"`
	DatabaseName      string    `gorm:"size:64;not null" json:"database_name"`
	Username          string    `gorm:"size:64;not null" json:"username"`
	EncryptedPassword string    `gorm:"type:text;not null" json:"-"`
	EncryptedRoot     string    `gorm:"type:text" json:"-"`
	StorageSize       string    `gorm:"size:32" json:"storage_size,omitempty"`
	StorageClass      string    `gorm:"size:128" json:"storage_class,omitempty"`
	Image             string    `gorm:"size:255" json:"image,omitempty"`
	Status            string    `gorm:"size:32;not null" json:"status"`
	LastError         string    `gorm:"type:text" json:"last_error,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type DatabaseBinding struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	InstanceID    uint      `gorm:"index;not null" json:"instance_id"`
	TemplateID    uint      `gorm:"uniqueIndex;not null" json:"template_id"`
	ApplicationID uint      `gorm:"index;not null" json:"application_id"`
	Prefix        string    `gorm:"size:32;not null" json:"prefix"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// MySQLCluster is a logical InnoDB Cluster managed by MySQL Operator.
// Node selections constrain placement; the primary role can move after failover.
type MySQLCluster struct {
	ID                    uint      `gorm:"primaryKey" json:"id"`
	Name                  string    `gorm:"size:128;uniqueIndex;not null" json:"name"`
	ClusterID             uint      `gorm:"index;not null" json:"cluster_id"`
	Namespace             string    `gorm:"size:63;not null" json:"namespace"`
	ResourceName          string    `gorm:"size:63" json:"resource_name"`
	DatabaseNodes         []string  `gorm:"type:text;serializer:json" json:"database_nodes"`
	DatabaseInstances     int       `json:"database_instances"`
	RouterNodes           []string  `gorm:"type:text;serializer:json" json:"router_nodes"`
	RouterInstances       *int      `json:"router_instances"`
	PreferredPrimaryNode  string    `gorm:"size:253" json:"preferred_primary_node"`
	PrimaryConfigured     bool      `gorm:"not null;default:false" json:"primary_configured"`
	StorageSize           string    `gorm:"size:32" json:"storage_size"`
	StorageClass          string    `gorm:"size:128" json:"storage_class"`
	MyCNF                 string    `gorm:"type:text" json:"mycnf"`
	ConfigRestartRequired bool      `gorm:"not null;default:false" json:"config_restart_required"`
	EncryptedRoot         string    `gorm:"type:text" json:"-"`
	Status                string    `gorm:"size:32;not null" json:"status"`
	LastError             string    `gorm:"type:text" json:"last_error,omitempty"`
	OnlineInstances       int       `json:"online_instances"`
	RouterReady           int       `json:"router_ready"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// MySQLClusterTask records a deployment attempt independently of the API process.
type MySQLClusterTask struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	ClusterID    uint      `gorm:"index;not null" json:"cluster_id"`
	Kind         string    `gorm:"size:32;not null;default:deploy" json:"kind"`
	Status       string    `gorm:"size:16;not null" json:"status"`
	LastProgress string    `gorm:"type:text" json:"last_progress,omitempty"`
	LastWarning  string    `gorm:"type:text" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type MySQLClusterTaskLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TaskID    uint      `gorm:"index;not null" json:"task_id"`
	Message   string    `gorm:"type:text;not null" json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
