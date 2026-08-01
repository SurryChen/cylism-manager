package model

import "testing"

func TestPersistentVolumeMigrationTerminalStatus(t *testing.T) {
	for _, status := range []string{PVCMigrationStatusSucceeded, PVCMigrationStatusFailed, PVCMigrationStatusRolledBack, PVCMigrationStatusCleaned} {
		if !IsPVCMigrationTerminal(status) {
			t.Fatalf("expected %q to be terminal", status)
		}
	}
	if IsPVCMigrationTerminal(PVCMigrationStatusCopying) {
		t.Fatal("copying must remain active")
	}
}
