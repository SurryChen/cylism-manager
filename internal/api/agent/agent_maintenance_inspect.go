package agent

import (
	"fmt"
	"math"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	infrastructureapi "github.com/cylism/cylism-manager/internal/api/infrastructure"
	"github.com/cylism/cylism-manager/internal/model"
)

const agentDiskInspectionTimeout = 90 * time.Second

var agentDiskInspectionPaths = []string{
	"/var/log",
	"/var/lib/rancher/k3s",
	"/var/lib/kubelet",
	"/var/lib/containerd",
	"/var/lib/docker",
}

var agentJournalUsagePattern = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*([KMGTPE]?)(?:i?B)?`)

type AgentMaintenanceInspector func(*model.Server) (agentDiskInspection, error)
type agentMaintenanceInspector = AgentMaintenanceInspector

type agentDiskInspection struct {
	Node           string                    `json:"node"`
	Filesystems    []agentFilesystemUsage    `json:"filesystems"`
	JournalBytes   int64                     `json:"journal_bytes,omitempty"`
	JournalSummary string                    `json:"journal_summary,omitempty"`
	Directories    []agentDiskDirectoryUsage `json:"directories"`
}

type agentFilesystemUsage struct {
	MountPoint     string `json:"mount_point"`
	SizeBytes      int64  `json:"size_bytes"`
	UsedBytes      int64  `json:"used_bytes"`
	AvailableBytes int64  `json:"available_bytes"`
	UsedPercent    int    `json:"used_percent"`
}

type agentDiskDirectoryUsage struct {
	Path    string                `json:"path"`
	Entries []agentDiskUsageEntry `json:"entries"`
}

type agentDiskUsageEntry struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

func DefaultAgentMaintenanceInspector(encKey []byte) AgentMaintenanceInspector {
	return func(server *model.Server) (agentDiskInspection, error) {
		if server == nil {
			return agentDiskInspection{}, fmt.Errorf("managed node unavailable")
		}
		args := infrastructureapi.BuildSSHArgs(server, encKey, server.Host)
		args = append(args, agentDiskInspectionCommand())
		output, err := infrastructureapi.SSHExec(agentDiskInspectionTimeout, args)
		if err != nil {
			return agentDiskInspection{}, fmt.Errorf("node disk inspection command failed: %w: %s", err, truncateAgentText(strings.TrimSpace(string(output)), 512))
		}
		inspection, err := parseAgentDiskInspection(string(output))
		if err != nil {
			return agentDiskInspection{}, fmt.Errorf("node disk inspection returned invalid data: %w", err)
		}
		return inspection, nil
	}
}

// agentDiskInspectionCommand contains no Runtime-controlled interpolation.
// It limits each allowed directory probe and its output before SSH returns it.
func agentDiskInspectionCommand() string {
	return `set -eu
export LC_ALL=C
printf '__CYLISM_FILESYSTEMS__\n'
df -B1 -x tmpfs -x devtmpfs -x overlay --output=target,size,used,avail,pcent 2>/dev/null | tail -n +2
printf '__CYLISM_JOURNAL__\n'
sudo -n journalctl --disk-usage 2>/dev/null || true
for path in /var/log /var/lib/rancher/k3s /var/lib/kubelet /var/lib/containerd /var/lib/docker; do
  [ -d "$path" ] || continue
  printf '__CYLISM_DIR__:%s\n' "$path"
  if command -v timeout >/dev/null 2>&1; then
    timeout 12s sudo -n du -x -B1 -d 1 "$path" 2>/dev/null | sort -rn | head -n 20 || true
  else
    sudo -n du -x -B1 -d 1 "$path" 2>/dev/null | sort -rn | head -n 20 || true
  fi
done`
}

func parseAgentDiskInspection(output string) (agentDiskInspection, error) {
	inspection := agentDiskInspection{Filesystems: []agentFilesystemUsage{}, Directories: []agentDiskDirectoryUsage{}}
	mode := ""
	directories := map[string]*agentDiskDirectoryUsage{}
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		switch {
		case line == "__CYLISM_FILESYSTEMS__":
			mode = "filesystems"
			continue
		case line == "__CYLISM_JOURNAL__":
			mode = "journal"
			continue
		case strings.HasPrefix(line, "__CYLISM_DIR__:"):
			pathName := strings.TrimPrefix(line, "__CYLISM_DIR__:")
			if !agentDiskInspectionPathAllowed(pathName) {
				mode = ""
				continue
			}
			directory := &agentDiskDirectoryUsage{Path: pathName, Entries: []agentDiskUsageEntry{}}
			directories[pathName] = directory
			inspection.Directories = append(inspection.Directories, *directory)
			mode = "directory:" + pathName
			continue
		}
		if line == "" {
			continue
		}
		switch {
		case mode == "filesystems":
			if usage, ok := parseAgentFilesystemUsage(line); ok {
				inspection.Filesystems = append(inspection.Filesystems, usage)
			}
		case mode == "journal":
			if inspection.JournalSummary == "" {
				inspection.JournalSummary = truncateAgentText(redactAgentText(line), 256)
				inspection.JournalBytes = parseAgentJournalBytes(line)
			}
		case strings.HasPrefix(mode, "directory:"):
			pathName := strings.TrimPrefix(mode, "directory:")
			directory := directories[pathName]
			if directory == nil || len(directory.Entries) >= 20 {
				continue
			}
			if entry, ok := parseAgentDiskUsageEntry(line, pathName); ok && entry.Path != pathName {
				directory.Entries = append(directory.Entries, entry)
				for index := range inspection.Directories {
					if inspection.Directories[index].Path == pathName {
						inspection.Directories[index] = *directory
						break
					}
				}
			}
		}
	}
	if len(inspection.Filesystems) == 0 {
		return agentDiskInspection{}, fmt.Errorf("filesystem section missing")
	}
	return inspection, nil
}

func parseAgentFilesystemUsage(line string) (agentFilesystemUsage, bool) {
	fields := strings.Fields(line)
	if len(fields) != 5 {
		return agentFilesystemUsage{}, false
	}
	size, sizeErr := strconv.ParseInt(fields[1], 10, 64)
	used, usedErr := strconv.ParseInt(fields[2], 10, 64)
	available, availableErr := strconv.ParseInt(fields[3], 10, 64)
	percentText := strings.TrimSuffix(fields[4], "%")
	percent, percentErr := strconv.Atoi(percentText)
	if sizeErr != nil || usedErr != nil || availableErr != nil || percentErr != nil || size < 0 || used < 0 || available < 0 || percent < 0 || percent > 100 || !strings.HasPrefix(fields[0], "/") {
		return agentFilesystemUsage{}, false
	}
	return agentFilesystemUsage{MountPoint: fields[0], SizeBytes: size, UsedBytes: used, AvailableBytes: available, UsedPercent: percent}, true
}

func parseAgentDiskUsageEntry(line, basePath string) (agentDiskUsageEntry, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return agentDiskUsageEntry{}, false
	}
	bytes, err := strconv.ParseInt(fields[0], 10, 64)
	entryPath := strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
	if err != nil || bytes < 0 || entryPath == "" || (entryPath != basePath && !strings.HasPrefix(entryPath, basePath+"/")) {
		return agentDiskUsageEntry{}, false
	}
	return agentDiskUsageEntry{Path: entryPath, Bytes: bytes}, true
}

func agentDiskInspectionPathAllowed(value string) bool {
	for _, allowed := range agentDiskInspectionPaths {
		if value == allowed && path.Clean(value) == value {
			return true
		}
	}
	return false
}

func parseAgentJournalBytes(line string) int64 {
	match := agentJournalUsagePattern.FindStringSubmatch(line)
	if len(match) != 3 {
		return 0
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil || value < 0 {
		return 0
	}
	multipliers := map[string]float64{"": 1, "K": 1 << 10, "M": 1 << 20, "G": 1 << 30, "T": 1 << 40, "P": 1 << 50, "E": 1 << 60}
	multiplier, ok := multipliers[strings.ToUpper(match[2])]
	if !ok || value > math.MaxInt64/multiplier {
		return 0
	}
	return int64(math.Round(value * multiplier))
}
