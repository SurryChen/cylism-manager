package model

import "time"

// AlertSuppressionRule controls platform notification delivery, independently of
// Alertmanager silences and the lifecycle of the underlying alert.
type AlertSuppressionRule struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	ClusterID  uint       `gorm:"index;not null" json:"cluster_id"`
	Name       string     `gorm:"size:100;not null" json:"name"`
	NodeName   string     `gorm:"size:253;index" json:"node_name"`
	AlertNames []string   `gorm:"serializer:json;type:text;not null" json:"alert_names"`
	Comment    string     `gorm:"size:300" json:"comment"`
	Enabled    bool       `gorm:"index;not null" json:"enabled"`
	StartsAt   time.Time  `json:"starts_at"`
	EndsAt     *time.Time `gorm:"index" json:"ends_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type AlertSuppressionFilter struct {
	ClusterID uint
	NodeName  string
	Status    string
	Now       time.Time
	Limit     int
	Offset    int
}
