package bootstrap

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	systemapi "github.com/cylism/cylism-manager/internal/api/system"
	"github.com/cylism/cylism-manager/internal/k8s"
	alertingservice "github.com/cylism/cylism-manager/internal/service/observability/alerting"
	loggingservice "github.com/cylism/cylism-manager/internal/service/observability/logging"
	monitoringservice "github.com/cylism/cylism-manager/internal/service/observability/monitoring"
	"k8s.io/client-go/rest"
)

// Kubernetes's Service proxy keeps observability endpoints private to their
// cluster. The same mTLS client used for Kubernetes resources reaches them
// through the selected cluster Agent.
func clusterServiceProxyURL(apiHost, serviceAddress string) (string, error) {
	service, err := url.Parse(serviceAddress)
	if err != nil || service.Scheme != "http" || service.Port() == "" {
		return "", fmt.Errorf("集群监控服务地址无效")
	}
	parts := strings.Split(service.Hostname(), ".")
	if len(parts) < 3 || parts[2] != "svc" || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("集群监控服务地址无效")
	}
	return strings.TrimRight(apiHost, "/") + "/api/v1/namespaces/" + parts[1] + "/services/http:" + parts[0] + ":" + service.Port() + "/proxy", nil
}

func (c *Container) clusterObservability(ctx context.Context, clusterID uint, factory *clusterAgentClientFactory) (*systemapi.MonitoringHandler, *systemapi.LoggingHandler, *systemapi.AlertingHandler, error) {
	client, err := factory.Client(ctx, clusterID)
	if err != nil {
		return nil, nil, nil, err
	}
	httpClient, err := rest.HTTPClientFor(client.Config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("创建集群监控连接失败: %w", err)
	}
	metricsURL, err := clusterServiceProxyURL(client.Config.Host, k8s.VictoriaMetricsServiceURL())
	if err != nil {
		return nil, nil, nil, err
	}
	logsURL, err := clusterServiceProxyURL(client.Config.Host, k8s.LokiServiceURL())
	if err != nil {
		return nil, nil, nil, err
	}
	alertsURL, err := clusterServiceProxyURL(client.Config.Host, k8s.AlertmanagerServiceURL())
	if err != nil {
		return nil, nil, nil, err
	}
	metrics := &monitoringservice.HTTPClient{BaseURL: func() string { return metricsURL }, Client: httpClient}
	metricsReady := monitoringservice.StatusFunc(func(ctx context.Context) bool { return client.VictoriaMetricsStatusContext(ctx).ReadyReplicas > 0 })
	monitoring := systemapi.NewMonitoringHandler(systemapi.MonitoringDependencies{
		Query: metrics.Query, Status: metricsReady, Component: client,
		Consumers: monitoringservice.PVCConsumerReader{Pods: client},
	})
	logClient := loggingservice.HTTPClient{BaseURL: logsURL, Client: httpClient}
	logging := systemapi.NewLoggingHandler(c.Store, systemapi.LoggingDependencies{
		Query: logClient.Query, Ready: func(ctx context.Context) bool { return client.LoggingStatusContext(ctx).LokiReady >= 1 },
		Component: client, FilterReader: client,
	})
	alertClient := &alertingservice.HTTPClient{BaseURL: func() string { return alertsURL }, Client: httpClient}
	alertReady := func(ctx context.Context) bool {
		return client.AlertingStatusContext(ctx).State == k8s.AlertingStateReady
	}
	alerting := systemapi.NewAlertingHandler(c.Auth.PlatformURL).WithDependencies(systemapi.AlertingDependencies{
		Alertmanager: alertClient.Request, Component: client, Ready: alertReady, Secrets: client,
		Sender: alertingservice.DefaultNotificationSender{},
	})
	return monitoring, logging, alerting, nil
}
