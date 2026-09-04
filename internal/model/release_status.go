package model

const (
	ReleaseStatusDraft        = "draft"
	ReleaseStatusValidating   = "validating"
	ReleaseStatusApplying     = "applying"
	ReleaseStatusWaitingReady = "waiting_ready"
	ReleaseStatusVerifying    = "verifying"
	ReleaseStatusSucceeded    = "succeeded"
	ReleaseStatusFailed       = "failed"
	ReleaseStatusRollingBack  = "rolling_back"
	ReleaseStatusRolledBack   = "rolled_back"

	ReleaseOperationRunning = "running"
	ReleaseOperationSuccess = "success"
	ReleaseOperationFailed  = "failed"
)
