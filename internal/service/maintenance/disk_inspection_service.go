package maintenance

import (
	"context"
	"fmt"
	"math"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

const inspectionTimeout = 90 * time.Second

var inspectionPaths = []string{"/var/log", "/var/lib/rancher/k3s", "/var/lib/kubelet", "/var/lib/containerd", "/var/lib/docker"}
var journalUsagePattern = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*([KMGTPE]?)(?:i?B)?`)

type DiskInspectionService struct{ ssh SSHExecutor }

func NewDiskInspectionService(ssh SSHExecutor) *DiskInspectionService {
	return &DiskInspectionService{ssh: ssh}
}
func (s *DiskInspectionService) Inspect(ctx context.Context, server *model.Server) (Inspection, error) {
	if server == nil {
		return Inspection{}, fmt.Errorf("managed node unavailable")
	}
	if s == nil || s.ssh == nil {
		return Inspection{}, fmt.Errorf("node disk inspection unavailable")
	}
	output, err := s.ssh.Execute(ctx, inspectionTimeout, server, InspectionCommand())
	if err != nil {
		return Inspection{}, fmt.Errorf("node disk inspection command failed: %w: %s", err, cleanOutput(string(output), 512))
	}
	result, err := ParseInspection(string(output))
	if err != nil {
		return Inspection{}, fmt.Errorf("node disk inspection returned invalid data: %w", err)
	}
	return result, nil
}

func InspectionCommand() string {
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

func ParseInspection(output string) (Inspection, error) {
	result := Inspection{Filesystems: []FilesystemUsage{}, Directories: []DirectoryUsage{}}
	mode := ""
	dirs := map[string]*DirectoryUsage{}
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "__CYLISM_FILESYSTEMS__":
			mode = "filesystems"
			continue
		case line == "__CYLISM_JOURNAL__":
			mode = "journal"
			continue
		case strings.HasPrefix(line, "__CYLISM_DIR__:"):
			name := strings.TrimPrefix(line, "__CYLISM_DIR__:")
			if !InspectionPathAllowed(name) {
				mode = ""
				continue
			}
			d := &DirectoryUsage{Path: name, Entries: []UsageEntry{}}
			dirs[name] = d
			result.Directories = append(result.Directories, *d)
			mode = "directory:" + name
			continue
		}
		if line == "" {
			continue
		}
		switch {
		case mode == "filesystems":
			if v, ok := parseFilesystemUsage(line); ok {
				result.Filesystems = append(result.Filesystems, v)
			}
		case mode == "journal":
			if result.JournalSummary == "" {
				result.JournalSummary = cleanOutput(line, 256)
				result.JournalBytes = parseJournalBytes(line)
			}
		case strings.HasPrefix(mode, "directory:"):
			name := strings.TrimPrefix(mode, "directory:")
			d := dirs[name]
			if d == nil || len(d.Entries) >= 20 {
				continue
			}
			if e, ok := parseUsageEntry(line, name); ok && e.Path != name {
				d.Entries = append(d.Entries, e)
				for i := range result.Directories {
					if result.Directories[i].Path == name {
						result.Directories[i] = *d
						break
					}
				}
			}
		}
	}
	if len(result.Filesystems) == 0 {
		return Inspection{}, fmt.Errorf("filesystem section missing")
	}
	return result, nil
}

func InspectionPathAllowed(value string) bool {
	for _, allowed := range inspectionPaths {
		if value == allowed && path.Clean(value) == value {
			return true
		}
	}
	return false
}
func parseFilesystemUsage(line string) (FilesystemUsage, bool) {
	fields := strings.Fields(line)
	if len(fields) != 5 {
		return FilesystemUsage{}, false
	}
	size, e1 := strconv.ParseInt(fields[1], 10, 64)
	used, e2 := strconv.ParseInt(fields[2], 10, 64)
	available, e3 := strconv.ParseInt(fields[3], 10, 64)
	percent, e4 := strconv.Atoi(strings.TrimSuffix(fields[4], "%"))
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || size < 0 || used < 0 || available < 0 || percent < 0 || percent > 100 || !strings.HasPrefix(fields[0], "/") {
		return FilesystemUsage{}, false
	}
	return FilesystemUsage{MountPoint: fields[0], SizeBytes: size, UsedBytes: used, AvailableBytes: available, UsedPercent: percent}, true
}
func parseUsageEntry(line, base string) (UsageEntry, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return UsageEntry{}, false
	}
	bytes, err := strconv.ParseInt(fields[0], 10, 64)
	entry := strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
	if err != nil || bytes < 0 || entry == "" || (entry != base && !strings.HasPrefix(entry, base+"/")) {
		return UsageEntry{}, false
	}
	return UsageEntry{Path: entry, Bytes: bytes}, true
}
func parseJournalBytes(line string) int64 {
	match := journalUsagePattern.FindStringSubmatch(line)
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
