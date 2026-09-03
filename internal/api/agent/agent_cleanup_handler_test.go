package agent

import (
	"fmt"
	"strings"
	"testing"
)

func TestMaintenanceCleanupFailureSummaryKeepsRedactedExecutorOutput(t *testing.T) {
	detail := maintenanceCleanupFailureSummary("load config file: token=secret-value permission denied", fmt.Errorf("exit status 1"))
	if !strings.Contains(detail, "load config file") || !strings.Contains(detail, "permission denied") || strings.Contains(detail, "secret-value") || !strings.Contains(detail, "[REDACTED]") {
		t.Fatalf("unexpected cleanup failure detail: %q", detail)
	}
}
