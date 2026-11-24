package system

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	alertingservice "github.com/cylism/cylism-manager/internal/service/observability/alerting"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

type historyRelaySecrets struct{ configured bool }

func (s historyRelaySecrets) GetSecretDataContext(context.Context, string, string) (map[string][]byte, error) {
	data := map[string][]byte{"relay-token": []byte("history-token")}
	if s.configured {
		data["feishu-webhook-url"] = []byte("https://open.feishu.cn/open-apis/bot/v2/hook/example")
	}
	return data, nil
}

func TestAlertHistoryPersistsWithoutAutomationOrNotificationChannel(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	h := NewAlertingHandler("").WithCluster(2).WithHistory(s).WithDependencies(AlertingDependencies{Secrets: historyRelaySecrets{}})
	r := setupAlertingRouter(h)
	for _, token := range []string{"wrong-token", "history-token", "history-token"} {
		request := newJSONRequest(http.MethodPost, "/api/monitoring/alerts/notify", gin.H{"status": "firing", "alerts": []gin.H{{"status": "firing", "startsAt": time.Now().UTC().Truncate(time.Hour), "fingerprint": "cpu", "labels": gin.H{"alertname": "NodeCPUHigh", "node": "cloudcone"}}}})
		request.Header.Set("Authorization", "Bearer "+token)
		response := serve(r, request)
		if token == "wrong-token" {
			if response.Code != http.StatusUnauthorized {
				t.Fatal(response.Body.String())
			}
			_, total, _ := s.ListAlertHistory(t.Context(), model.AlertHistoryFilter{ClusterID: 2})
			if total != 0 {
				t.Fatal("unauthorized callback persisted")
			}
		} else if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"delivered":false`) {
			t.Fatal(response.Body.String())
		}
	}
	items, total, err := s.ListAlertHistory(t.Context(), model.AlertHistoryFilter{ClusterID: 2})
	if err != nil || total != 1 || len(items) != 1 || items[0].NodeName != "cloudcone" {
		t.Fatalf("history: %+v %d %v", items, total, err)
	}
	// Delivery failures must not lose a persisted occurrence; the sender can retry.
	h.WithDependencies(AlertingDependencies{Secrets: historyRelaySecrets{configured: true}, Sender: &fakeNotificationSender{feishu: func(context.Context, string, alertingservice.AlertNotification) error { return errors.New("offline") }}})
	request := newJSONRequest(http.MethodPost, "/api/monitoring/alerts/notify", gin.H{"alerts": []gin.H{{"status": "firing", "startsAt": time.Now().UTC(), "labels": gin.H{"alertname": "NodeDiskHigh", "node": "cloudcone"}}}})
	request.Header.Set("Authorization", "Bearer history-token")
	if response := serve(r, request); response.Code != http.StatusBadGateway {
		t.Fatalf("delivery failure: %d %s", response.Code, response.Body.String())
	}
	_, total, err = s.ListAlertHistory(t.Context(), model.AlertHistoryFilter{ClusterID: 2})
	if err != nil || total != 2 {
		t.Fatalf("history lost on delivery error: %d %v", total, err)
	}
}

func TestAlertHistoryReadsWithoutLiveClusterAndValidatesFilters(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordAlertHistory(t.Context(), []model.AlertHistory{
		{ClusterID: 2, Fingerprint: "cpu", StartsAt: time.Now().UTC(), NodeName: "cloudcone", AlertName: "NodeCPUHigh", Status: "firing"},
		{ClusterID: 3, Fingerprint: "cpu", StartsAt: time.Now().UTC(), NodeName: "other", AlertName: "NodeCPUHigh", Status: "firing"},
	}); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// No Kubernetes, readiness or cluster resolver is configured.
	r.GET("/clusters/:id/history", NewAlertingHandler("").WithHistory(s).History)
	for _, tc := range []struct {
		path     string
		want     int
		contains string
	}{
		{"/clusters/2/history?node=cloudcone&alert_name=NodeCPUHigh&days=1&status=firing", 200, `"total":1`},
		{"/clusters/2/history?view=frequency&node=cloudcone", 200, `"count":1`},
		{"/clusters/2/history?node=other", 200, `"items":[]`},
		{"/clusters/0/history", 400, "集群 ID 无效"},
		{"/clusters/abc/history", 400, "集群 ID 无效"},
		{"/clusters/2/history?days=365", 400, "筛选条件无效"},
		{"/clusters/2/history?status=active", 400, "筛选条件无效"},
		{"/clusters/2/history?view=unknown", 400, "筛选条件无效"},
	} {
		response := serve(r, newJSONRequest(http.MethodGet, tc.path, nil))
		if response.Code != tc.want || !strings.Contains(response.Body.String(), tc.contains) {
			t.Errorf("%s: %d %s", tc.path, response.Code, response.Body.String())
		}
	}
}
