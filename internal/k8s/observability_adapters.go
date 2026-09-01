package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func VictoriaMetricsQuery(ctx context.Context, path string, values url.Values) (interface{}, error) {
	base := strings.TrimRight(VictoriaMetricsServiceURL(), "/") + path
	if len(values) > 0 {
		base += "?" + values.Encode()
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("VictoriaMetrics 返回 %s", resp.Status)
	}
	var payload struct {
		Status string      `json:"status"`
		Data   interface{} `json:"data"`
		Error  string      `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Status != "success" {
		if payload.Error == "" {
			payload.Error = "VictoriaMetrics 查询未成功"
		}
		return nil, fmt.Errorf("%s", payload.Error)
	}
	return payload.Data, nil
}

// MonitoringComponentAdapter implements the narrow monitoring lifecycle
// boundary for observability services.
type MonitoringComponentAdapter struct{ Client *Client }

type VictoriaMetricsReadiness struct{ Client *Client }

func (r VictoriaMetricsReadiness) Ready(_ context.Context) bool {
	return r.Client != nil && r.Client.VictoriaMetricsStatus() != nil && r.Client.VictoriaMetricsStatus().ReadyReplicas > 0
}

type LoggingReadiness struct{ Client *Client }

func (r LoggingReadiness) Ready(_ context.Context) bool {
	return r.Client != nil && r.Client.LoggingStatus() != nil && r.Client.LoggingStatus().LokiReady >= 1
}

func (a MonitoringComponentAdapter) VictoriaMetricsStatus(context.Context) *VictoriaMetricsStatus {
	return a.Client.VictoriaMetricsStatus()
}
func (a MonitoringComponentAdapter) InstallVictoriaMetrics(_ context.Context, c VictoriaMetricsConfig) (*VictoriaMetricsStatus, error) {
	return a.Client.InstallVictoriaMetrics(c)
}
func (a MonitoringComponentAdapter) UninstallVictoriaMetrics(_ context.Context) error {
	return a.Client.UninstallVictoriaMetrics()
}
func (a MonitoringComponentAdapter) StartVictoriaMetricsHostPathMigration(_ context.Context, c VictoriaMetricsMigrationRequest) (*VictoriaMetricsStatus, error) {
	return a.Client.StartVictoriaMetricsHostPathMigration(c)
}

type LoggingComponentAdapter struct{ Client *Client }

func (a LoggingComponentAdapter) LoggingStatus(context.Context) *LoggingStatus {
	return a.Client.LoggingStatus()
}
func (a LoggingComponentAdapter) InstallLogging(_ context.Context, c LoggingConfig) (*LoggingStatus, error) {
	return a.Client.InstallLogging(c)
}
func (a LoggingComponentAdapter) UninstallLogging(_ context.Context) error {
	return a.Client.UninstallLogging()
}

type AlertingComponentAdapter struct{ Client *Client }

func (a AlertingComponentAdapter) AlertingStatus(_ context.Context) *AlertingStatus {
	return a.Client.AlertingStatus()
}
func (a AlertingComponentAdapter) InstallAlerting(_ context.Context, c AlertingConfig) (*AlertingStatus, error) {
	return a.Client.InstallAlerting(c)
}
func (a AlertingComponentAdapter) UpdateAlerting(_ context.Context, c AlertingConfig) (*AlertingStatus, error) {
	return a.Client.UpdateAlerting(c)
}
func (a AlertingComponentAdapter) UninstallAlerting(_ context.Context) error {
	return a.Client.UninstallAlerting()
}
