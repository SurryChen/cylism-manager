package logging

import (
	"testing"
	"time"
)

func TestValidateAndResolveQuery(t *testing.T) {
	r := QueryRequest{Range: "", Limit: 0}
	if err := ValidateQuery(&r); err != nil {
		t.Fatal(err)
	}
	if r.Range != "1h" || r.Limit != DefaultLimit {
		t.Fatalf("defaults not applied: %+v", r)
	}
	now := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	start, end, err := ResolveBounds(r, now)
	if err != nil || !end.Equal(now) || !start.Equal(now.Add(-time.Hour)) {
		t.Fatalf("unexpected bounds %v %v %v", start, end, err)
	}
}

func TestBuildLogQLQueriesBoundsAndEscapes(t *testing.T) {
	r := QueryRequest{Namespace: "default", Keyword: `error AND "disk full" OR timeout`}
	queries, err := BuildLogQLQueries(r)
	if err != nil || len(queries) != 2 {
		t.Fatalf("queries=%v err=%v", queries, err)
	}
	if queries[0] == queries[1] || queries[0] == "" {
		t.Fatalf("unexpected queries: %#v", queries)
	}
	tooLong := QueryRequest{Keyword: "a"}
	for i := 0; i < MaxQueryTerms+1; i++ {
		tooLong.Keyword += " AND a"
	}
	if _, err := BuildLogQLQueries(tooLong); err == nil {
		t.Fatal("expected term limit error")
	}
}

func TestNormalizeLinesDeduplicatesSortsAndLimits(t *testing.T) {
	responses := []Response{{Streams: []Stream{{Labels: map[string]string{"pod": "api"}, Values: [][]string{{"2", "new"}, {"1", "old"}, {"2", "new"}}}}}}
	lines := NormalizeLines(responses, 1)
	if len(lines) != 1 || lines[0].Line != "new" || lines[0].Timestamp == "2" {
		t.Fatalf("unexpected normalized lines: %#v", lines)
	}
}
