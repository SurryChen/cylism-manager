package model

const (
	PVCMigrationStatusPending            = "pending"
	PVCMigrationStatusPreflight          = "preflight"
	PVCMigrationStatusProvisioningTarget = "provisioning_target"
	PVCMigrationStatusStoppingSource     = "stopping_source"
	PVCMigrationStatusCopying            = "copying"
	PVCMigrationStatusCutover            = "cutover"
	PVCMigrationStatusWaitingReady       = "waiting_ready"
	PVCMigrationStatusSucceeded          = "succeeded"
	PVCMigrationStatusFailed             = "failed"
	PVCMigrationStatusRollingBack        = "rolling_back"
	PVCMigrationStatusRolledBack         = "rolled_back"
	PVCMigrationStatusCleanupPending     = "cleanup_pending"
	PVCMigrationStatusCleaned            = "cleaned"

	PVCImportStatusPending           = "pending"
	PVCImportStatusPreflight         = "preflight"
	PVCImportStatusStoppingWorkload  = "stopping_workload"
	PVCImportStatusBackingUp         = "backing_up"
	PVCImportStatusCopying           = "copying"
	PVCImportStatusVerifying         = "verifying"
	PVCImportStatusRestoringWorkload = "restoring_workload"
	PVCImportStatusSucceeded         = "succeeded"
	PVCImportStatusFailed            = "failed"
)

func IsPVCMigrationTerminal(status string) bool {
	switch status {
	case PVCMigrationStatusSucceeded, PVCMigrationStatusFailed, PVCMigrationStatusRolledBack, PVCMigrationStatusCleaned:
		return true
	default:
		return false
	}
}

func IsPVCImportTerminal(status string) bool {
	return status == PVCImportStatusSucceeded || status == PVCImportStatusFailed
}
