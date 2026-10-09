package bootstrap

import "testing"

func TestClusterServiceProxyURL(t *testing.T) {
	got, err := clusterServiceProxyURL("https://agent.example:9443/", "http://victoria-metrics.monitoring.svc:8428")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://agent.example:9443/api/v1/namespaces/monitoring/services/http:victoria-metrics:8428/proxy"
	if got != want {
		t.Fatalf("proxy URL = %q, want %q", got, want)
	}
	for _, address := range []string{"https://victoria-metrics.monitoring.svc:8428", "http://localhost:8428", "http://victoria-metrics.monitoring.svc"} {
		if _, err := clusterServiceProxyURL("https://agent.example:9443", address); err == nil {
			t.Errorf("accepted non-Service address %q", address)
		}
	}
}
