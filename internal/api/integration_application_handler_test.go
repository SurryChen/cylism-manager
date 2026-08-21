package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestManagedDocumentPatchRejectsRootAndOutsidePaths(t *testing.T) {
	document := model.ManagedDocument{ResourceKind: model.ManagedDocumentResourceSecret, Format: model.ManagedDocumentFormatYAML}
	if err := document.SetAllowedPaths([]string{"/auth/userpass"}); err != nil {
		t.Fatal(err)
	}
	if pathsPermit(document, "/") || pathsPermit(document, "/trafficStats/secret") || !pathsPermit(document, "/auth/userpass/alice") {
		t.Fatal("managed path scope was not enforced")
	}
	root := map[string]interface{}{"auth": map[string]interface{}{"userpass": map[string]interface{}{}}}
	if err := applyObjectPatch(root, managedDocumentPatchOp{Op: "add", Path: "/auth/userpass/alice", Value: "secret-value"}); err != nil {
		t.Fatal(err)
	}
	if err := applyObjectPatch(root, managedDocumentPatchOp{Op: "replace", Path: "/auth/userpass/missing", Value: "rejected"}); err == nil {
		t.Fatal("expected missing patch target to be rejected")
	}
}

func TestManagedDocumentAuditRedactsPatchValue(t *testing.T) {
	redacted := redactManagedDocumentPatch(map[string]interface{}{"operations": []interface{}{map[string]interface{}{"path": "/auth/userpass/alice", "value": "do-not-log"}}})
	if strings.Contains(stringify(redacted), "do-not-log") {
		t.Fatal("managed document patch value leaked to audit detail")
	}
}

func stringify(value interface{}) string {
	encoded, _ := json.Marshal(value)
	return strings.TrimSpace(string(encoded))
}
