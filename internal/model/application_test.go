package model

import "testing"

func TestApplicationCapabilitiesNormalizeAndPersist(t *testing.T) {
	application := &Application{}
	if err := application.SetCapabilities([]string{"Config-Editor", "config-editor", "metrics"}); err != nil {
		t.Fatal(err)
	}
	if len(application.Capabilities) != 2 || application.Capabilities[0] != "config-editor" || application.Capabilities[1] != "metrics" {
		t.Fatalf("unexpected normalized capabilities: %#v", application.Capabilities)
	}
	loaded := &Application{CapabilitiesData: application.CapabilitiesData}
	loaded.LoadCapabilities()
	if len(loaded.Capabilities) != 2 || loaded.Capabilities[0] != "config-editor" {
		t.Fatalf("unexpected loaded capabilities: %#v", loaded.Capabilities)
	}
}
