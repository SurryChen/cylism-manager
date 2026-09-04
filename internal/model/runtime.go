package model

import "time"

// RuntimeInstance is a managed runtime endpoint deployed in Kubernetes.
type RuntimeInstance struct {
	ID                     uint       `gorm:"primaryKey" json:"id"`
	Name                   string     `gorm:"size:128;uniqueIndex;not null" json:"name"`
	RuntimeType            string     `gorm:"size:32;not null" json:"runtime_type"`
	DeploymentMode         string     `gorm:"size:16;not null;default:managed" json:"deployment_mode"`
	RuntimeVersion         string     `gorm:"size:64" json:"runtime_version,omitempty"`
	Image                  string     `gorm:"size:512;not null" json:"image"`
	Namespace              string     `gorm:"size:128;not null;index" json:"namespace"`
	Port                   int32      `gorm:"not null;default:8080" json:"port"`
	HealthPath             string     `gorm:"size:256;not null;default:/health" json:"health_path"`
	PVCName                string     `gorm:"size:253;not null" json:"pvc_name"`
	Storage                string     `gorm:"size:32;not null;default:10Gi" json:"storage"`
	StorageClassName       string     `gorm:"size:128" json:"storage_class_name,omitempty"`
	NodeName               string     `gorm:"size:256" json:"node_name,omitempty"`
	EndpointURL            string     `gorm:"size:512" json:"endpoint_url,omitempty"`
	ModelName              string     `gorm:"size:128" json:"model_name,omitempty"`
	ModelBaseURL           string     `gorm:"size:512" json:"model_base_url,omitempty"`
	APIStyle               string     `gorm:"size:32;default:responses" json:"api_style"`
	EncryptedAPIKey        string     `gorm:"type:text" json:"-"`
	EncryptedRuntimeAPIKey string     `gorm:"type:text" json:"-"`
	APIKeyConfigured       bool       `gorm:"-" json:"api_key_configured"`
	Config                 string     `gorm:"type:text" json:"config,omitempty"`
	SecretName             string     `gorm:"size:253" json:"secret_name,omitempty"`
	Status                 string     `gorm:"size:32;index;not null" json:"status"`
	DesiredGeneration      uint       `gorm:"not null;default:1" json:"desired_generation"`
	ObservedGeneration     uint       `gorm:"not null;default:0" json:"observed_generation"`
	HealthStatus           string     `gorm:"size:32" json:"health_status,omitempty"`
	HealthDetail           string     `gorm:"size:512" json:"health_detail,omitempty"`
	LastHealthAt           *time.Time `json:"last_health_at,omitempty"`
	AgentToolEnabled       bool       `gorm:"not null;default:false" json:"agent_tool_enabled"`
	CreatedBy              uint       `gorm:"index;not null" json:"created_by"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}
