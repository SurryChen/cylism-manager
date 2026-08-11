package agentartifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidatesManifestAndBinary(t *testing.T) {
	directory := t.TempDir()
	binary := []byte("static-cli-binary")
	if err := os.WriteFile(filepath.Join(directory, BinaryFileName), binary, 0o555); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	digest := sha256.Sum256(binary)
	manifest := Manifest{Version: "2026.08.10", Platform: "linux-amd64", Size: int64(len(binary)), SHA256: hex.EncodeToString(digest[:])}
	writeManifest(t, directory, manifest)

	artifact, err := Load(directory, "linux-amd64")
	if err != nil {
		t.Fatalf("load artifact: %v", err)
	}
	if artifact.BinaryPath != filepath.Join(directory, BinaryFileName) || artifact.Manifest != manifest {
		t.Fatalf("unexpected artifact: %#v", artifact)
	}
}

func TestLoadRejectsMismatchedPlatformOrChecksum(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, BinaryFileName), []byte("binary"), 0o555); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	writeManifest(t, directory, Manifest{Version: "v1", Platform: "linux-amd64", Size: 6, SHA256: "bad"})

	if _, err := Load(directory, "linux-arm64"); err == nil {
		t.Fatal("expected platform mismatch to be rejected")
	}
	if _, err := Load(directory, "linux-amd64"); err == nil {
		t.Fatal("expected checksum mismatch to be rejected")
	}
}

func writeManifest(t *testing.T, directory string, manifest Manifest) {
	t.Helper()
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, ManifestFileName), encoded, 0o444); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
