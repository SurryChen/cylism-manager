package system

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	alertingservice "github.com/cylism/cylism-manager/internal/service/observability/alerting"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func suppressionTestRouter(h *AlertingHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/clusters/:id/rules")
	g.GET("", h.ListSuppressionRules)
	g.POST("", h.SaveSuppressionRule)
	g.PUT("/:ruleID", h.SaveSuppressionRule)
	g.PATCH("/:ruleID/enabled", h.SetSuppressionEnabled)
	g.DELETE("/:ruleID", h.DeleteSuppressionRule)
	return r
}

func TestSuppressionManagementWorksWithoutClusterAndRejectsUnsafeOrCrossClusterUpdates(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	r := suppressionTestRouter(NewAlertingHandler("").WithSuppressionRules(s))
	for _, tc := range []struct {
		method, path string
		body         interface{}
		want         int
	}{
		{"POST", "/clusters/2/rules", gin.H{"name": "all", "enabled": true, "duration_minutes": 0}, 400},
		{"POST", "/clusters/2/rules", gin.H{"name": "bad duration", "node_name": "cloudcone", "enabled": true, "duration_minutes": -1}, 400},
		{"POST", "/clusters/2/rules", gin.H{"name": "cloudcone", "node_name": "cloudcone", "alert_names": []string{"NodeCPUHigh", "NodeMemoryHigh"}, "enabled": true, "duration_minutes": 0}, 200},
		{"GET", "/clusters/2/rules?status=active", nil, 200},
		{"PATCH", "/clusters/3/rules/1/enabled", gin.H{"enabled": false}, 404},
		{"DELETE", "/clusters/3/rules/1", nil, 404},
		{"PUT", "/clusters/3/rules/1", gin.H{"name": "changed", "node_name": "other", "enabled": false}, 404},
		{"PATCH", "/clusters/2/rules/1/enabled", gin.H{"enabled": false}, 200},
		{"PATCH", "/clusters/2/rules/1/enabled", gin.H{}, 400},
		{"GET", "/clusters/invalid/rules", nil, 400},
		{"GET", "/clusters/2/rules?status=bad", nil, 400},
		{"DELETE", "/clusters/2/rules/invalid", nil, 400},
	} {
		response := serve(r, newJSONRequest(tc.method, tc.path, tc.body))
		if response.Code != tc.want {
			t.Errorf("%s %s: %d %s", tc.method, tc.path, response.Code, response.Body.String())
		}
	}
	response := serve(r, newJSONRequest("GET", "/clusters/2/rules?status=disabled", nil))
	if !strings.Contains(response.Body.String(), `"alert_names":["NodeCPUHigh","NodeMemoryHigh"]`) || !strings.Contains(response.Body.String(), `"total":1`) {
		t.Fatal(response.Body.String())
	}
	if response := serve(r, newJSONRequest("DELETE", "/clusters/2/rules/1", nil)); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
}

func TestSuppressionFiltersMixedCallbacksAndKeepsHistory(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	rule := model.AlertSuppressionRule{ClusterID: 2, Name: "cloudcone CPU", NodeName: "cloudcone", AlertNames: []string{"NodeCPUHigh"}, Enabled: true, StartsAt: now.Add(-time.Minute)}
	if err := s.CreateAlertSuppressionRule(t.Context(), &rule); err != nil {
		t.Fatal(err)
	}
	sends := 0
	sender := &fakeNotificationSender{feishu: func(_ context.Context, _ string, payload alertingservice.AlertNotification) error {
		sends++
		if len(payload.Alerts) != 1 || payload.Alerts[0].Labels["node"] != "other" {
			t.Fatalf("suppressed alert leaked: %+v", payload)
		}
		return nil
	}}
	h := NewAlertingHandler("").WithCluster(2).WithHistory(s).WithSuppressionRules(s).WithDependencies(AlertingDependencies{Secrets: historyRelaySecrets{configured: true}, Sender: sender})
	r := setupAlertingRouter(h)
	callback := func(alerts []gin.H) *http.Request {
		req := newJSONRequest("POST", "/api/monitoring/alerts/notify", gin.H{"status": "firing", "alerts": alerts})
		req.Header.Set("Authorization", "Bearer history-token")
		return req
	}
	alert := func(node string) gin.H {
		return gin.H{"status": "firing", "startsAt": now, "fingerprint": fmt.Sprintf("cpu-%s", node), "labels": gin.H{"node": node, "alertname": "NodeCPUHigh"}}
	}
	response := serve(r, callback([]gin.H{alert("cloudcone"), alert("other")}))
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"suppressed":1`) || sends != 1 {
		t.Fatalf("partial delivery: %d %s sends=%d", response.Code, response.Body.String(), sends)
	}
	items, total, err := s.ListAlertHistory(t.Context(), model.AlertHistoryFilter{ClusterID: 2, NodeName: "cloudcone"})
	if err != nil || total != 1 || !items[0].NotificationSuppressed {
		t.Fatalf("suppressed alert history: %+v %d %v", items, total, err)
	}
	response = serve(r, callback([]gin.H{alert("cloudcone")}))
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"delivered":false`) || sends != 1 {
		t.Fatalf("all suppressed: %s sends=%d", response.Body.String(), sends)
	}
	// Restarting the handler and using another cluster must never reuse rules.
	h3 := NewAlertingHandler("").WithCluster(3).WithHistory(s).WithSuppressionRules(s).WithDependencies(AlertingDependencies{Secrets: historyRelaySecrets{configured: true}, Sender: &fakeNotificationSender{feishu: func(_ context.Context, _ string, payload alertingservice.AlertNotification) error {
		if len(payload.Alerts) != 1 {
			t.Fatal("other cluster delivery lost")
		}
		return nil
	}}})
	response = serve(setupAlertingRouter(h3), callback([]gin.H{alert("cloudcone")}))
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"delivered":true`) || !strings.Contains(response.Body.String(), `"suppressed":0`) {
		t.Fatal(response.Body.String())
	}
	if err := s.SetAlertSuppressionEnabled(t.Context(), 2, rule.ID, false); err != nil {
		t.Fatal(err)
	}
	sender.feishu = func(_ context.Context, _ string, payload alertingservice.AlertNotification) error {
		sends++
		return nil
	}
	response = serve(r, callback([]gin.H{alert("cloudcone")}))
	if response.Code != 200 || sends != 2 {
		t.Fatalf("disabled rule still suppresses: %s sends=%d", response.Body.String(), sends)
	}
	items, _, _ = s.ListAlertHistory(t.Context(), model.AlertHistoryFilter{ClusterID: 2, NodeName: "cloudcone"})
	if items[0].NotificationSuppressed {
		t.Fatal("last callback status not updated")
	}
}
