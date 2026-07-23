package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestServerModel(t *testing.T) {
	s := Server{
		Name:       "web-01",
		Host:       "10.0.0.1",
		Port:       9527,
		SSHHost:    "10.0.0.1",
		SSHPort:    22,
		SSHUser:    "root",
		SSHAuthType: "password",
		Status:     "offline",
	}

	if s.Name != "web-01" {
		t.Errorf("expected Name 'web-01', got '%s'", s.Name)
	}
	if s.Status != "offline" {
		t.Errorf("expected Status 'offline', got '%s'", s.Status)
	}
	if s.Port != 9527 {
		t.Errorf("expected Port 9527, got %d", s.Port)
	}
}

func TestSiteModel(t *testing.T) {
	s := Site{
		ServerID: 1,
		Domain:   "example.com",
		Port:     80,
		RootPath: "/var/www/example",
		Managed:  true,
	}

	if s.Domain != "example.com" {
		t.Errorf("expected Domain 'example.com', got '%s'", s.Domain)
	}
	if !s.Managed {
		t.Error("expected Managed to be true")
	}
	if s.SSLEnabled {
		t.Error("expected SSLEnabled to be false by default")
	}
}

func TestSiteUpstreamJSON(t *testing.T) {
	upstream := UpstreamConfig{
		Servers: []UpstreamServer{
			{Host: "127.0.0.1", Port: 3000},
		},
	}
	data, err := json.Marshal(upstream)
	if err != nil {
		t.Fatal(err)
	}
	var decoded UpstreamConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Servers) != 1 || decoded.Servers[0].Port != 3000 {
		t.Error("upstream JSON roundtrip failed")
	}
}

func TestCertModel(t *testing.T) {
	now := time.Now()
	c := Cert{
		SiteID:       1,
		Domains:      `["example.com","www.example.com"]`,
		Provider:     "acme.sh",
		CertPath:     "/etc/ssl/example.com/cert.pem",
		KeyPath:      "/etc/ssl/example.com/key.pem",
		FullchainPath: "/etc/ssl/example.com/fullchain.pem",
		ValidFrom:    now,
		ValidTo:      now.Add(90 * 24 * time.Hour),
		Status:       "issued",
		Challenge:    "http",
	}

	if c.Status != "issued" {
		t.Errorf("expected Status 'issued', got '%s'", c.Status)
	}
	if c.Provider != "acme.sh" {
		t.Errorf("expected Provider 'acme.sh', got '%s'", c.Provider)
	}
	if c.ValidTo.Before(c.ValidFrom) {
		t.Error("ValidTo should be after ValidFrom")
	}
}

func TestCertIsExpiring(t *testing.T) {
	now := time.Now()
	c := Cert{
		ValidFrom: now.Add(-60 * 24 * time.Hour),
		ValidTo:   now.Add(20 * 24 * time.Hour), // 20 days left
	}
	if days := int(time.Until(c.ValidTo).Hours() / 24); days >= 30 {
		t.Errorf("cert expiring in %d days should be < 30", days)
	}
}

func TestAuditLogModel(t *testing.T) {
	detail := map[string]string{"domain": "example.com"}
	detailJSON, _ := json.Marshal(detail)

	a := AuditLog{
		Action:       "create",
		ResourceType: "site",
		ResourceID:   1,
		Detail:       string(detailJSON),
	}

	if a.Action != "create" {
		t.Errorf("expected Action 'create', got '%s'", a.Action)
	}
	if a.ResourceType != "site" {
		t.Errorf("expected ResourceType 'site', got '%s'", a.ResourceType)
	}

	var decoded map[string]string
	if err := json.Unmarshal([]byte(a.Detail), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["domain"] != "example.com" {
		t.Error("detail JSON roundtrip failed")
	}
}
