package cluster

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	clusterservice "github.com/cylism/cylism-manager/internal/service/cluster"
)

func TestValidateSpeedTest(t *testing.T) {
	valid := speedTestRequest{SourceID: 1, TargetID: 2, Address: "tailscale", Protocol: "udp", Direction: "reverse"}
	if err := validateSpeedTest(valid); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*speedTestRequest){
		func(r *speedTestRequest) { r.TargetID = 1 },
		func(r *speedTestRequest) { r.Address = "192.0.2.1" },
		func(r *speedTestRequest) { r.Protocol = "icmp" },
		func(r *speedTestRequest) { r.Direction = "both" },
	} {
		request := valid
		mutate(&request)
		if err := validateSpeedTest(request); err == nil {
			t.Errorf("accepted invalid request: %+v", request)
		}
	}
}

func TestParseSpeedTestOutput(t *testing.T) {
	output := []byte(`{"end":{"sum_sent":{"bits_per_second":1000000,"retransmits":9},"sum_received":{"bits_per_second":724000,"lost_packets":692,"packets":2501,"lost_percent":27.67}}}`)
	tcp, err := parseSpeedTestOutput(output, "tcp")
	if err != nil || tcp.ReceiverMbps != 0.724 || tcp.Retransmits != 9 || tcp.LostPackets != 0 {
		t.Fatalf("TCP result = %+v, %v", tcp, err)
	}
	udp, err := parseSpeedTestOutput(output, "udp")
	if err != nil || udp.LostPackets != 692 || udp.TotalPackets != 2501 || udp.LossPercent != 27.67 {
		t.Fatalf("UDP result = %+v, %v", udp, err)
	}
	if _, err := parseSpeedTestOutput([]byte(`{"error":"connection refused"}`), "tcp"); err == nil {
		t.Fatal("accepted iperf3 error")
	}
	zero, err := parseSpeedTestOutput([]byte(`{"end":{"sum_received":{"bits_per_second":0}}}`), "tcp")
	if err != nil || zero.ReceiverMbps != 0 {
		t.Fatalf("zero throughput result = %+v, %v", zero, err)
	}
}

func TestSpeedTestRejectsSameNodeBeforeSSH(t *testing.T) {
	router, st := newClusterTestRouter(t)
	if err := st.CreateServer(&model.Server{Name: "node", Host: "192.0.2.10"}); err != nil {
		t.Fatal(err)
	}
	router.POST("/api/servers/speed-test", NewServerHandler(make([]byte, 32), clusterservice.NewService(st, nil)).SpeedTest)
	request := httptest.NewRequest(http.MethodPost, "/api/servers/speed-test", strings.NewReader(`{"source_id":1,"target_id":1,"address":"registered","protocol":"tcp","direction":"forward"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "两台不同") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestSpeedDependencyInstallCommand(t *testing.T) {
	for _, test := range []struct {
		status speedDependencies
		want   string
		avoid  string
	}{
		{speedDependencies{TimeoutInstalled: true}, "install -y iperf3", "install -y coreutils"},
		{speedDependencies{Iperf3Installed: true}, "install -y coreutils", "install -y iperf3"},
		{speedDependencies{}, "install -y iperf3 coreutils", ""},
	} {
		command := speedDependencyInstallCommand(test.status)
		if !strings.Contains(command, test.want) || test.avoid != "" && strings.Contains(command, test.avoid) {
			t.Fatalf("unexpected install command: %s", command)
		}
		for _, part := range []string{"sudo -n", "apt-get", "dnf", "yum", "apk"} {
			if !strings.Contains(command, part) {
				t.Fatalf("install command missing %s", part)
			}
		}
	}
}

func TestInstallSpeedDependenciesRejectsUnknownServer(t *testing.T) {
	router, st := newClusterTestRouter(t)
	router.POST("/api/servers/:id/speed-test/dependencies/install", NewServerHandler(make([]byte, 32), clusterservice.NewService(st, nil)).InstallSpeedDependencies)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/servers/42/speed-test/dependencies/install", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}
