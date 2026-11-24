package gateway

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPythonSelectionSkipsUnsupportedDefault(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python is unavailable")
	}
	if err := exec.Command(python, "-c", "import sys; sys.exit(0 if sys.version_info >= (3, 7) else 1)").Run(); err != nil {
		t.Skip("Python 3.7+ is unavailable")
	}
	dir := t.TempDir()
	legacy := filepath.Join(dir, "python3")
	if err := os.WriteFile(legacy, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	modern := filepath.Join(dir, "python3.11's runtime")
	if err := os.Symlink(python, modern); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", "-c", "set -eu\n"+pythonSelectionCommand([]string{legacy, filepath.Join(dir, "missing"), modern})+"printf '%s' \"$gateway_python\"")
	output, err := command.CombinedOutput()
	if err != nil || string(output) != modern {
		t.Fatalf("could not select a supported independent runtime: %s (%v)", output, err)
	}
}

func TestPythonSelectionFailsBeforeDeploymentWhenUnsupported(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "python3")
	if err := os.WriteFile(legacy, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "deployment-started")
	command := exec.Command("sh", "-c", "set -eu\n"+pythonSelectionCommand([]string{legacy})+"touch "+quote(marker))
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "Python 3.7+") {
		t.Fatalf("expected an actionable version error: %s (%v)", output, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("deployment continued after version check failed: %v", err)
	}
}
