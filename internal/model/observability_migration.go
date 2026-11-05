package model

import "time"

// ObservabilityMigration keeps recovery checkpoints separately from the shared
// cluster task and its append-only execution logs. Source volumes are retained.
type ObservabilityMigration struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	ClusterID   uint      `gorm:"index;not null" json:"cluster_id"`
	TaskID      uint      `gorm:"uniqueIndex;not null" json:"task_id"`
	Component   string    `gorm:"size:32;not null" json:"component"`
	SourceNode  string    `json:"source_node_name"`
	TargetNode  string    `json:"target_node_name"`
	SourcePVC   string    `json:"source_pvc_name"`
	TargetPVC   string    `json:"target_pvc_name"`
	Snapshot    string    `gorm:"type:text" json:"-"`
	Status      string    `gorm:"index;size:32;not null" json:"status"`
	Detail      string    `gorm:"type:text" json:"detail"`
	BytesCopied int64     `json:"bytes_copied"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
