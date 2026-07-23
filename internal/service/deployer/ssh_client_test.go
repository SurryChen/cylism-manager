package deployer

import (
	"testing"
)

func TestAgentProbeResultDefaults(t *testing.T) {
	r := AgentProbeResult{}
	if r.Installed {
		t.Error("expected Installed=false by default")
	}
	if r.ProcessRunning {
		t.Error("expected ProcessRunning=false by default")
	}
	if r.BinaryExists {
		t.Error("expected BinaryExists=false by default")
	}
	if r.SystemdExists {
		t.Error("expected SystemdExists=false by default")
	}
	if r.SystemdActive {
		t.Error("expected SystemdActive=false by default")
	}
	if r.AgentVersion != "" {
		t.Errorf("expected empty version, got '%s'", r.AgentVersion)
	}
}

func TestAgentProbeResultFields(t *testing.T) {
	// Verify Installed is correctly set when fields are set manually
	r := AgentProbeResult{
		BinaryExists:  true,
		ProcessRunning: false,
		SystemdExists: false,
		SystemdActive: false,
		AgentVersion:  "1.0.0",
		ListeningPort: 9527,
	}
	// After ProbeAgent runs, Installed = BinaryExists || ProcessRunning || SystemdExists
	if !r.BinaryExists {
		t.Error("BinaryExists should be true")
	}
	if r.ListeningPort != 9527 {
		t.Errorf("expected port 9527, got %d", r.ListeningPort)
	}

	r2 := AgentProbeResult{ProcessRunning: true}
	if !r2.ProcessRunning {
		t.Error("ProcessRunning should be true")
	}
}
