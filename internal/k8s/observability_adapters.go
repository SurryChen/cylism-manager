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

func (r VictoriaMetricsReadiness) Ready(ctx context.Context) bool {
	if r.Client == nil {
		return false
	}
	return r.Client.withContext(ctx).VictoriaMetricsStatus().ReadyReplicas > 0
}

type LoggingReadiness struct{ Client *Client }

func (r LoggingReadiness) Ready(ctx context.Context) bool {
	if r.Client == nil {
		return false
	}
	return r.Client.withContext(ctx).LoggingStatus().LokiReady >= 1
}

func (a MonitoringComponentAdapter) VictoriaMetricsStatus(ctx context.Context) *VictoriaMetricsStatus {
	if a.Client == nil {
		return (&Client{}).VictoriaMetricsStatus()
	}
	return a.Client.VictoriaMetricsStatusContext(ctx)
}
func (a MonitoringComponentAdapter) InstallVictoriaMetrics(ctx context.Context, c VictoriaMetricsConfig) (*VictoriaMetricsStatus, error) {
	return a.Client.InstallVictoriaMetricsContext(ctx, c)
}
func (a MonitoringComponentAdapter) UninstallVictoriaMetrics(ctx context.Context) error {
	return a.Client.UninstallVictoriaMetricsContext(ctx)
}
func (a MonitoringComponentAdapter) StartVictoriaMetricsHostPathMigration(ctx context.Context, c VictoriaMetricsMigrationRequest) (*VictoriaMetricsStatus, error) {
	return a.Client.withContext(ctx).startVictoriaMetricsHostPathMigration(c)
}

func (c *Client) StartVictoriaMetricsHostPathMigrationContext(ctx context.Context, request VictoriaMetricsMigrationRequest) (*VictoriaMetricsStatus, error) {
	return c.withContext(ctx).startVictoriaMetricsHostPathMigration(request)
}

type LoggingComponentAdapter struct{ Client *Client }

func (a LoggingComponentAdapter) LoggingStatus(ctx context.Context) *LoggingStatus {
	return a.Client.LoggingStatusContext(ctx)
}
func (a LoggingComponentAdapter) InstallLogging(ctx context.Context, c LoggingConfig) (*LoggingStatus, error) {
	return a.Client.InstallLoggingContext(ctx, c)
}
func (a LoggingComponentAdapter) UninstallLogging(ctx context.Context) error {
	return a.Client.UninstallLoggingContext(ctx)
}

type AlertingComponentAdapter struct{ Client *Client }

func (a AlertingComponentAdapter) AlertingStatus(ctx context.Context) *AlertingStatus {
	return a.Client.AlertingStatusContext(ctx)
}

func (c *Client) LoggingStatusContext(ctx context.Context) *LoggingStatus {
	return c.withContext(ctx).LoggingStatus()
}
func (c *Client) InstallLoggingContext(ctx context.Context, config LoggingConfig) (*LoggingStatus, error) {
	return c.withContext(ctx).installLogging(config)
}
func (c *Client) UninstallLoggingContext(ctx context.Context) error {
	return c.withContext(ctx).uninstallLogging()
}
func (c *Client) AlertingStatusContext(ctx context.Context) *AlertingStatus {
	return c.withContext(ctx).AlertingStatus()
}
func (c *Client) InstallAlertingContext(ctx context.Context, config AlertingConfig) (*AlertingStatus, error) {
	return c.withContext(ctx).installAlerting(config)
}
func (c *Client) UpdateAlertingContext(ctx context.Context, config AlertingConfig) (*AlertingStatus, error) {
	return c.withContext(ctx).updateAlerting(config)
}
func (c *Client) UninstallAlertingContext(ctx context.Context) error {
	return c.withContext(ctx).uninstallAlerting()
}
func (a AlertingComponentAdapter) InstallAlerting(ctx context.Context, c AlertingConfig) (*AlertingStatus, error) {
	return a.Client.InstallAlertingContext(ctx, c)
}
func (a AlertingComponentAdapter) UpdateAlerting(ctx context.Context, c AlertingConfig) (*AlertingStatus, error) {
	return a.Client.UpdateAlertingContext(ctx, c)
}
func (a AlertingComponentAdapter) UninstallAlerting(ctx context.Context) error {
	return a.Client.UninstallAlertingContext(ctx)
}
