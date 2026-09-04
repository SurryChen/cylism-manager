package registry

import (
	"strings"
	"testing"
)

func TestNodeRegistryMirrorApplyCommandSchedulesRestartOutsideSSHSession(t *testing.T) {
	command := nodeRegistryMirrorApplyCommand("encoded-config")
	for _, part := range []string{"registries.yaml", "encoded-config", "systemctl restart k3s"} {
		if !strings.Contains(command, part) {
			t.Fatalf("command missing %q: %s", part, command)
		}
	}
}
