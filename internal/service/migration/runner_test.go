package migration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

func TestPlanValidatesBeforeExecutingAnySQL(t *testing.T) {
	scripts := []Script{{Version: "1", Checksum: "one"}, {Version: "2", Checksum: "two"}, {Version: "10", Checksum: "ten"}}
	tests := []struct {
		name, target string
		records      []Record
		want         string
		count        int
	}{
		{name: "numeric ordering", target: "10", count: 3},
		{name: "only pending", target: "10", records: []Record{{Version: "1", Checksum: "one"}}, count: 2},
		{name: "already current", target: "2", records: []Record{{Version: "1", Checksum: "one"}, {Version: "2", Checksum: "two"}}, count: 0},
		{name: "dirty", target: "10", records: []Record{{Version: "1", Checksum: "one", Dirty: true}}, want: "incomplete"},
		{name: "changed", target: "10", records: []Record{{Version: "1", Checksum: "changed"}}, want: "checksum"},
		{name: "missing history", target: "10", records: []Record{{Version: "3", Checksum: "three"}}, want: "missing"},
		{name: "downgrade", target: "2", records: []Record{{Version: "10", Checksum: "ten"}}, want: "ahead"},
		{name: "gap", target: "10", records: []Record{{Version: "2", Checksum: "two"}}, want: "out-of-order"},
		{name: "unknown target", target: "9", want: "absent"},
		{name: "zero", target: "0", want: "positive"},
		{name: "invalid", target: "1; DROP TABLE x", want: "positive"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pending, err := Plan(scripts, test.records, test.target)
			if test.want != "" {
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("Plan error=%v want %s", err, test.want)
				}
				return
			}
			if err != nil || len(pending) != test.count {
				t.Fatalf("pending=%+v err=%v", pending, err)
			}
			if len(pending) == 3 && pending[2].Version != "10" {
				t.Fatal("versions were ordered lexically")
			}
		})
	}
}
func TestReadScriptsUsesChecksumsAndRejectsDuplicatesAndSymlinks(t *testing.T) {
	directory := t.TempDir()
	write := func(name, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(directory, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("10_more.sql", "ALTER TABLE orders ADD status INT;")
	write("2_initial.sql", "CREATE TABLE orders (id INT);")
	scripts, err := ReadScripts(directory)
	if err != nil || len(scripts) != 2 || scripts[0].Version != "2" || len(scripts[0].Checksum) != 64 {
		t.Fatalf("scripts=%+v err=%v", scripts, err)
	}
	write("02_duplicate.sql", "SELECT 1;")
	if _, err = ReadScripts(directory); err == nil {
		t.Fatal("accepted duplicate numeric version")
	}
	os.Remove(filepath.Join(directory, "02_duplicate.sql"))
	if err = os.Symlink(filepath.Join(directory, "2_initial.sql"), filepath.Join(directory, "3_link.sql")); err != nil {
		t.Fatal(err)
	}
	if _, err = ReadScripts(directory); err == nil {
		t.Fatal("accepted symbolic link")
	}
}

// This integration test is opt-in and only operates on an explicitly provided
// disposable schema, never on the configured application database.
func TestMySQLIntegration(t *testing.T) {
	dsn := os.Getenv("CYLISM_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set CYLISM_MIGRATION_TEST_DSN to an isolated disposable MySQL schema")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	// Hold the same advisory lock on a separate physical connection.
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var schema string
	if err = conn.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(schema))
	lockName := fmt.Sprintf("cylism:migrate:%x", hash[:20])
	var locked int
	if err = conn.QueryRowContext(ctx, "SELECT GET_LOCK(?,0)", lockName).Scan(&locked); err != nil || locked != 1 {
		t.Fatalf("lock=%d err=%v", locked, err)
	}
	if err = Run(ctx, db, nil, "1", io.Discard); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("concurrent migration accepted: %v", err)
	}
	conn.ExecContext(ctx, "SELECT RELEASE_LOCK(?)", lockName)
	conn.Close()
	for _, name := range []string{"_cylism_schema_migrations", "migration_test_orders"} {
		if _, err = db.ExecContext(ctx, "DROP TABLE IF EXISTS "+name); err != nil {
			t.Fatal(err)
		}
	}
	directory := t.TempDir()
	os.WriteFile(filepath.Join(directory, "1_initial.sql"), []byte("CREATE TABLE migration_test_orders (id INT PRIMARY KEY); INSERT INTO migration_test_orders VALUES (1);"), 0600)
	os.WriteFile(filepath.Join(directory, "2_status.sql"), []byte("ALTER TABLE migration_test_orders ADD status INT DEFAULT 0;"), 0600)
	scripts, err := ReadScripts(directory)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = Run(ctx, db, scripts, "2", io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM migration_test_orders").Scan(&count); err != nil || count != 1 {
		t.Fatalf("migration replayed: count=%d err=%v", count, err)
	}
	changed := append([]Script{}, scripts...)
	changed[0].Checksum = "changed"
	if err = Run(ctx, db, changed, "2", io.Discard); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("changed script accepted: %v", err)
	}
	if err = Run(ctx, db, scripts, "1", io.Discard); err == nil || !strings.Contains(err.Error(), "ahead") {
		t.Fatalf("downgrade accepted: %v", err)
	}
	os.WriteFile(filepath.Join(directory, "3_broken.sql"), []byte("ALTER TABLE migration_test_orders ADD partial_change INT; INVALID SQL;"), 0600)
	scripts, err = ReadScripts(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err = Run(ctx, db, scripts, "3", io.Discard); err == nil {
		t.Fatal("broken SQL succeeded")
	}
	var dirty bool
	if err = db.QueryRow("SELECT dirty FROM _cylism_schema_migrations WHERE version='3'").Scan(&dirty); err != nil || !dirty {
		t.Fatalf("missing dirty marker: %v", err)
	}
	if err = Run(ctx, db, scripts, "3", io.Discard); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("partial migration reran: %v", err)
	}
}
