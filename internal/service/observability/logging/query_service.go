package logging

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

type QueryFunc func(context.Context, string, url.Values) (Response, error)

// QueryService owns readiness, request validation, scope resolution and the
// bounded Loki query workflow. HTTP handlers only decode and map DTOs.
type QueryService struct {
	query QueryFunc
	ready func() bool
	scope ScopeResolver
}

func NewQueryService(query QueryFunc, ready func() bool, scope ScopeResolver) *QueryService {
	return &QueryService{query: query, ready: ready, scope: scope}
}

func (s *QueryService) Query(ctx context.Context, req QueryRequest, now time.Time) ([]Line, error) {
	if s == nil || s.query == nil {
		return nil, fmt.Errorf("日志查询不可用")
	}
	if s.ready != nil && !s.ready() {
		return nil, fmt.Errorf("日志采集尚未就绪")
	}
	if err := ValidateQuery(&req); err != nil {
		return nil, err
	}
	if err := ResolveApplicationScope(&req, s.scope); err != nil {
		return nil, err
	}
	return Query(ctx, req, now, s.query)
}

func Query(ctx context.Context, req QueryRequest, now time.Time, query QueryFunc) ([]Line, error) {
	if err := ValidateQuery(&req); err != nil {
		return nil, err
	}
	qs, err := BuildLogQLQueries(req)
	if err != nil {
		return nil, err
	}
	start, end, err := ResolveBounds(req, now)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	responses := make([]Response, 0, len(qs))
	for _, q := range qs {
		r, e := query(ctx, "/loki/api/v1/query_range", url.Values{"query": []string{q}, "start": []string{strconv.FormatInt(start.UnixNano(), 10)}, "end": []string{strconv.FormatInt(end.UnixNano(), 10)}, "limit": []string{strconv.Itoa(req.Limit)}, "direction": []string{"BACKWARD"}})
		if e != nil {
			return nil, fmt.Errorf("查询 Loki 日志失败: %w", e)
		}
		responses = append(responses, r)
	}
	return NormalizeLines(responses, req.Limit), nil
}
