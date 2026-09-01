package alerting

import (
	"bytes"
	"fmt"
	"html"
	"mime"
	"mime/quotedprintable"
	"strconv"
	"strings"
)

type NotificationField struct{ Label, Value string }

func NotificationFields(alert Alert) []NotificationField {
	name := alert.Labels["alertname"]
	if name == "" {
		name = "集群告警"
	}
	rule := alert.Annotations["rule_name"]
	if rule == "" {
		rule = name
	}
	summary := alert.Annotations["summary"]
	if summary == "" {
		summary = alert.Annotations["description"]
	}
	fields := []NotificationField{{"告警规则", rule}, {"告警对象", NotificationTarget(alert)}, {"告警说明", summary}}
	for _, item := range []struct{ key, label, prefix string }{{"current_value", "当前值", ""}, {"threshold", "阈值", ""}, {"duration", "触发条件", "持续 "}} {
		if value := strings.TrimSpace(alert.Annotations[item.key]); value != "" {
			fields = append(fields, NotificationField{item.label, item.prefix + value})
		}
	}
	if !alert.StartsAt.IsZero() {
		fields = append(fields, NotificationField{"开始时间", alert.StartsAt.Local().Format("2006-01-02 15:04:05 MST")})
	}
	return fields
}

func NotificationTarget(alert Alert) string {
	if node := strings.TrimSpace(alert.Labels["node"]); node != "" {
		return "节点 " + node
	}
	ns := strings.TrimSpace(alert.Labels["namespace"])
	for _, key := range []string{"pod", "deployment", "statefulset", "daemonset", "job"} {
		if value := strings.TrimSpace(alert.Labels[key]); value != "" {
			if ns != "" {
				return ns + "/" + value
			}
			return value
		}
	}
	if ns != "" {
		return "命名空间 " + ns
	}
	return "集群"
}

func BuildFeishuCard(payload AlertNotification, platformURL string) map[string]interface{} {
	state := "告警恢复"
	if payload.Status == "firing" {
		state = "告警触发"
	}
	elements := make([]interface{}, 0, len(payload.Alerts)*2+2)
	for _, alert := range payload.Alerts {
		fields := make([]interface{}, 0, 6)
		for _, field := range NotificationFields(alert) {
			fields = append(fields, map[string]interface{}{"is_short": field.Label != "告警说明", "text": map[string]interface{}{"tag": "lark_md", "content": "**" + EscapeLarkMarkdown(field.Label) + "**\n" + EscapeLarkMarkdown(field.Value)}})
		}
		elements = append(elements, map[string]interface{}{"tag": "div", "fields": fields}, map[string]interface{}{"tag": "hr"})
	}
	if len(elements) > 0 {
		elements = elements[:len(elements)-1]
	}
	if platformURL != "" {
		elements = append(elements, map[string]interface{}{"tag": "action", "actions": []interface{}{map[string]interface{}{"tag": "button", "text": map[string]interface{}{"tag": "plain_text", "content": "查看平台告警"}, "type": "primary", "url": platformURL}}})
	}
	return map[string]interface{}{"msg_type": "interactive", "card": map[string]interface{}{"config": map[string]interface{}{"wide_screen_mode": true, "enable_forward": true}, "header": map[string]interface{}{"title": map[string]interface{}{"tag": "plain_text", "content": fmt.Sprintf("%s · %d 条", state, len(payload.Alerts))}, "template": map[bool]string{true: "red", false: "green"}[payload.Status == "firing"]}, "elements": elements}}
}

func BuildEmailText(payload AlertNotification, platformURL string) string {
	state := "告警恢复"
	if payload.Status == "firing" {
		state = "告警触发"
	}
	lines := []string{state, ""}
	for _, alert := range payload.Alerts {
		for _, field := range NotificationFields(alert) {
			lines = append(lines, field.Label+": "+field.Value)
		}
		lines = append(lines, "")
	}
	if platformURL != "" {
		lines = append(lines, "查看平台告警: "+platformURL)
	}
	return strings.Join(lines, "\n")
}

func BuildEmailHTML(payload AlertNotification, platformURL string) string {
	state := "告警恢复"
	accent := "#198754"
	if payload.Status == "firing" {
		state = "告警触发"
		accent = "#d92d20"
	}
	var body strings.Builder
	body.WriteString(`<!doctype html><html><body style="margin:0;background:#f5f7fa;color:#1f2937;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;"><div style="max-width:680px;margin:24px auto;background:#ffffff;border:1px solid #e5e7eb;border-radius:8px;overflow:hidden;"><div style="padding:18px 22px;background:` + accent + `;color:#ffffff;font-size:18px;font-weight:700;">` + html.EscapeString(state) + ` · ` + strconv.Itoa(len(payload.Alerts)) + ` 条</div><div style="padding:20px 22px;">`)
	for _, alert := range payload.Alerts {
		body.WriteString(`<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="margin:0 0 16px;border:1px solid #e5e7eb;border-radius:6px;border-collapse:separate;overflow:hidden;">`)
		for _, field := range NotificationFields(alert) {
			body.WriteString(`<tr><td style="width:112px;padding:10px 12px;background:#f9fafb;color:#6b7280;font-size:12px;border-bottom:1px solid #e5e7eb;vertical-align:top;">` + html.EscapeString(field.Label) + `</td><td style="padding:10px 12px;font-size:14px;border-bottom:1px solid #e5e7eb;word-break:break-word;">` + html.EscapeString(field.Value) + `</td></tr>`)
		}
		body.WriteString(`</table>`)
	}
	if platformURL != "" {
		body.WriteString(`<a href="` + html.EscapeString(platformURL) + `" style="display:inline-block;padding:10px 15px;background:#2563eb;color:#ffffff;text-decoration:none;border-radius:6px;font-weight:600;">查看平台告警</a>`)
	}
	body.WriteString(`</div></div></body></html>`)
	return body.String()
}

func BuildEmailMIME(from, to string, payload AlertNotification, platformURL string) string {
	boundary := "cylism-alert-message"
	subject := "Cylism 告警恢复"
	if payload.Status == "firing" {
		subject = "Cylism 告警触发"
	}
	plain, htmlBody := BuildEmailText(payload, platformURL), BuildEmailHTML(payload, platformURL)
	return "From: " + from + "\r\nTo: " + to + "\r\nSubject: " + mime.QEncoding.Encode("UTF-8", subject) + "\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n--" + boundary + "\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n" + QuotedPrintableEncode(plain) + "\r\n--" + boundary + "\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n" + QuotedPrintableEncode(htmlBody) + "\r\n--" + boundary + "--\r\n"
}

func QuotedPrintableEncode(body string) string {
	var encoded bytes.Buffer
	writer := quotedprintable.NewWriter(&encoded)
	_, _ = writer.Write([]byte(body))
	_ = writer.Close()
	return encoded.String()
}
func EscapeLarkMarkdown(value string) string {
	return strings.NewReplacer("\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "(", "\\(", ")", "\\)").Replace(value)
}
