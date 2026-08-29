package api

import (
	"strings"
	"testing"
)

func TestNodeRegistryMirrorApplyCommandSchedulesRestartOutsideSSHSession(t *testing.T) {
	command := nodeRegistryMirrorApplyCommand("encoded-config")
	if !strings.Contains(command, "printf %s encoded-config | base64 -d") {
		t.Fatalf("expected encoded configuration write: %s", command)
	}
	if !strings.Contains(command, "nohup sudo -n sh -c \"sleep 2; systemctl restart $service\"") {
		t.Fatalf("expected deferred restart: %s", command)
	}
	if strings.Contains(command, "then sudo -n systemctl restart k3s") {
		t.Fatalf("restart must not block the SSH session: %s", command)
	}
}
