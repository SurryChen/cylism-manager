package agent

import (
	"context"
	"testing"

	pb "github.com/cylism/cylism-manager/api/proto/agent"
)

func TestPing(t *testing.T) {
	srv := NewServer("1.0.0")
	resp, err := srv.Ping(context.Background(), &pb.PingRequest{})
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if !resp.Ok {
		t.Error("expected Ping Ok=true")
	}
	if resp.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", resp.Version)
	}
}

func TestSystemInfo(t *testing.T) {
	srv := NewServer("1.0.0")
	resp, err := srv.SystemInfo(context.Background(), &pb.SystemInfoRequest{})
	if err != nil {
		t.Fatalf("SystemInfo: %v", err)
	}
	if resp.Os == "" {
		t.Error("expected non-empty OS")
	}
	if resp.Arch == "" {
		t.Error("expected non-empty Arch")
	}
}

func TestAcmeDetect(t *testing.T) {
	srv := NewServer("1.0.0")
	resp, err := srv.AcmeDetect(context.Background(), &pb.AcmeDetectRequest{})
	if err != nil {
		t.Fatalf("AcmeDetect: %v", err)
	}
	// acme.sh may or may not be installed — just verify no error
	_ = resp.Installed
}
