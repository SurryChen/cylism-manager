package monitoring

import (
	"context"
	"net/url"
	"testing"
	"time"
)

type readyFake bool

func (r readyFake) Ready(context.Context) bool { return bool(r) }

func TestQueryServiceBuildsBoundedRange(t *testing.T) {
	called := false
	s := NewQueryService(func(_ context.Context, path string, values url.Values) (interface{}, error) {
		called = true
		if path != "/api/v1/query_range" || values.Get("step") != "120" {
			t.Fatalf("unexpected %s %#v", path, values)
		}
		return map[string]interface{}{}, nil
	}, readyFake(true))
	if _, err := s.QueryRange(context.Background(), "up", "6h", time.Unix(1000, 0)); err != nil || !called {
		t.Fatalf("query failed: %v", err)
	}
}
