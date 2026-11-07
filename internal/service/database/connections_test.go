package database

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConnectionDetectionFormats(t *testing.T) {
	for _, test := range []struct {
		name, content string
		file          bool
	}{
		{"url", `mysql://bill:p%40ss@rds.internal:3306/cylism_bill?sslaccept=strict`, false},
		{"go", `bill:p@ss:word@tcp(rds.internal:3306)/cylism_bill?parseTime=true`, false},
		{"yaml", "database:\n  host: rds.internal\n  port: 3306\n  name: cylism_bill\n  username: bill\n  password: do-not-return", true},
		{"json", `{"mysql":{"host":"rds.internal","database":"cylism_bill","user":"bill","password":"do-not-return"}}`, true},
		{"dotenv", "DATABASE_HOST=rds.internal\nDATABASE_PORT=3306\nDATABASE_NAME=cylism_bill\nDATABASE_USER=bill\nDATABASE_PASSWORD=do-not-return", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var hints []ConnectionHint
			if test.file {
				hints = DetectFileConnections(test.content, "config")
			} else {
				hints = DetectConnections(map[string]string{"MYSQL_DSN": test.content}, "env:")
			}
			if len(hints) == 0 || hints[0].Host != "rds.internal" || hints[0].Port != 3306 || hints[0].Database != "cylism_bill" || hints[0].Username != "bill" {
				t.Fatalf("missing connection: %+v", hints)
			}
			raw, _ := json.Marshal(hints)
			if strings.Contains(string(raw), "do-not-return") || strings.Contains(string(raw), "p@ss") || strings.Contains(string(raw), "mysql://") {
				t.Fatal("returned raw credentials")
			}
		})
	}
}

func TestConnectionDetectionDoesNotGuessOrCombineUnrelatedSettings(t *testing.T) {
	for _, values := range []map[string]string{
		{"DATABASE_URL": "postgresql://bill:secret@rds.internal/cylism_bill"},
		{"MYSQL_HOST": "${DB_HOST}", "MYSQL_NAME": "db"},
		{"MYSQL_HOST": "rds.internal", "MYSQL_PORT": "invalid"},
		{"MYSQL_HOST": "rds.internal.evil", "REDIS_USER": "bill", "REDIS_NAME": "db"},
	} {
		hints := DetectConnections(values, "")
		for _, hint := range hints {
			if hint.Host == "rds.internal" && hint.Database == "cylism_bill" || hint.Username == "bill" {
				t.Fatalf("invented connection from unrelated settings: %+v", hints)
			}
		}
	}
	if hints := DetectFileConnections("mysql: &loop\n  nested: *loop\n", "config"); len(hints) != 0 {
		t.Fatalf("followed recursive aliases: %+v", hints)
	}
}
