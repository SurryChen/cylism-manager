package system

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/repository"
	alertingservice "github.com/cylism/cylism-manager/internal/service/observability/alerting"
	loggingservice "github.com/cylism/cylism-manager/internal/service/observability/logging"
	monitoringservice "github.com/cylism/cylism-manager/internal/service/observability/monitoring"
	"github.com/gin-gonic/gin"
	"k8s.io/client-go/kubernetes"
)

// k8sClient is test-local state used by legacy system handler fixtures. The
// production package no longer keeps a mutable Kubernetes client global.
var k8sClient *k8sclient.Client

func newJSONRequest(method, path string, body interface{}) *http.Request {
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func serve(r *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	return response
}

type fakeNotificationSender struct {
	feishu func(context.Context, string, alertingservice.AlertNotification) error
	email  func(context.Context, alertingservice.EmailConfig, alertingservice.AlertNotification) error
}

func (f *fakeNotificationSender) Send(ctx context.Context, secrets alertingservice.NotificationSecrets, payload alertingservice.AlertNotification) error {
	if secrets.FeishuWebhookURL != "" && f.feishu != nil {
		if err := f.feishu(ctx, secrets.FeishuWebhookURL, payload); err != nil {
			return err
		}
	}
	if secrets.Email.Enabled && f.email != nil {
		return f.email(ctx, secrets.Email, payload)
	}
	return nil
}
func (f *fakeNotificationSender) SendTest(ctx context.Context, secrets alertingservice.NotificationSecrets, payload alertingservice.AlertNotification, channel string) error {
	if channel == "email" {
		if f.email != nil {
			return f.email(ctx, secrets.Email, payload)
		}
		return nil
	}
	if channel == "feishu" {
		if f.feishu != nil {
			return f.feishu(ctx, secrets.FeishuWebhookURL, payload)
		}
		return nil
	}
	return f.Send(ctx, secrets, payload)
}

func newTestAlertingHandler(platformURL ...string) (*AlertingHandler, *fakeNotificationSender) {
	var client *k8sclient.Client = k8sClient
	sender := &fakeNotificationSender{}
	configuredURL := ""
	if len(platformURL) > 0 {
		configuredURL = platformURL[0]
	}
	h := NewAlertingHandler(configuredURL)
	h.WithDependencies(AlertingDependencies{Component: k8sclient.AlertingComponentAdapter{Client: client}, Ready: func(ctx context.Context) bool {
		return client != nil && client.AlertingStatusContext(ctx).State == k8sclient.AlertingStateReady
	}, Secrets: k8sclient.SecretReader{Client: client}, Sender: sender})
	return h, sender
}

func newTestMonitoringHandler() *MonitoringHandler {
	var client *k8sclient.Client = k8sClient
	return NewMonitoringHandler(MonitoringDependencies{Component: k8sclient.MonitoringComponentAdapter{Client: client}, Status: k8sclient.VictoriaMetricsReadiness{Client: client}, Consumers: monitoringservice.PVCConsumerReader{Pods: k8sclient.PodReader{Clientset: func() kubernetes.Interface {
		if client == nil {
			return nil
		}
		return client.Clientset
	}()}}})
}

func newTestLoggingHandler(scope repository.LoggingScopeRepository) *LoggingHandler {
	var client *k8sclient.Client = k8sClient
	return NewLoggingHandler(scope, LoggingDependencies{Component: k8sclient.LoggingComponentAdapter{Client: client}, Ready: func(ctx context.Context) bool {
		return client != nil && client.LoggingStatusContext(ctx).LokiReady >= 1
	}, FilterReader: func() loggingservice.FilterReader {
		if client == nil {
			return nil
		}
		return k8sclient.LoggingFilterReader{Clientset: client.Clientset}
	}()})
}
