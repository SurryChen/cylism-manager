package alerting

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Silence struct {
	ID        string      `json:"id,omitempty"`
	Matchers  []Matcher   `json:"matchers"`
	StartsAt  time.Time   `json:"startsAt"`
	EndsAt    time.Time   `json:"endsAt"`
	CreatedBy string      `json:"createdBy,omitempty"`
	Comment   string      `json:"comment,omitempty"`
	Status    AlertStatus `json:"status"`
}
type SilenceResult struct {
	SilenceID string `json:"silenceID"`
}

type QueryService struct {
	client *Client
	ready  func() bool
}

func NewQueryService(client *Client, ready func() bool) *QueryService {
	return &QueryService{client: client, ready: ready}
}
func (s *QueryService) ensureReady() error {
	if s == nil || s.client == nil {
		return fmt.Errorf("Alertmanager 查询不可用")
	}
	if s.ready != nil && !s.ready() {
		return fmt.Errorf("Alertmanager 尚未就绪")
	}
	return nil
}
func (s *QueryService) Overview(ctx context.Context, recent []Alert) (Overview, error) {
	if err := s.ensureReady(); err != nil {
		return Overview{}, err
	}
	var alerts []Alert
	if err := s.client.Request(ctx, http.MethodGet, "/api/v2/alerts", nil, &alerts); err != nil {
		return Overview{}, err
	}
	return BuildOverview(alerts, recent), nil
}
func (s *QueryService) Silences(ctx context.Context) ([]Silence, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	var out []Silence
	if err := s.client.Request(ctx, http.MethodGet, "/api/v2/silences", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *QueryService) CreateSilence(ctx context.Context, req SilenceRequest, now time.Time) (SilenceResult, time.Time, error) {
	if err := s.ensureReady(); err != nil {
		return SilenceResult{}, time.Time{}, err
	}
	if err := ValidateSilence(&req); err != nil {
		return SilenceResult{}, time.Time{}, err
	}
	payload := Silence{Matchers: req.Matchers, StartsAt: now.UTC(), EndsAt: now.UTC().Add(time.Duration(req.DurationMinutes) * time.Minute), CreatedBy: "cylism-manager", Comment: strings.TrimSpace(req.Comment)}
	var out SilenceResult
	if err := s.client.Request(ctx, http.MethodPost, "/api/v2/silences", payload, &out); err != nil {
		return out, payload.EndsAt, err
	}
	return out, payload.EndsAt, nil
}
func (s *QueryService) DeleteSilence(ctx context.Context, id string) error {
	if err := s.ensureReady(); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 128 {
		return fmt.Errorf("静默 ID 无效")
	}
	return s.client.Request(ctx, http.MethodDelete, "/api/v2/silence/"+id, nil, nil)
}
