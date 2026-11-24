package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	systemapi "github.com/cylism/cylism-manager/internal/api/system"
	alertingservice "github.com/cylism/cylism-manager/internal/service/observability/alerting"
	"github.com/gin-gonic/gin"
)

type clusterCallbackSecrets struct{ token string }

func (s clusterCallbackSecrets) GetSecretDataContext(context.Context, string, string) (map[string][]byte, error) {
	return map[string][]byte{"relay-token": []byte(s.token), "feishu-webhook-url": []byte("https://open.feishu.cn/open-apis/bot/v2/hook/example")}, nil
}

type clusterCallbackSender struct {
	clusterID uint
	delivered *[]uint
}

func (s clusterCallbackSender) Send(_ context.Context, _ alertingservice.NotificationSecrets, payload alertingservice.AlertNotification) error {
	if payload.PlatformURL != fmt.Sprintf("https://manager.example/#/cluster/%d/monitoring?tab=alerts", s.clusterID) {
		return fmt.Errorf("notification links to another cluster: %s", payload.PlatformURL)
	}
	*s.delivered = append(*s.delivered, s.clusterID)
	return nil
}
func (s clusterCallbackSender) SendTest(ctx context.Context, secrets alertingservice.NotificationSecrets, payload alertingservice.AlertNotification, _ string) error {
	return s.Send(ctx, secrets, payload)
}

func TestClusterAlertCallbacksAuthenticateAgainstSelectedCluster(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	protected := router.Group("/api", func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) })
	var delivered []uint
	registerClusterObservabilityRoutes(router, protected, func(_ context.Context, id uint) (*systemapi.MonitoringHandler, *systemapi.LoggingHandler, *systemapi.AlertingHandler, error) {
		if id != 1 && id != 2 {
			return nil, nil, nil, fmt.Errorf("unknown cluster")
		}
		handler := systemapi.NewAlertingHandler("https://manager.example").WithCluster(id).WithDependencies(systemapi.AlertingDependencies{
			Secrets: clusterCallbackSecrets{token: fmt.Sprintf("cluster-%d-token", id)},
			Sender:  clusterCallbackSender{clusterID: id, delivered: &delivered},
		})
		return nil, nil, handler, nil
	})
	for _, tc := range []struct {
		id, token string
		want      int
	}{
		{"1", "", http.StatusUnauthorized},
		{"1", "cluster-2-token", http.StatusUnauthorized},
		{"2", "cluster-1-token", http.StatusUnauthorized},
		{"2", "cluster-2-token", http.StatusOK},
		{"1", "cluster-1-token", http.StatusOK},
		{"invalid", "cluster-1-token", http.StatusBadRequest},
		{"3", "cluster-1-token", http.StatusBadRequest},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/clusters/"+tc.id+"/monitoring/alerts/notify", strings.NewReader(`{"status":"firing","alerts":[{"status":"firing","labels":{"alertname":"NodeDown"}}]}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+tc.token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != tc.want {
			t.Errorf("cluster %s token %q: status = %d, want %d: %s", tc.id, tc.token, response.Code, tc.want, response.Body.String())
		}
	}
	if fmt.Sprint(delivered) != "[2 1]" {
		t.Fatalf("unexpected delivered notifications: %v", delivered)
	}
	request := httptest.NewRequest(http.MethodPut, "/api/clusters/1/monitoring/alerts/config", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer cluster-1-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("relay token must not bypass management authentication: %d", response.Code)
	}
}
