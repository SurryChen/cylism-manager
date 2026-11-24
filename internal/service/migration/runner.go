// Package migration executes forward-only MySQL migrations from a versioned
// image. The ledger lives in the target schema, alongside the actual changes.
package migration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const HistoryTable = "_cylism_schema_migrations"
const MaxScriptBytes = 16 << 20

var filenamePattern = regexp.MustCompile(`^([0-9]{1,32})_([A-Za-z0-9][A-Za-z0-9_.-]*)\.sql$`)
var targetPattern = regexp.MustCompile(`^[0-9]{1,32}$`)

type Script struct{ Version, Name, Checksum, SQL string }
type Record struct {
	Version, Checksum string
	Dirty             bool
}

func numericVersion(value string) *big.Int { n, _ := new(big.Int).SetString(value, 10); return n }
func validVersion(value string) bool {
	return targetPattern.MatchString(value) && numericVersion(value).Sign() > 0
}
func CompareVersion(a, b string) int { return numericVersion(a).Cmp(numericVersion(b)) }

func ReadScripts(directory string) ([]Script, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read migration directory: %w", err)
	}
	scripts := make([]Script, 0)
	seen := map[string]bool{}
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		match := filenamePattern.FindStringSubmatch(entry.Name())
		if match == nil || !validVersion(match[1]) {
			return nil, fmt.Errorf("invalid migration filename %s (expected positiveVersion_description.sql)", entry.Name())
		}
		info, e := entry.Info()
		if e != nil {
			return nil, e
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("migration must be a regular file: %s", entry.Name())
		}
		if info.Size() > MaxScriptBytes {
			return nil, fmt.Errorf("migration exceeds 16 MiB: %s", entry.Name())
		}
		total += info.Size()
		if total > 64<<20 {
			return nil, fmt.Errorf("migration bundle exceeds 64 MiB")
		}
		canonical := numericVersion(match[1]).String()
		if seen[canonical] {
			return nil, fmt.Errorf("duplicate migration version: %s", match[1])
		}
		seen[canonical] = true
		file, e := os.Open(filepath.Join(directory, entry.Name()))
		if e != nil {
			return nil, e
		}
		data, e := io.ReadAll(io.LimitReader(file, MaxScriptBytes+1))
		file.Close()
		if e != nil {
			return nil, e
		}
		if len(data) > MaxScriptBytes {
			return nil, fmt.Errorf("migration exceeds size limit")
		}
		if strings.TrimSpace(string(data)) == "" {
			return nil, fmt.Errorf("empty migration: %s", entry.Name())
		}
		sum := sha256.Sum256(data)
		scripts = append(scripts, Script{Version: match[1], Name: entry.Name(), Checksum: hex.EncodeToString(sum[:]), SQL: string(data)})
	}
	sort.Slice(scripts, func(i, j int) bool { return CompareVersion(scripts[i].Version, scripts[j].Version) < 0 })
	return scripts, nil
}

// Plan validates the entire applied history before returning any pending SQL.
// Unknown/applied-but-modified scripts and partial failures stop the release.
func Plan(scripts []Script, records []Record, target string) ([]Script, error) {
	if !validVersion(target) {
		return nil, fmt.Errorf("target must be a positive version of at most 32 digits")
	}
	available := map[string]Script{}
	targetFound := false
	for _, script := range scripts {
		if !validVersion(script.Version) {
			return nil, fmt.Errorf("invalid script version")
		}
		key := numericVersion(script.Version).String()
		if _, exists := available[key]; exists {
			return nil, fmt.Errorf("duplicate script version %s", script.Version)
		}
		available[key] = script
		if CompareVersion(script.Version, target) == 0 {
			targetFound = true
		}
	}
	if !targetFound {
		return nil, fmt.Errorf("target %s is absent from migration bundle", target)
	}
	applied := map[string]bool{}
	latest := "0"
	for _, record := range records {
		if !validVersion(record.Version) {
			return nil, fmt.Errorf("invalid stored migration version")
		}
		if record.Dirty {
			return nil, fmt.Errorf("migration %s is incomplete; inspect and repair the database before retrying", record.Version)
		}
		script, ok := available[numericVersion(record.Version).String()]
		if !ok {
			return nil, fmt.Errorf("applied migration %s is missing from this image", record.Version)
		}
		if script.Checksum != record.Checksum {
			return nil, fmt.Errorf("checksum mismatch for migration %s", record.Version)
		}
		if CompareVersion(record.Version, target) > 0 {
			return nil, fmt.Errorf("database version %s is ahead of target %s; downgrades are not supported", record.Version, target)
		}
		applied[numericVersion(record.Version).String()] = true
		if CompareVersion(record.Version, latest) > 0 {
			latest = record.Version
		}
	}
	var pending []Script
	for _, script := range scripts {
		if CompareVersion(script.Version, target) > 0 || applied[numericVersion(script.Version).String()] {
			continue
		}
		if CompareVersion(script.Version, latest) < 0 {
			return nil, fmt.Errorf("out-of-order migration %s predates applied version %s", script.Version, latest)
		}
		pending = append(pending, script)
	}
	sort.Slice(pending, func(i, j int) bool { return CompareVersion(pending[i].Version, pending[j].Version) < 0 })
	return pending, nil
}

func Run(ctx context.Context, db *sql.DB, scripts []Script, target string, out io.Writer) error {
	if !validVersion(target) {
		return fmt.Errorf("invalid migration target")
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("connect to migration database: %w", err)
	}
	defer conn.Close()
	var schema sql.NullString
	if err = conn.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&schema); err != nil || !schema.Valid || schema.String == "" {
		return fmt.Errorf("migration database must be selected")
	}
	lockHash := sha256.Sum256([]byte(schema.String))
	lockName := fmt.Sprintf("cylism:migrate:%x", lockHash[:20])
	var acquired sql.NullInt64
	if err = conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", lockName).Scan(&acquired); err != nil || !acquired.Valid || acquired.Int64 != 1 {
		return fmt.Errorf("database migration is already running or lock is unavailable")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(cleanup, "SELECT RELEASE_LOCK(?)", lockName)
	}()
	_, err = conn.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS `_cylism_schema_migrations` (version VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY, name VARCHAR(255) NOT NULL, checksum CHAR(64) NOT NULL, dirty BOOLEAN NOT NULL, started_at DATETIME(6) NOT NULL, completed_at DATETIME(6) NULL) ENGINE=InnoDB")
	if err != nil {
		return fmt.Errorf("initialize migration ledger: %w", err)
	}
	rows, err := conn.QueryContext(ctx, "SELECT version, checksum, dirty FROM `_cylism_schema_migrations`")
	if err != nil {
		return fmt.Errorf("read migration ledger: %w", err)
	}
	var records []Record
	for rows.Next() {
		var record Record
		if err = rows.Scan(&record.Version, &record.Checksum, &record.Dirty); err != nil {
			rows.Close()
			return err
		}
		records = append(records, record)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	pending, err := Plan(scripts, records, target)
	if err != nil {
		return err
	}
	for _, script := range pending {
		fmt.Fprintf(out, "Applying %s sha256=%s\n", script.Name, script.Checksum)
		_, err = conn.ExecContext(ctx, "INSERT INTO `_cylism_schema_migrations` (version,name,checksum,dirty,started_at) VALUES (?,?,?,TRUE,UTC_TIMESTAMP(6))", script.Version, script.Name, script.Checksum)
		if err != nil {
			return fmt.Errorf("record migration %s: %w", script.Version, err)
		}
		// MySQL DDL can implicitly commit. A durable dirty marker is deliberately
		// written before SQL and is never automatically cleared on an error.
		if _, err = conn.ExecContext(ctx, script.SQL); err != nil {
			return fmt.Errorf("migration %s failed; ledger is dirty and automatic retry is blocked: %w", script.Version, err)
		}
		result, e := conn.ExecContext(ctx, "UPDATE `_cylism_schema_migrations` SET dirty=FALSE,completed_at=UTC_TIMESTAMP(6) WHERE version=? AND checksum=? AND dirty=TRUE", script.Version, script.Checksum)
		if e != nil {
			return fmt.Errorf("complete migration %s: %w", script.Version, e)
		}
		count, e := result.RowsAffected()
		if e != nil || count != 1 {
			return fmt.Errorf("migration %s ledger changed unexpectedly", script.Version)
		}
		fmt.Fprintf(out, "Completed %s\n", script.Version)
	}
	fmt.Fprintf(out, "Database verified at target %s; applied %d migration(s).\n", target, len(pending))
	return nil
}
