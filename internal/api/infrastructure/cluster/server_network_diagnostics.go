package cluster

import (
	"context"
	"strings"
	"sync"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	"github.com/cylism/cylism-manager/internal/transport"
	"github.com/gin-gonic/gin"
)

// ServerNetworkDiagnosticsHandler owns bounded, read-only K3s VPN
// compatibility diagnostics. It never manages or invokes a VPN runtime.
type ServerNetworkDiagnosticsHandler struct {
	store                    repository.ServerRepository
	encKey                   []byte
	networkSnapshotCollector func(context.Context, *model.Server) (serverNetworkDiagnostic, error)
}

func NewServerNetworkDiagnosticsHandler(st repository.ServerRepository, encKey []byte) *ServerNetworkDiagnosticsHandler {
	h := &ServerNetworkDiagnosticsHandler{store: st, encKey: encKey}
	h.networkSnapshotCollector = h.collectNetworkSnapshot
	return h
}

type serverNetworkDiagnostic struct {
	ServerID   uint         `json:"server_id"`
	Name       string       `json:"name"`
	K8sUnit    string       `json:"k8s_unit,omitempty"`
	K3sVPN     k3sVPNStatus `json:"k3s_vpn"`
	ErrorCode  string       `json:"error_code,omitempty"`
	SampledAt  string       `json:"sampled_at"`
	DurationMS int64        `json:"duration_ms"`
}

type k3sVPNStatus struct {
	Configured bool   `json:"configured"`
	Provider   string `json:"provider,omitempty"`
}

const networkSnapshotCommand = `
unit=""
for candidate in k3s k3s-agent; do
  if systemctl is-active --quiet "$candidate" 2>/dev/null; then
    unit="$candidate"
    break
  fi
done
printf 'K8S_UNIT=%s\n' "$unit"
configured=false
provider=""
if [ -n "$unit" ]; then
  active_config="$(systemctl show "$unit" -p ExecStart -p Environment --no-pager 2>/dev/null || true)"
  file_config="$(grep -RhsE '^[[:space:]]*(vpn-auth|vpn-auth-file):' /etc/rancher/k3s/config.yaml /etc/rancher/k3s/config.yaml.d 2>/dev/null || true)"
  if printf '%s\n%s' "$active_config" "$file_config" | grep -qE -- '(-{2}vpn-auth|-{2}vpn-auth-file|K3S_VPN_AUTH|K3S_VPN_AUTH_FILE|^[[:space:]]*(vpn-auth|vpn-auth-file):)'; then
    configured=true
    if printf '%s\n%s' "$active_config" "$file_config" | grep -qi 'tailscale'; then
      provider="tailscale"
    else
      provider="unknown"
    fi
  fi
fi
printf 'K3S_VPN_CONFIGURED=%s\n' "$configured"
printf 'K3S_VPN_PROVIDER=%s\n' "$provider"
`

// NetworkDiagnostics collects independent snapshots so one unreachable server
// never hides results from other registered hosts.
func (h *ServerNetworkDiagnosticsHandler) NetworkDiagnostics(c *gin.Context) {
	servers, err := h.store.ListServers()
	if err != nil {
		apiShared.Error(c, 500, apiShared.CodeInternalError, err.Error())
		return
	}

	snapshots := make([]serverNetworkDiagnostic, len(servers))
	semaphore := make(chan struct{}, 3)
	var group sync.WaitGroup
	for index := range servers {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			started := time.Now()
			snapshot, collectErr := h.networkSnapshotCollector(c.Request.Context(), &servers[index])
			snapshot.ServerID = servers[index].ID
			snapshot.Name = servers[index].Name
			snapshot.SampledAt = started.UTC().Format(time.RFC3339)
			snapshot.DurationMS = time.Since(started).Milliseconds()
			if collectErr != nil {
				snapshot.ErrorCode = "ssh_unreachable"
			}
			snapshots[index] = snapshot
		}(index)
	}
	group.Wait()

	apiShared.Success(c, gin.H{
		"servers":    snapshots,
		"sampled_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *ServerNetworkDiagnosticsHandler) collectNetworkSnapshot(ctx context.Context, server *model.Server) (serverNetworkDiagnostic, error) {
	args := transport.BuildSSHArgs(server, h.encKey, server.Host)
	out, err := transport.SSHExecContext(ctx, 25*time.Second, append(args, networkSnapshotCommand))
	if err != nil {
		return serverNetworkDiagnostic{}, err
	}
	return parseNetworkSnapshot(string(out)), nil
}

func parseNetworkSnapshot(raw string) serverNetworkDiagnostic {
	values := taggedOutput(raw)
	snapshot := serverNetworkDiagnostic{K8sUnit: strings.TrimSpace(values["K8S_UNIT"])}
	if snapshot.K8sUnit != "" && values["K3S_VPN_CONFIGURED"] == "true" {
		snapshot.K3sVPN.Configured = true
		switch values["K3S_VPN_PROVIDER"] {
		case "tailscale", "other":
			snapshot.K3sVPN.Provider = values["K3S_VPN_PROVIDER"]
		default:
			snapshot.K3sVPN.Provider = "unknown"
		}
	}
	return snapshot
}

func taggedOutput(raw string) map[string]string {
	values := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found {
			values[key] = strings.TrimSpace(value)
		}
	}
	return values
}
