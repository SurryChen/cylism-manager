package model

import "time"

// DatabaseOperation is a durable, checkpointed database capability invocation.
// Job identities are deterministic; a platform restart observes rather than
// repeats a submitted SQL phase. Secrets are encrypted separately from its spec.
type DatabaseOperation struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	Kind                string    `gorm:"size:24;index;not null" json:"kind"`
	Source              string    `gorm:"size:64;index" json:"source"`
	Target              string    `gorm:"size:64;index" json:"target"`
	ClusterID           uint      `gorm:"index" json:"cluster_id"`
	NodeName            string    `gorm:"size:253" json:"node_name"`
	Spec                string    `gorm:"type:text;not null" json:"-"`
	EncryptedInput      string    `gorm:"type:text" json:"-"`
	Status              string    `gorm:"size:24;index;not null" json:"status"`
	Phase               int       `json:"phase"`
	PhaseName           string    `gorm:"size:128" json:"phase_name"`
	Progress            string    `gorm:"type:text" json:"progress"`
	LastError           string    `gorm:"type:text" json:"last_error,omitempty"`
	Result              string    `gorm:"type:text" json:"result,omitempty"`
	PauseRequested      bool      `json:"pause_requested"`
	ConnectionsApplied  bool      `json:"-"`
	CancellationSettled bool      `json:"-"`
	CleanupPending      bool      `json:"-"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type DatabaseOperationLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TaskID    uint      `gorm:"index;not null" json:"task_id"`
	Message   string    `gorm:"type:text;not null" json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// Locks remain held while a task is paused or failed, until explicit abandon
// or successful completion, so a half-loaded target cannot be reused silently.
type DatabaseOperationLock struct {
	Resource string `gorm:"primaryKey;size:96" json:"resource"`
	TaskID   uint   `gorm:"index;not null" json:"task_id"`
}

// A relation is recorded after channel creation, including an interrupted setup.
// This is an asynchronous copy, not an automatic failover cluster or backup.
type DatabaseReplication struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	Source        string     `gorm:"size:64;index;not null" json:"source"`
	Target        string     `gorm:"size:64;uniqueIndex;not null" json:"target"`
	Channel       string     `gorm:"size:64;not null" json:"channel"`
	TaskID        uint       `gorm:"index" json:"task_id"`
	Status        string     `gorm:"size:24;not null" json:"status"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	IORunning     bool       `json:"io_running"`
	SQLRunning    bool       `json:"sql_running"`
	LagSeconds    *int64     `json:"lag_seconds"`
	ReadOnly      bool       `json:"read_only"`
	// The observation contains only allowlisted status and filter fields.
	Observation string    `gorm:"type:text" json:"observation,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
