package transport

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
)

// SSHTimeout is the default bound for a single remote SSH command.
const SSHTimeout = 15 * time.Second

// SSHExecContext executes a remote command with the caller's cancellation
// boundary and a per-command timeout.
func SSHExecContext(parent context.Context, timeout time.Duration, args []string) ([]byte, error) {
	if parent == nil {
		return nil, fmt.Errorf("ssh context is required")
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	log.Printf("[ssh] running remote command for host %s", sshTarget(args))
	return exec.CommandContext(ctx, "ssh", args...).CombinedOutput()
}

// BuildSSHArgs constructs non-interactive SSH arguments.
func BuildSSHArgs(server *model.Server, encKey []byte, host string) []string {
	port := 22
	if server != nil && server.SSHPort > 0 {
		port = server.SSHPort
	}
	args := []string{"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-o", "GlobalKnownHostsFile=/dev/null", "-o", "ConnectTimeout=5", "-o", "BatchMode=yes", "-p", strconv.Itoa(port)}
	if server != nil && server.SSHAuthType == "key" && server.SSHKey != "" {
		if decKey, err := crypto.Decrypt(encKey, server.SSHKey); err == nil && decKey != "" {
			if server.SSHKeyHash != "" {
				got := md5.Sum([]byte(decKey))
				if hex.EncodeToString(got[:]) != server.SSHKeyHash {
					log.Printf("[ssh] private key hash mismatch for server=%d", server.ID)
				}
			}
			if !strings.HasSuffix(decKey, "\n") {
				decKey += "\n"
			}
			keyDir := filepath.Join("/data", "tmp")
			if err := os.MkdirAll(keyDir, 0700); err == nil {
				keyPath := filepath.Join(keyDir, fmt.Sprintf("cylism-ssh-%d", server.ID))
				if err := os.WriteFile(keyPath, []byte(decKey), 0600); err == nil {
					args = append(args, "-i", keyPath)
				} else {
					log.Printf("[ssh] private key file unavailable for server=%d: %v", server.ID, err)
				}
			}
		} else {
			log.Printf("[ssh] private key decryption failed for server=%d", server.ID)
		}
	}
	if server != nil {
		args = append(args, fmt.Sprintf("%s@%s", server.SSHUser, host))
	} else {
		args = append(args, host)
	}
	return args
}

func sshTarget(args []string) string {
	for index := len(args) - 1; index >= 0; index-- {
		if strings.Contains(args[index], "@") {
			return args[index]
		}
	}
	return "unknown"
}
