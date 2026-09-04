package cluster

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/transport"
	"github.com/gin-gonic/gin"
)

// ServerMetricsInspector collects resource statistics over SSH.
type ServerMetricsInspector struct{ EncKey []byte }

func (i ServerMetricsInspector) ResourceStats(ctx context.Context, server *model.Server) (map[string]interface{}, error) {
	command := "echo 'CPU:' $(top -bn1 | awk '/^%Cpu|^CPU:/{print 100-$8}');echo 'CPU_CORES:' $(nproc);echo 'MEM:' $(free -m | awk '/^Mem:/{print $2,$3,$7}');echo 'DISK:' $(df -BG / | awk 'NR==2{print $2,$3,$4,$5}' | sed 's/G//g');echo 'LOAD:' $(cat /proc/loadavg | awk '{print $1,$2,$3}');echo 'UP:' $(uptime -p | sed 's/up //')"
	out, err := transport.SSHExecContext(ctx, 5*time.Second, append(transport.BuildSSHArgs(server, i.EncKey, server.Host), command))
	if err != nil {
		return nil, err
	}
	result := ParseServerStats(string(out))
	parsed := make(map[string]interface{}, len(result))
	for key, value := range result {
		parsed[key] = value
	}
	return parsed, nil
}

// ParseServerStats parses the stable key/value protocol emitted by the stats command.
func ParseServerStats(raw string) gin.H {
	result := gin.H{}
	for _, line := range strings.Split(raw, "\n") {
		parts := strings.Fields(strings.TrimSpace(line))
		if len(parts) < 2 {
			continue
		}
		switch parts[0] {
		case "CPU:":
			result["cpu_percent"], _ = strconv.ParseFloat(parts[1], 64)
		case "CPU_CORES:":
			result["cpu_cores"], _ = strconv.Atoi(parts[1])
		case "MEM:":
			if len(parts) >= 4 {
				result["memory_total_mb"], _ = strconv.Atoi(parts[1])
				result["memory_used_mb"], _ = strconv.Atoi(parts[2])
				result["memory_available_mb"], _ = strconv.Atoi(parts[3])
			}
		case "DISK:":
			if len(parts) >= 4 {
				result["disk_total_gb"], _ = strconv.Atoi(parts[1])
				result["disk_used_gb"], _ = strconv.Atoi(parts[2])
				result["disk_available_gb"], _ = strconv.Atoi(parts[3])
				if len(parts) >= 5 {
					result["disk_percent"] = parts[4]
				}
			}
		case "LOAD:":
			if len(parts) >= 4 {
				result["load_1m"], _ = strconv.ParseFloat(parts[1], 64)
				result["load_5m"], _ = strconv.ParseFloat(parts[2], 64)
				result["load_15m"], _ = strconv.ParseFloat(parts[3], 64)
			}
		case "UP:":
			result["uptime"] = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "UP:"))
		}
	}
	return result
}

// CleanHostname strips SSH warning lines and returns the first useful line.
func CleanHostname(raw string) string {
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "Warning:") || strings.Contains(line, "Permanently") {
			continue
		}
		return line
	}
	return ""
}
