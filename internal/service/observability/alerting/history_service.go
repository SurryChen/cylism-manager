package alerting

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

type HistoryStore interface {
	RecordAlertHistory(context.Context, []model.AlertHistory) error
	ListAlertHistory(context.Context, model.AlertHistoryFilter) ([]model.AlertHistory, int64, error)
	ListAlertFrequency(context.Context, model.AlertHistoryFilter) ([]model.AlertFrequency, int64, error)
}

func RecordHistory(ctx context.Context, store HistoryStore, clusterID uint, payload AlertNotification) error {
	if store == nil {
		return nil
	}
	records := make([]model.AlertHistory, 0, len(payload.Alerts))
	for _, alert := range payload.Alerts {
		if alert.StartsAt.IsZero() || strings.TrimSpace(alert.Labels["alertname"]) == "" {
			return ErrInvalidNotification
		}
		labels, err := json.Marshal(alert.Labels)
		if err != nil {
			return err
		}
		annotations, err := json.Marshal(alert.Annotations)
		if err != nil {
			return err
		}
		fingerprint := alert.Fingerprint
		if fingerprint == "" {
			fingerprint = fmt.Sprintf("%x", sha256.Sum256(labels))
		}
		record := model.AlertHistory{ClusterID: clusterID, Fingerprint: fingerprint, StartsAt: alert.StartsAt.UTC(),
			NotificationSuppressed: alert.NotificationSuppressed,
			AlertName:              alert.Labels["alertname"], RuleName: alert.Annotations["rule_name"], NodeName: alert.Labels["node"],
			Severity: alert.Labels["severity"], Status: "firing", Summary: alert.Annotations["summary"], Labels: string(labels), Annotations: string(annotations)}
		if strings.EqualFold(alert.Status.State, "resolved") || strings.EqualFold(payload.Status, "resolved") {
			record.Status = "resolved"
			ended := alert.EndsAt.UTC()
			if ended.IsZero() {
				ended = time.Now().UTC()
			}
			record.EndsAt = &ended
		}
		records = append(records, record)
	}
	return store.RecordAlertHistory(ctx, records)
}
