package alerting

import (
	"strings"
	"testing"
)

func TestNotificationRenderersIncludeAlertContext(t *testing.T) {
	payload := AlertNotification{Status: "firing", Alerts: []Alert{{Labels: map[string]string{"alertname": "NodeDiskHigh", "node": "node-a"}, Annotations: map[string]string{"summary": "disk high", "current_value": "92%"}}}}
	card := BuildFeishuCard(payload, "https://example.test/alerts")
	if !strings.Contains(card["card"].(map[string]interface{})["elements"].([]interface{})[0].(map[string]interface{})["fields"].([]interface{})[0].(map[string]interface{})["text"].(map[string]interface{})["content"].(string), "告警规则") {
		t.Fatal("card should contain alert fields")
	}
	if !strings.Contains(BuildEmailText(payload, "https://example.test/alerts"), "92%") {
		t.Fatal("plain email should contain current value")
	}
	if !strings.Contains(BuildEmailHTML(payload, "https://example.test/alerts"), "查看平台告警") {
		t.Fatal("html email should contain platform link")
	}
}
