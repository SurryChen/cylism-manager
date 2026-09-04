package bootstrap

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	infrastructureapi "github.com/cylism/cylism-manager/internal/api/infrastructure"
	"github.com/cylism/cylism-manager/internal/model"
)

// applyK3sRegistriesToNode is a composition-root adapter. The delivery API
// only receives the registry service's NodeMirrorApplier function and does not
// depend on infrastructure HTTP helpers.
func applyK3sRegistriesToNode(ctx context.Context, server *model.Server, encKey, content []byte) (string, string) {
	payload := base64.StdEncoding.EncodeToString(content)
	out, err := infrastructureapi.SSHExecContext(ctx, 90*time.Second, append(infrastructureapi.BuildSSHArgs(server, encKey, server.Host), nodeRegistryMirrorApplyCommand(payload)))
	if err != nil {
		return "failed", err.Error()
	}
	return "success", strings.TrimSpace(string(out))
}

func nodeRegistryMirrorApplyCommand(payload string) string {
	return fmt.Sprintf("sudo mkdir -p /etc/rancher/k3s && echo %q | base64 -d | sudo tee /etc/rancher/k3s/registries.yaml >/dev/null && (sudo systemctl restart k3s || sudo systemctl restart k3s-agent)", payload)
}
