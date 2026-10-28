package application

import (
	"encoding/json"
	"strings"
	"testing"
)

func envString(value string) *string { return &value }

func TestEnvFileInspectionWithholdsAllValuesAndReadsOnlySelectedField(t *testing.T) {
	content := "MYSQL_HOST=mysql.svc\nMYSQL_PASSWORD=private-password\nDATABASE_URL=mysql://secret@host\nCUSTOM_SETTING=private-value\nMYSQL_PORT=3306\n"
	fields, err := InspectEnvFile(content)
	if err != nil || len(fields) != 5 {
		t.Fatalf("inspection failed: %v", err)
	}
	encoded, _ := json.Marshal(fields)
	for _, value := range []string{"mysql.svc", "private-password", "mysql://secret@host", "private-value", "3306"} {
		if strings.Contains(string(encoded), value) {
			t.Fatal("field listing must not disclose any assignment value")
		}
	}
	value, err := ReadEnvFileField(content, 1, "MYSQL_PASSWORD")
	if err != nil || value != "private-password" {
		t.Fatal("selected field could not be read")
	}
	for _, field := range []EnvFileField{{Line: 0, Name: "MYSQL_PASSWORD"}, {Line: -1, Name: "MYSQL_HOST"}, {Line: 99, Name: "MYSQL_HOST"}} {
		if _, err := ReadEnvFileField(content, field.Line, field.Name); err == nil {
			t.Fatal("mismatched field identity must not return a value")
		}
	}
}

func TestEnvFilePartialUpdatePreservesOpaqueValuesCommentsAndLineEndings(t *testing.T) {
	original := "# database\r\nexport MYSQL_HOST = old.svc\r\nMYSQL_PASSWORD='p#ass=word' # keep this\r\n\r\nMYSQL_PORT=3306\r\n"
	updated, err := UpdateEnvFile(original, []EnvFileChange{{Line: 1, Name: "MYSQL_HOST", Value: envString("new.svc")}, {Line: -1, Name: "LOG_LEVEL", Value: envString("info")}})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(original, "old.svc", "new.svc", 1) + "LOG_LEVEL=info\r\n"
	if updated != want {
		t.Fatalf("file changed unexpectedly: %q", updated)
	}
	updated, err = UpdateEnvFile(original, []EnvFileChange{{Line: 2, Name: "MYSQL_PASSWORD", Value: envString("")}, {Line: 4, Name: "MYSQL_PORT", Remove: true}})
	if err != nil || !strings.Contains(updated, "MYSQL_PASSWORD=\r\n") || strings.Contains(updated, "MYSQL_PORT") {
		t.Fatalf("explicit empty values and removal failed: %q %v", updated, err)
	}
}

func TestEnvFileRejectsAmbiguousOrInjectedEdits(t *testing.T) {
	for _, content := range []string{"A=one\nA=two", "A=\"multi\nline\"", "not an assignment", "A=foo\\\nB=bar"} {
		if _, err := InspectEnvFile(content); err == nil {
			t.Fatalf("unsupported document accepted: %q", content)
		}
	}
	for _, changes := range [][]EnvFileChange{
		{{Line: 0, Name: "WRONG", Value: envString("x")}},
		{{Line: 0, Name: "A", Value: envString("x\nB=injected")}},
		{{Line: -1, Name: "A", Value: envString("duplicate")}},
		{{Line: -1, Name: "bad-name", Value: envString("x")}},
		{{Line: 0, Name: "A", Remove: true, Value: envString("x")}},
		{{Line: 0, Name: "A"}, {Line: 0, Name: "A"}},
	} {
		if _, err := UpdateEnvFile("A=original\n", changes); err == nil {
			t.Fatalf("invalid changes accepted: %+v", changes)
		}
	}
}
