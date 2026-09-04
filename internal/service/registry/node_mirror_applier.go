package registry

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

// NodeMirrorApplierService applies rendered K3s registry configuration to a
// selected node. SSH transport is injected so this service remains independent
// of the concrete SSH implementation and of HTTP packages.
type NodeMirrorApplierService struct {
	ssh SSHExecutor
}

func NewNodeMirrorApplier(ssh SSHExecutor) *NodeMirrorApplierService {
	return &NodeMirrorApplierService{ssh: ssh}
}

// Apply writes registries.yaml on the node and restarts the appropriate K3s
// service. The payload is encoded before being embedded in the shell command.
func (s *NodeMirrorApplierService) Apply(ctx context.Context, server *model.Server, content []byte) (string, string) {
	if s == nil || s.ssh == nil {
		return "failed", "节点镜像源应用能力不可用"
	}
	if server == nil {
		return "failed", "节点不可用"
	}
	payload := base64.StdEncoding.EncodeToString(content)
	out, err := s.ssh.Execute(ctx, 90*time.Second, server, nodeRegistryMirrorApplyCommand(payload))
	if err != nil {
		return "failed", err.Error()
	}
	return "success", strings.TrimSpace(string(out))
}

func nodeRegistryMirrorApplyCommand(payload string) string {
	return fmt.Sprintf("sudo mkdir -p /etc/rancher/k3s && echo %q | base64 -d | sudo tee /etc/rancher/k3s/registries.yaml >/dev/null && (sudo systemctl restart k3s || sudo systemctl restart k3s-agent)", payload)
}
