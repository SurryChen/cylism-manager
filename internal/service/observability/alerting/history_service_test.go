package alerting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

type capturedHistory struct {
	HistoryStore
	records []model.AlertHistory
}

func (s *capturedHistory) RecordAlertHistory(_ context.Context, records []model.AlertHistory) error {
	s.records = records
	return nil
}

func TestRecordHistoryValidatesWholeBatchAndBuildsStableFingerprint(t *testing.T) {
	s := &capturedHistory{}
	a := Alert{StartsAt: time.Now().UTC(), Labels: map[string]string{"node": "cloudcone", "alertname": "NodeCPUHigh"}}
	err := RecordHistory(t.Context(), s, 2, AlertNotification{Alerts: []Alert{a, {Labels: a.Labels}}})
	if !errors.Is(err, ErrInvalidNotification) || len(s.records) != 0 {
		t.Fatalf("invalid batch must not write: %+v %v", s.records, err)
	}
	if err := RecordHistory(t.Context(), s, 2, AlertNotification{Alerts: []Alert{a}}); err != nil {
		t.Fatal(err)
	}
	fingerprint := s.records[0].Fingerprint
	a.Labels = map[string]string{"alertname": "NodeCPUHigh", "node": "cloudcone"}
	if err := RecordHistory(t.Context(), s, 2, AlertNotification{Status: "resolved", Alerts: []Alert{a}}); err != nil {
		t.Fatal(err)
	}
	r := s.records[0]
	if r.ClusterID != 2 || r.NodeName != "cloudcone" || r.Status != "resolved" || r.EndsAt == nil || r.Fingerprint == "" || r.Fingerprint != fingerprint {
		t.Fatalf("unexpected converted record: %+v", r)
	}
	a.Labels = map[string]string{"node": "cloudcone"}
	if err := RecordHistory(t.Context(), s, 2, AlertNotification{Alerts: []Alert{a}}); !errors.Is(err, ErrInvalidNotification) {
		t.Fatalf("missing name: %v", err)
	}
}
