package monitoring

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDashboardQueriesAllDefaultPanels(t *testing.T) {
	called := 0
	data, err := Dashboard(context.Background(), "1h", time.Unix(1000, 0), func(_ context.Context, _ string, _ url.Values) (interface{}, error) {
		called++
		return map[string]interface{}{"ok": true}, nil
	})
	if err != nil || called != 4 || len(data) != 4 {
		t.Fatalf("dashboard result=%v calls=%d err=%v", data, called, err)
	}
}

func TestResolveRangeAndValues(t *testing.T) {
	spec, ok := ResolveRange("6h")
	if !ok || spec.Window != 6*time.Hour || spec.Step != 2*time.Minute {
		t.Fatalf("unexpected range: %+v %v", spec, ok)
	}
	values := RangeValues("up", spec, time.Unix(1000, 0))
	if values.Get("query") != "up" || values.Get("start") != "-20600" || values.Get("end") != "1000" || values.Get("step") != "120" {
		t.Fatalf("unexpected query values: %#v", values)
	}
}

func TestDiskGrowthQueriesConstrainNode(t *testing.T) {
	queries := DiskGrowthQueries("1h", "node-a")
	if len(queries) != 2 || !strings.Contains(queries[0].Query, `node="node-a"`) {
		t.Fatalf("unexpected queries: %#v", queries)
	}
}
