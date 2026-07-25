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
		SSHHost:     "10.0.0.1",
		SSHPort:     22,
		SSHUser:     "root",
		SSHAuthType: "password",
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
	got.ClusterRole = "worker"
	if err := s.UpdateServer(got); err != nil {
		t.Fatalf("UpdateServer: %v", err)
	}
	got2, _ := s.GetServer(server.ID)
	if got2.ClusterRole != "worker" {
		t.Errorf("expected ClusterRole 'worker', got '%s'", got2.ClusterRole)
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

func TestOperationLogCRUD(t *testing.T) {
	st := setupTestDB(t)

	// Create logs
	log1 := &model.OperationLog{
		ResourceType: "server",
		ResourceID:   1,
		Step:         "正在连接 SSH",
		Status:       "success",
		Detail:       "连接成功",
	}
	if err := st.CreateOperationLog(log1); err != nil {
		t.Fatalf("CreateOperationLog: %v", err)
	}
	if log1.ID == 0 {
		t.Error("expected non-zero ID")
	}

	log2 := &model.OperationLog{
		ResourceType: "server",
		ResourceID:   1,
		Step:         "正在上传 Agent",
		Status:       "running",
	}
	if err := st.CreateOperationLog(log2); err != nil {
		t.Fatalf("CreateOperationLog: %v", err)
	}

	// List by resource
	logs, err := st.ListOperationsByResource("server", 1)
	if err != nil {
		t.Fatalf("ListOperationsByResource: %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(logs))
	}
	// Logs should be ordered by created_at desc
	if logs[0].Step != "正在上传 Agent" {
		t.Errorf("expected first log step '正在上传 Agent', got '%s'", logs[0].Step)
	}

	// List for non-existing resource
	empty, err := st.ListOperationsByResource("server", 999)
	if err != nil {
		t.Fatalf("ListOperationsByResource (empty): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected 0 logs for non-existing resource, got %d", len(empty))
	}

	// Update log status
	log2.Status = "success"
	log2.Detail = "上传完毕"
	if err := st.UpdateOperationLog(log2); err != nil {
		t.Fatalf("UpdateOperationLog: %v", err)
	}

	// Verify update
	logs, _ = st.ListOperationsByResource("server", 1)
	if logs[0].Status != "success" {
		t.Errorf("expected status 'success', got '%s'", logs[0].Status)
	}
	if logs[0].Detail != "上传完毕" {
		t.Errorf("expected detail '上传完毕', got '%s'", logs[0].Detail)
	}
}

func TestDeleteExpiredOperationLogs(t *testing.T) {
	st := setupTestDB(t)

	// Direct insert with old timestamp using raw SQL
	err := st.db.Exec("INSERT INTO operation_logs (resource_type, resource_id, step, status, created_at) VALUES (?, ?, ?, ?, ?)",
		"server", 1, "old step", "success", time.Now().Add(-40*24*time.Hour)).Error
	if err != nil {
		t.Fatalf("insert old log: %v", err)
	}

	// Insert a recent log
	err = st.db.Exec("INSERT INTO operation_logs (resource_type, resource_id, step, status, created_at) VALUES (?, ?, ?, ?, ?)",
		"server", 1, "recent step", "success", time.Now()).Error
	if err != nil {
		t.Fatalf("insert recent log: %v", err)
	}

	// Delete logs older than 30 days
	if err := st.DeleteExpiredOperationLogs(30); err != nil {
		t.Fatalf("DeleteExpiredOperationLogs: %v", err)
	}

	// Only the recent log should remain
	logs, err := st.ListOperationsByResource("server", 1)
	if err != nil {
		t.Fatalf("ListOperationsByResource: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("expected 1 log after cleanup, got %d", len(logs))
	}
	if len(logs) > 0 && logs[0].Step != "recent step" {
		t.Errorf("expected 'recent step', got '%s'", logs[0].Step)
	}
}

func TestDeleteExpiredZeroRetention(t *testing.T) {
	st := setupTestDB(t)

	err := st.db.Exec("INSERT INTO operation_logs (resource_type, resource_id, step, status, created_at) VALUES (?, ?, ?, ?, ?)",
		"server", 1, "test", "success", time.Now().Add(-100*24*time.Hour)).Error
	if err != nil {
		t.Fatalf("insert log: %v", err)
	}

	// retention_days=0 means never delete
	if err := st.DeleteExpiredOperationLogs(0); err != nil {
		t.Fatalf("DeleteExpiredOperationLogs: %v", err)
	}

	logs, _ := st.ListOperationsByResource("server", 1)
	if len(logs) != 1 {
		t.Errorf("expected 1 log (never expire), got %d", len(logs))
	}
}
