// Package agentartifact loads the Manager-owned CLI binary shipped in the image.
package agentartifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

const (
	BinaryFileName   = "cylism-cli"
	ManifestFileName = "cylism-cli.manifest.json"
)

var (
	platformPattern = regexp.MustCompile(`^linux-(amd64|arm64)$`)
	versionPattern  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
	digestPattern   = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type Manifest struct {
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
}

type Artifact struct {
	Manifest   Manifest
	BinaryPath string
}

// Load only accepts the binary and manifest at the Manager image's fixed path.
func Load(directory, platform string) (Artifact, error) {
	if !platformPattern.MatchString(platform) {
		return Artifact{}, errors.New("unsupported platform")
	}
	manifestPath := filepath.Join(directory, ManifestFileName)
	encoded, err := os.ReadFile(manifestPath)
	if err != nil {
		return Artifact{}, fmt.Errorf("read CLI manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		return Artifact{}, fmt.Errorf("decode CLI manifest: %w", err)
	}
	if !versionPattern.MatchString(manifest.Version) || manifest.Platform != platform || manifest.Size <= 0 || !digestPattern.MatchString(manifest.SHA256) {
		return Artifact{}, errors.New("invalid CLI manifest")
	}
	binaryPath := filepath.Join(directory, BinaryFileName)
	file, err := os.Open(binaryPath)
	if err != nil {
		return Artifact{}, fmt.Errorf("open CLI binary: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != manifest.Size {
		return Artifact{}, errors.New("CLI binary size mismatch")
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return Artifact{}, fmt.Errorf("hash CLI binary: %w", err)
	}
	if hex.EncodeToString(digest.Sum(nil)) != manifest.SHA256 {
		return Artifact{}, errors.New("CLI binary checksum mismatch")
	}
	return Artifact{Manifest: manifest, BinaryPath: binaryPath}, nil
}
