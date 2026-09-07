package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// legacyManagedOCIRegistry mirrors the pre-PVC database contract. Its
// data_path column has no default and must be removed during the upgrade.
type legacyManagedOCIRegistry struct {
	ID                   uint   `gorm:"primaryKey"`
	Name                 string `gorm:"size:128;uniqueIndex;not null"`
	Namespace            string `gorm:"size:253;not null"`
	ResourceName         string `gorm:"size:253;uniqueIndex;not null"`
	Endpoint             string `gorm:"size:253;uniqueIndex;not null"`
	RegistryImage        string `gorm:"size:512;not null"`
	DataNode             string `gorm:"size:253;not null"`
	DataPath             string `gorm:"size:1024;not null"`
	InsecureHTTP         bool   `gorm:"not null;default:false"`
	TLSSecretName        string `gorm:"size:253"`
	PullUsername         string `gorm:"size:256;not null"`
	ImageRegistryID      *uint  `gorm:"uniqueIndex"`
	NodeRegistryMirrorID *uint  `gorm:"uniqueIndex"`
	Status               string `gorm:"size:32;not null;default:pending"`
	LastError            string `gorm:"size:512"`
	LastCheckedAt        *time.Time
	CreatedBy            uint `gorm:"index;not null"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (legacyManagedOCIRegistry) TableName() string { return "managed_oci_registries" }

func setupTestDB(t *testing.T) *Store {
	t.Helper()
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	return s
}

func TestManagedOCIRegistryMigrationRemovesLegacyDataPathColumn(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "store.db")
	legacy, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.AutoMigrate(&legacyManagedOCIRegistry{}); err != nil {
		t.Fatalf("create legacy managed registry schema: %v", err)
	}
	legacyRecord := &legacyManagedOCIRegistry{Name: "旧制品库", Namespace: "cylism-system", ResourceName: "legacy-registry", Endpoint: "legacy.registry.internal", RegistryImage: "registry:2", DataNode: "node-a", DataPath: "/data/registry", PullUsername: "legacy-pull", Status: "ready", CreatedBy: 1}
	if err := legacy.Create(legacyRecord).Error; err != nil {
		t.Fatalf("create legacy managed registry record: %v", err)
	}

	migrated, err := New(dsn)
	if err != nil {
		t.Fatalf("migrate legacy managed registry schema: %v", err)
	}
	if migrated.DB().Migrator().HasColumn(&model.ManagedOCIRegistry{}, "data_path") {
		t.Fatal("legacy managed registry data_path column must be removed")
	}
	preserved, err := migrated.GetManagedOCIRegistry(legacyRecord.ID)
	if err != nil || preserved.Name != legacyRecord.Name || preserved.Endpoint != legacyRecord.Endpoint || preserved.PVCName != "cylism-oci-registry-data" {
		t.Fatalf("legacy Registry record was not preserved: %#v err=%v", preserved, err)
	}

	managed := &model.ManagedOCIRegistry{Name: "平台制品库", Namespace: "cylism-system", ResourceName: "cylism-oci-registry", Endpoint: "registry.internal", RegistryImage: "registry:2", DataNode: "node-a", StorageClassName: "local-path", PVCName: "cylism-oci-registry-data", StorageSize: "10Gi", CPURequest: "100m", CPULimit: "500m", MemoryRequest: "256Mi", MemoryLimit: "1Gi", PullUsername: "cylism-pull", EncryptedCredential: "encrypted", CreatedBy: 1}
	image := &model.ImageRegistry{Name: "受管制品库 · 平台制品库", Endpoint: managed.Endpoint, AuthType: "basic", Username: managed.PullUsername, Credential: managed.EncryptedCredential, Enabled: true, CreatedBy: 1}
	mirror := &model.NodeRegistryMirror{Name: image.Name, Registry: managed.Endpoint, Endpoints: `["https://registry.internal"]`, Username: managed.PullUsername, Credential: managed.EncryptedCredential, Enabled: true, CreatedBy: 1}
	if err := migrated.CreateManagedOCIRegistry(managed, image, mirror, nil); err != nil {
		t.Fatalf("create Registry after legacy schema migration: %v", err)
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
