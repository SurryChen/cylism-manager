package store

import (
	"errors"
	"path/filepath"
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

func TestApplicationDeploymentTemplateMigrationRemovesLegacySingleTemplateIndex(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "store.db")
	st, err := New(dsn)
	if err != nil {
		t.Fatal(err)
	}
	app := &model.Application{ProjectID: 1, EnvironmentID: 1, Name: "order-api", WorkloadKind: "deployment", CreatedBy: 1}
	if err := st.CreateApplication(app); err != nil {
		t.Fatal(err)
	}
	legacy := &model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "legacy", Enabled: true, Spec: "{}", Revision: 1}
	if err := st.DB().Create(legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Model(legacy).Updates(map[string]interface{}{"name": "", "enabled": false}).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Exec("CREATE UNIQUE INDEX idx_application_deployment_templates_application_id ON application_deployment_templates(application_id)").Error; err != nil {
		t.Fatal(err)
	}

	migrated, err := New(dsn)
	if err != nil {
		t.Fatalf("migrate legacy template database: %v", err)
	}
	templates, err := migrated.ListApplicationDeploymentTemplates(app.ID)
	if err != nil || len(templates) != 1 || templates[0].Name != "默认模板" || !templates[0].Enabled {
		t.Fatalf("legacy template was not backfilled: %#v err=%v", templates, err)
	}
	if err := migrated.CreateApplicationDeploymentTemplate(&model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "灰度模板", Enabled: true, Spec: "{}"}, false); err != nil {
		t.Fatalf("legacy single-template index should have been removed: %v", err)
	}
}

func TestApplicationStackSchemaIsRemoved(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "store.db")
	st, err := New(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Exec("ALTER TABLE applications ADD COLUMN stack_template_id integer").Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Exec("CREATE INDEX idx_applications_stack_template_id ON applications(stack_template_id)").Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Exec("CREATE TABLE application_stack_templates (id integer primary key, environment_id integer not null)").Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Exec("CREATE TABLE application_stack_releases (id integer primary key, template_id integer not null)").Error; err != nil {
		t.Fatal(err)
	}

	migrated, err := New(dsn)
	if err != nil {
		t.Fatalf("remove obsolete application stack schema: %v", err)
	}
	if migrated.DB().Migrator().HasTable("application_stack_templates") || migrated.DB().Migrator().HasTable("application_stack_releases") {
		t.Fatal("application stack tables must be removed")
	}
	if migrated.DB().Migrator().HasColumn("applications", "stack_template_id") {
		t.Fatal("applications.stack_template_id must be removed")
	}
}

func TestObsoleteAssistantSchemaIsRemoved(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "store.db")
	st, err := New(dsn)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{
		"assistant_providers",
		"assistant_conversations",
		"assistant_messages",
		"assistant_runtime_migrations",
	} {
		if err := st.DB().Exec("CREATE TABLE " + table + " (id integer primary key)").Error; err != nil {
			t.Fatalf("create legacy table %s: %v", table, err)
		}
	}
	if err := st.DB().Exec("INSERT INTO system_configs (key, value) VALUES (?, ?)", "assistant_default_provider_id", "1").Error; err != nil {
		t.Fatal(err)
	}

	migrated, err := New(dsn)
	if err != nil {
		t.Fatalf("remove obsolete assistant schema: %v", err)
	}
	for _, table := range []string{
		"assistant_providers",
		"assistant_conversations",
		"assistant_messages",
		"assistant_runtime_migrations",
	} {
		if migrated.DB().Migrator().HasTable(table) {
			t.Fatalf("legacy assistant table %s must be removed", table)
		}
	}
	var count int64
	if err := migrated.DB().Table("system_configs").Where("key = ?", "assistant_default_provider_id").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("legacy assistant default provider config must be removed")
	}
}

func TestServerCRUD(t *testing.T) {
	s := setupTestDB(t)

	// Create
	server := &model.Server{
		Name:        "web-01",
		Host:        "10.0.0.1",
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
		ServerID: server.ID,
		Domain:   "example.com",
		Port:     80,
		RootPath: "/var/www/example",
		Managed:  true,
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

func TestApplicationReleaseCRUD(t *testing.T) {
	st := setupTestDB(t)
	project := &model.Project{Name: "commerce", OwnerID: 1}
	if err := st.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	environment := &model.Environment{ProjectID: project.ID, Name: "production", Namespace: "commerce-prod"}
	if err := st.CreateEnvironment(environment); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	application := &model.Application{
		ProjectID: project.ID, EnvironmentID: environment.ID, Name: "order-api", WorkloadKind: "deployment", CreatedBy: 1,
	}
	if err := st.CreateApplication(application); err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}
	if err := st.CreateApplicationEndpoint(&model.ApplicationEndpoint{
		ApplicationID: application.ID, Exposure: "public", Domain: "api.example.com", Path: "/", ServicePort: 80, TLSEnabled: true,
	}); err != nil {
		t.Fatalf("CreateApplicationEndpoint: %v", err)
	}

	release := &model.Release{
		ApplicationID: application.ID, Sequence: 1, Image: "registry.example.com/order-api:1.0.0",
		DesiredSpec: `{"replicas":2}`, Status: model.ReleaseStatusDraft, CreatedBy: 1,
	}
	if err := st.CreateRelease(release); err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	operation := &model.ReleaseOperation{ReleaseID: release.ID, Step: "preflight", Status: model.ReleaseOperationRunning}
	if err := st.CreateReleaseOperation(operation); err != nil {
		t.Fatalf("CreateReleaseOperation: %v", err)
	}

	got, err := st.GetApplication(application.ID)
	if err != nil {
		t.Fatalf("GetApplication: %v", err)
	}
	if got.Environment.Namespace != "commerce-prod" || len(got.Endpoints) != 1 {
		t.Fatalf("expected loaded environment and endpoint, got %+v", got)
	}
	releases, err := st.ListReleases(application.ID)
	if err != nil || len(releases) != 1 || releases[0].Sequence != 1 {
		t.Fatalf("expected one release, got %+v, err=%v", releases, err)
	}
	operations, err := st.ListReleaseOperations(release.ID)
	if err != nil || len(operations) != 1 || operations[0].Step != "preflight" {
		t.Fatalf("expected preflight operation, got %+v, err=%v", operations, err)
	}
}

func TestListResourceReferencesIncludesTemplatesAndReleaseSnapshots(t *testing.T) {
	st := setupTestDB(t)
	project := &model.Project{Name: "commerce", OwnerID: 1}
	if err := st.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	environment := &model.Environment{ProjectID: project.ID, Name: "production", Namespace: "commerce-prod"}
	if err := st.CreateEnvironment(environment); err != nil {
		t.Fatal(err)
	}
	app := &model.Application{ProjectID: project.ID, EnvironmentID: environment.ID, Name: "edge", WorkloadKind: "deployment", CreatedBy: 1}
	if err := st.CreateApplication(app); err != nil {
		t.Fatal(err)
	}
	spec := `{"file_mounts":[{"source_type":"secret","source_name":"edge-tls","key":"tls.crt","mount_path":"/etc/tls/tls.crt"}]}`
	if err := st.CreateApplicationDeploymentTemplate(&model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "production", Enabled: true, Spec: spec}, true); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateRelease(&model.Release{ApplicationID: app.ID, Sequence: 1, Image: "example.com/edge:1", DesiredSpec: spec, Status: model.ReleaseStatusSucceeded, CreatedBy: 1}); err != nil {
		t.Fatal(err)
	}
	references, err := st.ListResourceReferences("commerce-prod", "secret", "edge-tls")
	if err != nil {
		t.Fatal(err)
	}
	if len(references) != 2 || references[0].ApplicationName != "edge" {
		t.Fatalf("expected template and release references, got %#v", references)
	}
	missing, err := st.ListResourceReferences("commerce-prod", "configmap", "edge-tls")
	if err != nil || len(missing) != 0 {
		t.Fatalf("expected no configmap references, got %#v err=%v", missing, err)
	}
}

func TestProjectAndEnvironmentCRUD(t *testing.T) {
	st := setupTestDB(t)
	project := &model.Project{Name: "commerce", Description: "订单服务", OwnerID: 1}
	if err := st.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	environment := &model.Environment{ProjectID: project.ID, Name: "staging", Namespace: "commerce-staging"}
	if err := st.CreateEnvironment(environment); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}

	projects, err := st.ListProjects()
	if err != nil || len(projects) != 1 || len(projects[0].Environments) != 1 {
		t.Fatalf("ListProjects should preload environments, got %+v, err=%v", projects, err)
	}

	project.Description = "订单服务与部署环境"
	if err := st.UpdateProject(project); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	environment.Namespace = "commerce-preview"
	if err := st.UpdateEnvironment(environment); err != nil {
		t.Fatalf("UpdateEnvironment: %v", err)
	}

	if err := st.DeleteEnvironment(environment.ID); err != nil {
		t.Fatalf("DeleteEnvironment: %v", err)
	}
	if err := st.DeleteProject(project.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
}

func TestEnvironmentNamespaceIsGloballyUnique(t *testing.T) {
	st := setupTestDB(t)
	first := &model.Environment{ProjectID: 1, Name: "production", Namespace: "commerce"}
	if err := st.CreateEnvironment(first); err != nil {
		t.Fatalf("create first environment: %v", err)
	}
	second := &model.Environment{ProjectID: 2, Name: "production", Namespace: "commerce"}
	err := st.CreateEnvironment(second)
	var conflict *NamespaceConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected NamespaceConflictError, got %v", err)
	}
	if conflict.EnvironmentID != first.ID || conflict.ProjectID != first.ProjectID {
		t.Fatalf("unexpected conflict: %#v", conflict)
	}
}

func TestReconcileEnvironmentNamespaceUniquenessWaitsForLegacyConflicts(t *testing.T) {
	st := setupTestDB(t)
	if err := st.db.Exec("DROP INDEX IF EXISTS idx_environments_namespace_unique").Error; err != nil {
		t.Fatal(err)
	}
	if err := st.db.Exec("INSERT INTO environments (project_id, name, namespace, created_at, updated_at) VALUES (1, 'one', 'shared', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP), (2, 'two', 'shared', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)").Error; err != nil {
		t.Fatal(err)
	}
	if err := st.ReconcileEnvironmentNamespaceUniqueness(); err != nil {
		t.Fatalf("reconcile with legacy conflicts: %v", err)
	}
	conflicts, err := st.ListEnvironmentNamespaceConflicts()
	if err != nil || len(conflicts) != 1 || conflicts[0].Namespace != "shared" || len(conflicts[0].Environments) != 2 {
		t.Fatalf("unexpected conflicts: %#v, err=%v", conflicts, err)
	}
	if err := st.db.Where("project_id = ?", 2).Delete(&model.Environment{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.ReconcileEnvironmentNamespaceUniqueness(); err != nil {
		t.Fatalf("reconcile resolved conflicts: %v", err)
	}
	if err := st.db.Exec("INSERT INTO environments (project_id, name, namespace, created_at, updated_at) VALUES (3, 'three', 'shared', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)").Error; err == nil {
		t.Fatal("expected unique index to reject duplicate namespace")
	}
}

func TestBackfillManagedDomainEnvironmentWhenNamespaceHasSingleOwner(t *testing.T) {
	st := setupTestDB(t)
	environment := &model.Environment{ProjectID: 1, Name: "production", Namespace: "commerce-prod"}
	if err := st.CreateEnvironment(environment); err != nil {
		t.Fatal(err)
	}
	domain := &model.ManagedDomain{Hostname: "api.example.com", Namespace: environment.Namespace}
	if err := st.CreateManagedDomain(domain); err != nil {
		t.Fatal(err)
	}
	if err := st.backfillManagedDomainEnvironments(); err != nil {
		t.Fatal(err)
	}
	updated, err := st.GetManagedDomain(domain.ID)
	if err != nil || updated.EnvironmentID != environment.ID {
		t.Fatalf("expected domain to bind environment %d, got %#v err=%v", environment.ID, updated, err)
	}
}

func TestApplicationEndpointsSupportIndependentUpdatesAndExactRouteExclusion(t *testing.T) {
	st := setupTestDB(t)
	first := &model.ApplicationEndpoint{ApplicationID: 1, DomainID: 7, Exposure: "public", Domain: "api.example.com", Path: "/", ServicePort: 80}
	if err := st.CreateApplicationEndpoint(first); err != nil {
		t.Fatal(err)
	}
	second := &model.ApplicationEndpoint{ApplicationID: 1, DomainID: 8, Exposure: "public", Domain: "admin.example.com", Path: "/", ServicePort: 80}
	if err := st.CreateApplicationEndpoint(second); err != nil {
		t.Fatal(err)
	}
	third := &model.ApplicationEndpoint{ApplicationID: 2, DomainID: 7, Exposure: "public", Domain: "api.example.com", Path: "/health", ServicePort: 80}
	if err := st.CreateApplicationEndpoint(third); err != nil {
		t.Fatal(err)
	}

	endpoints, err := st.ListApplicationEndpoints(1)
	if err != nil || len(endpoints) != 2 || endpoints[0].ID != first.ID || endpoints[1].ID != second.ID {
		t.Fatalf("expected two ordered endpoints, got %#v err=%v", endpoints, err)
	}
	second.Path = "/admin"
	if err := st.UpdateApplicationEndpoint(second); err != nil {
		t.Fatal(err)
	}
	updated, err := st.GetApplicationEndpoint(1, second.ID)
	if err != nil || updated.Path != "/admin" {
		t.Fatalf("expected independently updated endpoint, got %#v err=%v", updated, err)
	}

	count, err := st.CountApplicationEndpointRoute(7, "/", first.ID)
	if err != nil || count != 0 {
		t.Fatalf("expected current endpoint route to be excluded, count=%d err=%v", count, err)
	}
	count, err = st.CountApplicationEndpointRoute(7, "/", second.ID)
	if err != nil || count != 1 {
		t.Fatalf("expected another endpoint in same application to conflict, count=%d err=%v", count, err)
	}
	count, err = st.CountApplicationEndpointRoute(7, "/health", first.ID)
	if err != nil || count != 1 {
		t.Fatalf("expected conflicting route, count=%d err=%v", count, err)
	}
	if err := st.DeleteApplicationEndpoint(1, first.ID); err != nil {
		t.Fatal(err)
	}
	endpoints, err = st.ListApplicationEndpoints(1)
	if err != nil || len(endpoints) != 1 || endpoints[0].ID != second.ID {
		t.Fatalf("expected only second endpoint after delete, got %#v err=%v", endpoints, err)
	}
}

func TestApplicationEndpointRouteCountIgnoresMetadataOnlyBindings(t *testing.T) {
	st := setupTestDB(t)
	metadata := &model.ApplicationEndpoint{ApplicationID: 1, DomainID: 7, Exposure: "public", Domain: "proxy.example.com", Path: "/", ServicePort: 8443, IngressEnabled: false, IngressMode: "metadata"}
	if err := st.CreateApplicationEndpoint(metadata); err != nil {
		t.Fatal(err)
	}
	count, err := st.CountApplicationEndpointRoute(7, "/", 0)
	if err != nil || count != 0 {
		t.Fatalf("expected metadata-only binding to leave route available, count=%d err=%v", count, err)
	}
}
