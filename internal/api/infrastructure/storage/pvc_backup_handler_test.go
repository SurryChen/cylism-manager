package storage

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPersistentVolumeBackupRequiresKubernetesDependencies(t *testing.T) {
	router := gin.New()
	router.POST("/api/k8s/persistent-volume-claims/:name/backups", (&StorageHandler{}).CreatePersistentVolumeBackup)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/k8s/persistent-volume-claims/data/backups", strings.NewReader(`{}`)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "K8s") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}
