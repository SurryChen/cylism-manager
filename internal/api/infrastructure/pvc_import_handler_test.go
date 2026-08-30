package infrastructure

import (
	"net/http"
	"strings"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	storageservice "github.com/cylism/cylism-manager/internal/service/storage"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestSafeHostDirectoryImportPath(t *testing.T) {
	valid := []string{"/data/karakeep", "/srv/legacy/app-data", "/home/operator/backups"}
	for _, value := range valid {
		if _, ok := safeHostDirectoryImportPath(value); !ok {
			t.Fatalf("expected %q to be accepted", value)
		}
	}
	invalid := []string{"", "relative/path", "/", "/etc/ssl", "/proc/1", "/var/lib/rancher/k3s/server"}
	for _, value := range invalid {
		if _, ok := safeHostDirectoryImportPath(value); ok {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}

func TestHostDirectoryImportBackupPathIsOutsideSource(t *testing.T) {
	backup := hostDirectoryImportBackupPath("karakeep-data", 12, false)
	if backup != "/data/cylism-import-backups/karakeep-data-import-12-source.tar.gz" {
		t.Fatalf("unexpected source backup path %q", backup)
	}
	targetBackup := hostDirectoryImportBackupPath("karakeep-data", 12, true)
	if targetBackup != "/data/cylism-import-backups/karakeep-data-import-12-target.tar.gz" {
		t.Fatalf("unexpected target backup path %q", targetBackup)
	}
}

func TestHostDirectoryImportPathsOverlap(t *testing.T) {
	if !hostDirectoryImportPathsOverlap("/data/source", "/data/source/nested") {
		t.Fatal("nested source and target must be rejected")
	}
	if hostDirectoryImportPathsOverlap("/data/source", "/data/target") {
		t.Fatal("distinct sibling directories must be allowed")
	}
}

func TestHostDirectoryImportChecksumFromSSHOutput(t *testing.T) {
	checksum := "2f3f5d1a9af59a4e93a6efb1f7c82dfb77c0b3c80f08d2c9ea3294df7c6a3b41"
	output := "Warning: Permanently added '100.64.0.8' (ED25519) to the list of known hosts.\n" + checksum + "\n"
	if got := hostDirectoryImportChecksumFromSSHOutput(output); got != checksum {
		t.Fatalf("expected checksum %q, got %q", checksum, got)
	}
	if got := hostDirectoryImportChecksumFromSSHOutput("warning only"); got != "" {
		t.Fatalf("expected no checksum, got %q", got)
	}
}

func TestStreamCommandDiagnosticRemovesSSHHostKeyWarning(t *testing.T) {
	output := "Warning: Permanently added '100.64.0.8' (ED25519) to the list of known hosts.\ntar: permission denied\n"
	if got := streamCommandDiagnostic(output); got != "tar: permission denied" {
		t.Fatalf("unexpected diagnostic %q", got)
	}
}

func TestDeleteHostDirectoryPVCImportBackupAcceptsEnvironmentIDFromBody(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateProject(&model.Project{Name: "knowledge", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateEnvironment(&model.Environment{ProjectID: 1, Name: "production", Namespace: "project-knowledge-prod"}); err != nil {
		t.Fatal(err)
	}
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "project-knowledge-prod"}})}
	h := NewStorageHandlerWithClient(storageservice.NewService(client, st), st, nil, client)
	r := gin.New()
	r.DELETE("/api/k8s/persistent-volume-claims/:name/imports/:id/backup", h.DeleteHostDirectoryPVCImportBackup)
	response := serve(r, newJSONRequest(http.MethodDelete, "/api/k8s/persistent-volume-claims/karakeep-data/imports/3/backup", gin.H{"environment_id": 1}))
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "目录导入记录不存在") {
		t.Fatalf("expected body environment ID to be accepted, got %d %s", response.Code, response.Body.String())
	}
}
