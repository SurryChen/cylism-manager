package alerting

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

type AlertStatus struct {
	State string `json:"state"`
}

func (s *AlertStatus) UnmarshalJSON(data []byte) error {
	var state string
	if err := json.Unmarshal(data, &state); err == nil {
		s.State = state
		return nil
	}
	var obj struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	s.State = obj.State
	return nil
}

type Alert struct {
	Fingerprint  string            `json:"fingerprint,omitempty"`
	Status       AlertStatus       `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL,omitempty"`
}
type Overview struct {
	Active, Resolved []Alert
	Firing, Silenced int
}
type Matcher struct {
	Name, Value      string
	IsRegex, IsEqual bool
}
type SilenceRequest struct {
	Matchers        []Matcher
	DurationMinutes int
	Comment         string
}

func IsFiring(alert Alert) bool {
	switch strings.ToLower(strings.TrimSpace(alert.Status.State)) {
	case "active", "firing", "unprocessed":
		return true
	}
	return false
}
func BuildOverview(alerts []Alert, recent []Alert) Overview {
	o := Overview{Active: make([]Alert, 0), Resolved: make([]Alert, 0)}
	for _, a := range alerts {
		switch a.Status.State {
		case "resolved":
			o.Resolved = append(o.Resolved, a)
		case "suppressed":
			o.Silenced++
			o.Active = append(o.Active, a)
		default:
			o.Active = append(o.Active, a)
			if IsFiring(a) {
				o.Firing++
			}
		}
	}
	if len(o.Resolved) > 12 {
		o.Resolved = o.Resolved[:12]
	}
	if len(recent) > 0 {
		o.Resolved = append(recent, o.Resolved...)
		if len(o.Resolved) > 12 {
			o.Resolved = o.Resolved[:12]
		}
	}
	return o
}
func ValidateSilence(r *SilenceRequest) error {
	if len(r.Matchers) == 0 || len(r.Matchers) > 12 || r.DurationMinutes < 1 || r.DurationMinutes > 7*24*60 {
		return fmt.Errorf("静默需包含匹配条件，且时长应在 1 分钟到 7 天之间")
	}
	for i := range r.Matchers {
		r.Matchers[i].Name = strings.TrimSpace(r.Matchers[i].Name)
		r.Matchers[i].Value = strings.TrimSpace(r.Matchers[i].Value)
		r.Matchers[i].IsEqual = true
		if r.Matchers[i].Name == "" || r.Matchers[i].Value == "" {
			return fmt.Errorf("静默匹配条件不能为空")
		}
	}
	return nil
}
func EventFromAlert(a Alert, resolved bool) *model.AlertEvent {
	labels, _ := json.Marshal(a.Labels)
	annotations, _ := json.Marshal(a.Annotations)
	name := strings.TrimSpace(a.Labels["alertname"])
	if name == "" {
		name = "unnamed-alert"
	}
	fp := strings.TrimSpace(a.Fingerprint)
	if fp == "" {
		fp = fmt.Sprintf("%x", sha256.Sum256([]byte(name+"|"+a.Labels["node"]+"|"+a.Labels["mountpoint"]+"|"+a.StartsAt.UTC().Format(time.RFC3339Nano))))
	}
	starts := a.StartsAt
	if starts.IsZero() {
		starts = time.Now().UTC()
	}
	status := model.AlertEventFiring
	var ends *time.Time
	if resolved || a.Status.State == "resolved" {
		status = model.AlertEventResolved
		end := a.EndsAt
		if end.IsZero() {
			end = time.Now().UTC()
		}
		ends = &end
	}
	return &model.AlertEvent{Fingerprint: fp, AlertName: name, Severity: strings.ToLower(strings.TrimSpace(a.Labels["severity"])), NodeName: strings.TrimSpace(a.Labels["node"]), MountPoint: strings.TrimSpace(a.Labels["mountpoint"]), Labels: string(labels), Annotations: string(annotations), Status: status, StartsAt: starts, EndsAt: ends}
}
