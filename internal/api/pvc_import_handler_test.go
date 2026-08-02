package api

import "testing"

func TestSafeHostDirectoryImportPath(t *testing.T) {
	valid := []string{"/data/karakeep", "/srv/legacy/app-data", "/home/operator/backups"}
	for _, value := range valid {
		if _, ok := safeHostDirectoryImportPath(value); !ok {
			t.Fatalf("expected %q to be accepted", value)
		}
	}
	invalid := []string{"", "relative/path", "/", "/etc/ssl", "/proc/1", "/var/lib/rancher/k3s/server"}
	for _, value := range invalid {
		if _, ok := safeHostDirectoryImportPath(value); ok {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}

func TestHostDirectoryImportBackupPathIsOutsideSource(t *testing.T) {
	backup := hostDirectoryImportBackupPath("karakeep-data", 12, false)
	if backup != "/data/cylism-import-backups/karakeep-data-import-12-source.tar.gz" {
		t.Fatalf("unexpected source backup path %q", backup)
	}
	targetBackup := hostDirectoryImportBackupPath("karakeep-data", 12, true)
	if targetBackup != "/data/cylism-import-backups/karakeep-data-import-12-target.tar.gz" {
		t.Fatalf("unexpected target backup path %q", targetBackup)
	}
}

func TestHostDirectoryImportPathsOverlap(t *testing.T) {
	if !hostDirectoryImportPathsOverlap("/data/source", "/data/source/nested") {
		t.Fatal("nested source and target must be rejected")
	}
	if hostDirectoryImportPathsOverlap("/data/source", "/data/target") {
		t.Fatal("distinct sibling directories must be allowed")
	}
}
