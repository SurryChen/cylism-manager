package infrastructure

import (
	"fmt"
	"math"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	"github.com/gin-gonic/gin"
)

// ServerNetworkDiagnosticsHandler owns the read-only SSH/Tailscale diagnostics
// endpoint independently from the server lifecycle handler.
type ServerNetworkDiagnosticsHandler struct {
	store                    repository.ServerRepository
	encKey                   []byte
	networkSnapshotCollector func(*model.Server) (serverNetworkDiagnostic, error)
	networkLinkCollector     func(*model.Server, string) (tailnetLinkDiagnostic, error)
}

func NewServerNetworkDiagnosticsHandler(st repository.ServerRepository, encKey []byte) *ServerNetworkDiagnosticsHandler {
	h := &ServerNetworkDiagnosticsHandler{store: st, encKey: encKey}
	h.networkSnapshotCollector = h.collectNetworkSnapshot
	h.networkLinkCollector = h.collectNetworkLink
	return h
}

// serverNetworkDiagnostic contains only the safe, operator-facing subset of a
// server's K3s and Tailscale state. Raw service configuration and Tailscale
// status output are intentionally never returned to the browser.
type serverNetworkDiagnostic struct {
	ServerID    uint            `json:"server_id"`
	Name        string          `json:"name"`
	K8sUnit     string          `json:"k8s_unit,omitempty"`
	NetworkMode string          `json:"network_mode"`
	Tailscale   tailscaleStatus `json:"tailscale"`
	ErrorCode   string          `json:"error_code,omitempty"`
	SampledAt   string          `json:"sampled_at"`
	DurationMS  int64           `json:"duration_ms"`
}

type tailscaleStatus struct {
	Installed                  bool   `json:"installed"`
	Online                     bool   `json:"online"`
	TailnetIP                  string `json:"tailnet_ip,omitempty"`
	UDP                        *bool  `json:"udp,omitempty"`
	IPv4                       *bool  `json:"ipv4,omitempty"`
	MappingVariesByDestination *bool  `json:"mapping_varies_by_dest_ip,omitempty"`
	NearestDERP                string `json:"nearest_derp,omitempty"`
}

type tailnetLinkDiagnostic struct {
	SourceServerID uint   `json:"source_server_id"`
	TargetServerID uint   `json:"target_server_id"`
	Path           string `json:"path"`
	DERPRegion     string `json:"derp_region,omitempty"`
	LatencyMS      int64  `json:"latency_ms,omitempty"`
	ErrorCode      string `json:"error_code,omitempty"`
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
embedded=false
if [ -n "$unit" ] && systemctl show "$unit" -p ExecStart -p Environment --no-pager 2>/dev/null | grep -qE -- '(-{2}vpn-auth|-{2}vpn-auth-file|K3S_VPN_AUTH|K3S_VPN_AUTH_FILE)'; then
  embedded=true
fi
if [ -n "$unit" ] && grep -RqlE '^[[:space:]]*(vpn-auth|vpn-auth-file):' /etc/rancher/k3s/config.yaml /etc/rancher/k3s/config.yaml.d 2>/dev/null; then
  embedded=true
fi
printf 'EMBEDDED_VPN=%s\n' "$embedded"
if ! command -v tailscale >/dev/null 2>&1; then
  printf 'TAILSCALE_INSTALLED=false\n'
  exit 0
fi
printf 'TAILSCALE_INSTALLED=true\n'
status="$(tailscale status --json 2>/dev/null || true)"
online=false
if printf '%s' "$status" | grep -qE '"Online"[[:space:]]*:[[:space:]]*true'; then online=true; fi
printf 'TAILSCALE_ONLINE=%s\n' "$online"
ip="$(tailscale ip -4 2>/dev/null | head -n 1 || true)"
printf 'TAILNET_IP=%s\n' "$ip"
netcheck="$(tailscale netcheck --format=json 2>/dev/null || true)"
for field in UDP IPv4 MappingVariesByDestIP; do
  value="$(printf '%s' "$netcheck" | sed -n "s/.*\\\"$field\\\"[[:space:]]*:[[:space:]]*\\\(true\\\|false\\\).*/\\1/p" | head -n 1)"
  printf '%s=%s\n' "$field" "$value"
done
derp="$(printf '%s' "$netcheck" | sed -n 's/.*"NearestDERP"[[:space:]]*:[[:space:]]*"\([A-Za-z0-9_-]*\)".*/\1/p' | head -n 1)"
printf 'NEAREST_DERP=%s\n' "$derp"
`

var (
	linkDERPPattern   = regexp.MustCompile(`(?i)(?:pong|PONG).*?via DERP\(([A-Za-z0-9_-]+)\).*?in ([0-9.]+)ms`)
	linkDirectPattern = regexp.MustCompile(`(?i)(?:pong|PONG).*?via [^[:space:]]+.*?in ([0-9.]+)ms`)
)

// NetworkDiagnostics collects a bounded, on-demand snapshot. It uses the
// existing server SSH credentials and invokes only fixed, read-only commands.
func (h *ServerNetworkDiagnosticsHandler) NetworkDiagnostics(c *gin.Context) {
	servers, err := h.store.ListServers()
	if err != nil {
		apiShared.Error(c, 500, model.CodeInternalError, err.Error())
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
			snapshot, collectErr := h.networkSnapshotCollector(&servers[index])
			snapshot.ServerID = servers[index].ID
			snapshot.Name = servers[index].Name
			snapshot.SampledAt = started.UTC().Format(time.RFC3339)
			snapshot.DurationMS = time.Since(started).Milliseconds()
			if collectErr != nil {
				snapshot.NetworkMode = "unknown"
				snapshot.ErrorCode = "ssh_unreachable"
			}
			if snapshot.NetworkMode == "" {
				snapshot.NetworkMode = "unknown"
			}
			snapshots[index] = snapshot
		}(index)
	}
	group.Wait()

	links := h.collectNetworkLinks(servers, snapshots)
	model.Success(c, gin.H{
		"servers":    snapshots,
		"links":      links,
		"sampled_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *ServerNetworkDiagnosticsHandler) collectNetworkSnapshot(server *model.Server) (serverNetworkDiagnostic, error) {
	args := buildSSHArgs(server, h.encKey, server.Host)
	out, err := sshExec(25*time.Second, append(args, networkSnapshotCommand))
	if err != nil {
		return serverNetworkDiagnostic{}, err
	}
	return parseNetworkSnapshot(string(out)), nil
}

func (h *ServerNetworkDiagnosticsHandler) collectNetworkLinks(servers []model.Server, snapshots []serverNetworkDiagnostic) []tailnetLinkDiagnostic {
	eligible := make([]int, 0, len(snapshots))
	for index := range snapshots {
		if snapshots[index].Tailscale.Online && validTailnetIPv4(snapshots[index].Tailscale.TailnetIP) {
			eligible = append(eligible, index)
		}
	}

	links := make([]tailnetLinkDiagnostic, 0, len(eligible)*(len(eligible)-1))
	for _, sourceIndex := range eligible {
		for _, targetIndex := range eligible {
			if sourceIndex == targetIndex {
				continue
			}
			links = append(links, tailnetLinkDiagnostic{
				SourceServerID: servers[sourceIndex].ID,
				TargetServerID: servers[targetIndex].ID,
				Path:           "unknown",
			})
		}
	}

	semaphore := make(chan struct{}, 3)
	var group sync.WaitGroup
	for index := range links {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			var source *model.Server
			var targetIP string
			for serverIndex := range servers {
				if servers[serverIndex].ID == links[index].SourceServerID {
					source = &servers[serverIndex]
				}
				if servers[serverIndex].ID == links[index].TargetServerID {
					targetIP = snapshots[serverIndex].Tailscale.TailnetIP
				}
			}
			if source == nil || !validTailnetIPv4(targetIP) {
				links[index].ErrorCode = "invalid_target"
				return
			}
			result, collectErr := h.networkLinkCollector(source, targetIP)
			result.SourceServerID = links[index].SourceServerID
			result.TargetServerID = links[index].TargetServerID
			if collectErr != nil && result.ErrorCode == "" {
				result.Path = "unreachable"
				result.ErrorCode = "ping_failed"
			}
			if result.Path == "" {
				result.Path = "unknown"
			}
			links[index] = result
		}(index)
	}
	group.Wait()
	return links
}

func (h *ServerNetworkDiagnosticsHandler) collectNetworkLink(source *model.Server, targetIP string) (tailnetLinkDiagnostic, error) {
	if !validTailnetIPv4(targetIP) {
		return tailnetLinkDiagnostic{Path: "unknown", ErrorCode: "invalid_target"}, fmt.Errorf("invalid tailnet target")
	}
	args := buildSSHArgs(source, h.encKey, source.Host)
	out, err := sshExec(20*time.Second, append(args, "tailscale ping --c=3 --timeout=5s "+targetIP))
	result := parseTailnetLinkDiagnostic(string(out))
	if err != nil {
		return result, err
	}
	return result, nil
}

func parseNetworkSnapshot(raw string) serverNetworkDiagnostic {
	values := taggedOutput(raw)
	snapshot := serverNetworkDiagnostic{
		K8sUnit: strings.TrimSpace(values["K8S_UNIT"]),
		Tailscale: tailscaleStatus{
			Installed: values["TAILSCALE_INSTALLED"] == "true",
			Online:    values["TAILSCALE_ONLINE"] == "true",
		},
	}
	if validTailnetIPv4(values["TAILNET_IP"]) {
		snapshot.Tailscale.TailnetIP = values["TAILNET_IP"]
	}
	snapshot.Tailscale.UDP = taggedBool(values, "UDP")
	snapshot.Tailscale.IPv4 = taggedBool(values, "IPv4")
	snapshot.Tailscale.MappingVariesByDestination = taggedBool(values, "MappingVariesByDestIP")
	if nearestDERP := values["NEAREST_DERP"]; regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`).MatchString(nearestDERP) {
		snapshot.Tailscale.NearestDERP = nearestDERP
	}

	if snapshot.K8sUnit != "" && values["EMBEDDED_VPN"] == "true" {
		snapshot.NetworkMode = "k3s_embedded_tailscale"
	} else if snapshot.Tailscale.Installed {
		snapshot.NetworkMode = "external_tailscale"
	} else {
		snapshot.NetworkMode = "standard_network"
	}
	return snapshot
}

func parseTailnetLinkDiagnostic(raw string) tailnetLinkDiagnostic {
	if match := linkDERPPattern.FindStringSubmatch(raw); len(match) == 3 {
		return tailnetLinkDiagnostic{Path: "derp", DERPRegion: match[1], LatencyMS: parseLatencyMS(match[2])}
	}
	if match := linkDirectPattern.FindStringSubmatch(raw); len(match) == 2 {
		return tailnetLinkDiagnostic{Path: "direct", LatencyMS: parseLatencyMS(match[1])}
	}
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "timed out") || strings.Contains(lower, "no reply") {
		return tailnetLinkDiagnostic{Path: "unreachable", ErrorCode: "ping_timeout"}
	}
	return tailnetLinkDiagnostic{Path: "unknown", ErrorCode: "ping_unclassified"}
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

func taggedBool(values map[string]string, key string) *bool {
	value, exists := values[key]
	if !exists || (value != "true" && value != "false") {
		return nil
	}
	parsed := value == "true"
	return &parsed
}

func validTailnetIPv4(value string) bool {
	ip := net.ParseIP(strings.TrimSpace(value))
	return ip != nil && ip.To4() != nil
}

func parseLatencyMS(value string) int64 {
	latency, err := strconv.ParseFloat(value, 64)
	if err != nil || latency < 0 {
		return 0
	}
	return int64(math.Round(latency))
}
