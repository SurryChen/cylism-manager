package store

import (
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

func setupTestDB(t *testing.T) *Store {
	t.Helper()
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	return s
}

func TestServerCRUD(t *testing.T) {
	s := setupTestDB(t)

	// Create
	server := &model.Server{
		Name:        "web-01",
		Host:        "10.0.0.1",
		Port:        9527,
		SSHHost:     "10.0.0.1",
		SSHPort:     22,
		SSHUser:     "root",
		SSHAuthType: "password",
		Status:      "offline",
	}
	if err := s.CreateServer(server); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if server.ID == 0 {
		t.Error("expected non-zero ID after create")
	}

	// Read
	got, err := s.GetServer(server.ID)
	if err != nil {
		t.Fatalf("GetServer: %v", err)
	}
	if got.Name != "web-01" {
		t.Errorf("expected Name 'web-01', got '%s'", got.Name)
	}

	// List
	servers, err := s.ListServers()
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if len(servers) != 1 {
		t.Errorf("expected 1 server, got %d", len(servers))
	}

	// Update
	got.Status = "online"
	if err := s.UpdateServer(got); err != nil {
		t.Fatalf("UpdateServer: %v", err)
	}
	got2, _ := s.GetServer(server.ID)
	if got2.Status != "online" {
		t.Errorf("expected Status 'online', got '%s'", got2.Status)
	}

	// Delete
	if err := s.DeleteServer(server.ID); err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}
	_, err = s.GetServer(server.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestSiteCRUD(t *testing.T) {
	st := setupTestDB(t)

	// Need a server first
	server := &model.Server{Name: "s1", Host: "10.0.0.1"}
	st.CreateServer(server)

	// Create
	site := &model.Site{
		ServerID:   server.ID,
		Domain:     "example.com",
		Port:       80,
		RootPath:   "/var/www/example",
		Managed:    true,
	}
	if err := st.CreateSite(site); err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	// Read
	got, err := st.GetSite(site.ID)
	if err != nil {
		t.Fatalf("GetSite: %v", err)
	}
	if got.Domain != "example.com" {
		t.Errorf("expected Domain 'example.com', got '%s'", got.Domain)
	}

	// List by server
	sites, err := st.ListSitesByServer(server.ID)
	if err != nil {
		t.Fatalf("ListSitesByServer: %v", err)
	}
	if len(sites) != 1 {
		t.Errorf("expected 1 site, got %d", len(sites))
	}

	// Update
	got.RootPath = "/var/www/new"
	if err := st.UpdateSite(got); err != nil {
		t.Fatalf("UpdateSite: %v", err)
	}

	// Delete
	if err := st.DeleteSite(site.ID); err != nil {
		t.Fatalf("DeleteSite: %v", err)
	}
}

func TestSiteDuplicateDomain(t *testing.T) {
	st := setupTestDB(t)
	server := &model.Server{Name: "s1", Host: "10.0.0.1"}
	st.CreateServer(server)

	s1 := &model.Site{ServerID: server.ID, Domain: "dup.com", Port: 80}
	if err := st.CreateSite(s1); err != nil {
		t.Fatal(err)
	}

	s2 := &model.Site{ServerID: server.ID, Domain: "dup.com", Port: 443}
	err := st.CreateSite(s2)
	if err == nil {
		t.Error("expected duplicate domain error")
	}
}

func TestCertCRUD(t *testing.T) {
	st := setupTestDB(t)
	server := &model.Server{Name: "s1", Host: "10.0.0.1"}
	st.CreateServer(server)
	site := &model.Site{ServerID: server.ID, Domain: "example.com", Port: 80}
	st.CreateSite(site)

	now := time.Now()
	cert := &model.Cert{
		SiteID:        site.ID,
		Domains:       `["example.com"]`,
		Provider:      "acme.sh",
		CertPath:      "/etc/ssl/cert.pem",
		KeyPath:       "/etc/ssl/key.pem",
		FullchainPath: "/etc/ssl/fullchain.pem",
		ValidFrom:     now,
		ValidTo:       now.Add(90 * 24 * time.Hour),
		Status:        "issued",
		Challenge:     "http",
	}
	if err := st.CreateCert(cert); err != nil {
		t.Fatalf("CreateCert: %v", err)
	}

	got, err := st.GetCert(cert.ID)
	if err != nil {
		t.Fatalf("GetCert: %v", err)
	}
	if got.Status != "issued" {
		t.Errorf("expected Status 'issued', got '%s'", got.Status)
	}

	// Get by site
	gotBySite, err := st.GetCertBySite(site.ID)
	if err != nil {
		t.Fatalf("GetCertBySite: %v", err)
	}
	if gotBySite.ID != cert.ID {
		t.Error("GetCertBySite returned wrong cert")
	}

	// List expiring
	expiring, err := st.ListExpiringCerts(30)
	if err != nil {
		t.Fatalf("ListExpiringCerts: %v", err)
	}
	if len(expiring) != 0 {
		t.Errorf("expected 0 expiring (90 days), got %d", len(expiring))
	}

	// Update
	got.Status = "renewing"
	if err := st.UpdateCert(got); err != nil {
		t.Fatalf("UpdateCert: %v", err)
	}

	// Delete
	if err := st.DeleteCert(cert.ID); err != nil {
		t.Fatalf("DeleteCert: %v", err)
	}
}

func TestAuditLogCRUD(t *testing.T) {
	st := setupTestDB(t)

	entry := &model.AuditLog{
		Action:       "create",
		ResourceType: "site",
		ResourceID:   1,
		Detail:       `{"domain":"example.com"}`,
	}
	if err := st.CreateAuditLog(entry); err != nil {
		t.Fatalf("CreateAuditLog: %v", err)
	}
	if entry.ID == 0 {
		t.Error("expected non-zero ID")
	}

	// List
	logs, total, err := st.ListAuditLogs("", "", 10, 0)
	if err != nil {
		t.Fatalf("ListAuditLogs: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total 1, got %d", total)
	}
	if len(logs) != 1 {
		t.Errorf("expected 1 log, got %d", len(logs))
	}

	// Filter by resource type
	logs2, total2, _ := st.ListAuditLogs("cert", "", 10, 0)
	if total2 != 0 {
		t.Errorf("expected 0 cert logs, got %d", total2)
	}
	_ = logs2
}
