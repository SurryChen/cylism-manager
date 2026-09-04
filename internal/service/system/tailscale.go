package system

import (
	"context"
	"os"
	"os/exec"
	"time"
)

// Runtime is the host boundary used by Tailscale operations.
type Runtime interface {
	Installed(context.Context) bool
	Run(context.Context, string, ...string) ([]byte, error)
	Install(context.Context) ([]byte, error)
	ReadFile(context.Context, string) ([]byte, error)
}

// HostRuntime executes Tailscale commands on the manager host.
type HostRuntime struct{}

func (HostRuntime) Installed(context.Context) bool {
	_, err := exec.LookPath("tailscale")
	return err == nil
}

func (HostRuntime) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return runCommand(ctx, 60*time.Second, name, args...)
}

func (HostRuntime) Install(ctx context.Context) ([]byte, error) {
	return runCommand(ctx, 120*time.Second, "sh", "-c", "curl -fsSL https://tailscale.com/install.sh | sh")
}

func (HostRuntime) ReadFile(ctx context.Context, path string) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return os.ReadFile(path)
}

func runCommand(parent context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
