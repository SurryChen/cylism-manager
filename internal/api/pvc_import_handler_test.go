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

func TestHostDirectoryImportChecksumFromSSHOutput(t *testing.T) {
	checksum := "2f3f5d1a9af59a4e93a6efb1f7c82dfb77c0b3c80f08d2c9ea3294df7c6a3b41"
	output := "Warning: Permanently added '100.64.0.8' (ED25519) to the list of known hosts.\n" + checksum + "\n"
	if got := hostDirectoryImportChecksumFromSSHOutput(output); got != checksum {
		t.Fatalf("expected checksum %q, got %q", checksum, got)
	}
	if got := hostDirectoryImportChecksumFromSSHOutput("warning only"); got != "" {
		t.Fatalf("expected no checksum, got %q", got)
	}
}
