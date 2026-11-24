package model

import "time"

// AlertHistory stores an alert occurrence, independently of automation state.
// Repeated callbacks for the same labels and start time update the same row.
type AlertHistory struct {
	ID                     uint       `gorm:"primaryKey" json:"id"`
	ClusterID              uint       `gorm:"uniqueIndex:idx_alert_occurrence;index;not null" json:"cluster_id"`
	Fingerprint            string     `gorm:"size:128;uniqueIndex:idx_alert_occurrence;not null" json:"-"`
	StartsAt               time.Time  `gorm:"uniqueIndex:idx_alert_occurrence;index;not null" json:"starts_at"`
	EndsAt                 *time.Time `json:"ends_at,omitempty"`
	AlertName              string     `gorm:"size:128;index;not null" json:"alert_name"`
	RuleName               string     `gorm:"size:256" json:"rule_name"`
	NodeName               string     `gorm:"size:253;index" json:"node_name"`
	Severity               string     `gorm:"size:32;index" json:"severity"`
	Status                 string     `gorm:"size:16;index" json:"status"`
	Summary                string     `gorm:"type:text" json:"summary"`
	NotificationSuppressed bool       `json:"notification_suppressed"`
	Labels                 string     `gorm:"type:text;not null" json:"-"`
	Annotations            string     `gorm:"type:text;not null" json:"-"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type AlertHistoryFilter struct {
	ClusterID uint
	Since     time.Time
	NodeName  string
	AlertName string
	Status    string
	Limit     int
	Offset    int
}

type AlertFrequency struct {
	NodeName  string `json:"node_name"`
	AlertName string `json:"alert_name"`
	RuleName  string `json:"rule_name"`
	Count     int64  `json:"count"`
	LastSeen  string `json:"last_seen"`
}
