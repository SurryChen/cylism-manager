package infrastructure

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestPersistentVolumeClaimsAreScopedToEnvironment(t *testing.T) {
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
	r.POST("/api/k8s/persistent-volume-claims", h.CreatePersistentVolumeClaim)
	r.GET("/api/k8s/persistent-volume-claims", h.ListPersistentVolumeClaims)

	create := serve(r, newJSONRequest(http.MethodPost, "/api/k8s/persistent-volume-claims", map[string]any{"environment_id": 1, "name": "karakeep-data", "storage": "5Gi"}))
	if create.Code != http.StatusOK || !strings.Contains(create.Body.String(), "karakeep-data") {
		t.Fatalf("unexpected create response: %d %s", create.Code, create.Body.String())
	}
	listed := serve(r, httptest.NewRequest(http.MethodGet, "/api/k8s/persistent-volume-claims?environment_id=1", nil))
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), "karakeep-data") {
		t.Fatalf("unexpected list response: %d %s", listed.Code, listed.Body.String())
	}
}

func TestPersistentVolumeClaimsListClusterInventoryWithoutEnvironment(t *testing.T) {
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
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "manual-data", Namespace: "default"}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "app-data", Namespace: "project-knowledge-prod", Labels: map[string]string{k8sclient.ManagedByLabel: k8sclient.ManagedByValue, k8sclient.EnvironmentLabel: k8sclient.EnvironmentLabelValue(1)}}},
	)}
	h := NewStorageHandlerWithClient(storageservice.NewService(client, st), st, nil, client)
	r := gin.New()
	r.GET("/api/k8s/persistent-volume-claims", h.ListPersistentVolumeClaims)
	response := serve(r, httptest.NewRequest(http.MethodGet, "/api/k8s/persistent-volume-claims", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "manual-data") || !strings.Contains(response.Body.String(), "project_name") {
		t.Fatalf("unexpected cluster PVC inventory: %d %s", response.Code, response.Body.String())
	}
}

func TestPersistentVolumeClaimCanBeCreatedInNamespaceWithoutEnvironment(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}})}
	h := NewStorageHandlerWithClient(storageservice.NewService(client, st), st, nil, client)
	r := gin.New()
	r.POST("/api/k8s/persistent-volume-claims", h.CreatePersistentVolumeClaim)
	response := serve(r, newJSONRequest(http.MethodPost, "/api/k8s/persistent-volume-claims", gin.H{"namespace": "default", "name": "shared-data", "storage": "2Gi"}))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "shared-data") {
		t.Fatalf("unexpected independent PVC create response: %d %s", response.Code, response.Body.String())
	}
	claim, err := client.Clientset.CoreV1().PersistentVolumeClaims("default").Get(context.Background(), "shared-data", metav1.GetOptions{})
	if err != nil || claim.Labels[k8sclient.ManagedByLabel] != k8sclient.ManagedByValue || claim.Labels[k8sclient.EnvironmentLabel] != "" {
		t.Fatalf("expected platform-managed PVC without application environment, claim=%#v err=%v", claim, err)
	}
}
