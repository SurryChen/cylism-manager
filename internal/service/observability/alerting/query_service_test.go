package alerting

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestSilenceMatcherUsesFrontendJSONFieldNames(t *testing.T) {
	raw, err := json.Marshal(Matcher{Name: "alertname", Value: "NodeDown", IsEqual: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"name":"alertname","value":"NodeDown","isRegex":false,"isEqual":true}` {
		t.Fatalf("unexpected matcher JSON: %s", raw)
	}
}

func TestQueryServiceDelegatesOverviewAndSilences(t *testing.T) {
	client := NewClient(func(_ context.Context, method, path string, _ interface{}, out interface{}) error {
		if method == http.MethodGet && path == "/api/v2/alerts" {
			*(out.(*[]Alert)) = []Alert{{Status: AlertStatus{State: "active"}}}
		}
		return nil
	})
	s := NewQueryService(client, func() bool { return true })
	o, err := s.Overview(context.Background(), nil)
	if err != nil || o.Firing != 1 {
		t.Fatalf("overview=%#v err=%v", o, err)
	}
}
func TestQueryServiceRejectsWhenNotReady(t *testing.T) {
	s := NewQueryService(NewClient(nil), func() bool { return false })
	if _, err := s.Silences(context.Background()); err == nil {
		t.Fatal("expected readiness error")
	}
}
func TestResolvedCacheDeduplicatesAndBounds(t *testing.T) {
	c := NewResolvedCache(1)
	c.Add([]Alert{{Fingerprint: "a", Status: AlertStatus{State: "resolved"}}})
	c.Add([]Alert{{Fingerprint: "a", Status: AlertStatus{State: "resolved"}}, {Fingerprint: "b", Status: AlertStatus{State: "firing"}}})
	if got := c.List(); len(got) != 1 || got[0].Fingerprint != "a" {
		t.Fatalf("unexpected cache: %#v", got)
	}
}
