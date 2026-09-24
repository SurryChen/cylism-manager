package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	storageservice "github.com/cylism/cylism-manager/internal/service/storage"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

type countingPVCWorkloads struct {
	mu               sync.Mutex
	deploymentCalls  int
	statefulSetCalls int
	namespaces       map[string]struct{}
}

func (w *countingPVCWorkloads) NamespaceExists(context.Context, string) error { return nil }
func (w *countingPVCWorkloads) GetDeployment(context.Context, string, string) (*appsv1.Deployment, error) {
	return nil, nil
}

func (w *countingPVCWorkloads) ListDeployments(_ context.Context, namespace string) ([]appsv1.Deployment, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.deploymentCalls++
	if w.namespaces == nil {
		w.namespaces = make(map[string]struct{})
	}
	w.namespaces[namespace] = struct{}{}
	return nil, nil
}
func (w *countingPVCWorkloads) ListStatefulSets(_ context.Context, namespace string) ([]appsv1.StatefulSet, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.statefulSetCalls++
	if w.namespaces == nil {
		w.namespaces = make(map[string]struct{})
	}
	w.namespaces[namespace] = struct{}{}
	return nil, nil
}

func (w *countingPVCWorkloads) calls() (int, int, map[string]struct{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	namespaces := make(map[string]struct{}, len(w.namespaces))
	for namespace := range w.namespaces {
		namespaces[namespace] = struct{}{}
	}
	return w.deploymentCalls, w.statefulSetCalls, namespaces
}

type blockingPVCWorkloads struct {
	started chan string
	release <-chan struct{}
}

func (w *blockingPVCWorkloads) NamespaceExists(context.Context, string) error { return nil }
func (w *blockingPVCWorkloads) GetDeployment(context.Context, string, string) (*appsv1.Deployment, error) {
	return nil, nil
}
func (w *blockingPVCWorkloads) ListDeployments(_ context.Context, namespace string) ([]appsv1.Deployment, error) {
	w.started <- namespace
	<-w.release
	return nil, nil
}
func (w *blockingPVCWorkloads) ListStatefulSets(context.Context, string) ([]appsv1.StatefulSet, error) {
	return nil, nil
}

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
	pvc, migration, workloads := NewPVCAdapters(client)
	h := NewStorageHandlerWithDependencies(storageservice.NewService(client, st, st), st, nil, pvc, migration, workloads)
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
	pvc, migration, workloads := NewPVCAdapters(client)
	h := NewStorageHandlerWithDependencies(storageservice.NewService(client, st, st), st, nil, pvc, migration, workloads)
	r := gin.New()
	r.GET("/api/k8s/persistent-volume-claims", h.ListPersistentVolumeClaims)
	response := serve(r, httptest.NewRequest(http.MethodGet, "/api/k8s/persistent-volume-claims", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "manual-data") || !strings.Contains(response.Body.String(), "project_name") {
		t.Fatalf("unexpected cluster PVC inventory: %d %s", response.Code, response.Body.String())
	}
}

func TestPersistentVolumeClaimListBatchesWorkloadReferencesByNamespace(t *testing.T) {
	workloads := &countingPVCWorkloads{}
	h := &StorageHandler{workloads: workloads}
	h.pvcResponses(context.Background(), []k8sclient.PersistentVolumeClaimInfo{
		{Name: "one", Namespace: "default"},
		{Name: "two", Namespace: "default"},
		{Name: "three", Namespace: "project-a"},
	}, true)
	deployments, statefulSets, _ := workloads.calls()
	if deployments != 2 || statefulSets != 2 {
		t.Fatalf("expected one workload scan per namespace, deployments=%d statefulsets=%d", deployments, statefulSets)
	}
}

func TestPersistentVolumeClaimListScansNamespacesConcurrently(t *testing.T) {
	release := make(chan struct{})
	workloads := &blockingPVCWorkloads{started: make(chan string, 2), release: release}
	h := &StorageHandler{workloads: workloads}
	done := make(chan struct{})
	go func() {
		h.pvcResponses(context.Background(), []k8sclient.PersistentVolumeClaimInfo{
			{Name: "one", Namespace: "default"},
			{Name: "two", Namespace: "project-a"},
		}, true)
		close(done)
	}()
	for range 2 {
		select {
		case <-workloads.started:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("expected workload scans for distinct namespaces to start concurrently")
		}
	}
	close(release)
	<-done
}

func TestPersistentVolumeClaimListEnrichesOnlyTheRequestedPage(t *testing.T) {
	workloads := &countingPVCWorkloads{}
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "first", Namespace: "default"}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "second", Namespace: "project-a"}},
	)}
	pvc, migration, _ := NewPVCAdapters(client)
	h := NewStorageHandlerWithDependencies(storageservice.NewService(client, nil), nil, nil, pvc, migration, workloads)
	r := gin.New()
	r.GET("/api/k8s/persistent-volume-claims", h.ListPersistentVolumeClaims)

	response := serve(r, httptest.NewRequest(http.MethodGet, "/api/k8s/persistent-volume-claims?page=1&size=1", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"total":2`) || !strings.Contains(response.Body.String(), `"name":"first"`) {
		t.Fatalf("unexpected paginated inventory: %d %s", response.Code, response.Body.String())
	}
	deployments, statefulSets, namespaces := workloads.calls()
	if deployments != 0 || statefulSets != 0 {
		t.Fatalf("expected inventory to skip workload reference scans, deployments=%d statefulsets=%d", deployments, statefulSets)
	}
	if len(namespaces) != 0 {
		t.Fatalf("expected no namespaces to be scanned, got %#v", namespaces)
	}
}

func TestPersistentVolumeClaimReferencesAreLoadedSeparately(t *testing.T) {
	workloads := &countingPVCWorkloads{}
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "first", Namespace: "default"}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "second", Namespace: "project-a"}},
	)}
	pvc, migration, _ := NewPVCAdapters(client)
	h := NewStorageHandlerWithDependencies(storageservice.NewService(client, nil), nil, nil, pvc, migration, workloads)
	r := gin.New()
	r.GET("/api/k8s/persistent-volume-claims/references", h.ListPersistentVolumeClaimReferences)

	response := serve(r, httptest.NewRequest(http.MethodGet, "/api/k8s/persistent-volume-claims/references?claim=default/first&claim=project-a/second", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"namespace":"default","name":"first","references":[]`) || !strings.Contains(response.Body.String(), `"namespace":"project-a","name":"second","references":[]`) {
		t.Fatalf("unexpected PVC references response: %d %s", response.Code, response.Body.String())
	}
	deployments, statefulSets, namespaces := workloads.calls()
	if deployments != 2 || statefulSets != 2 || len(namespaces) != 2 {
		t.Fatalf("expected one workload scan per requested namespace, deployments=%d statefulsets=%d namespaces=%#v", deployments, statefulSets, namespaces)
	}
}

func TestPersistentVolumeClaimsListReturnsPageEnvelopeWhenRequested(t *testing.T) {
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "one", Namespace: "default"}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "two", Namespace: "default"}},
	)}
	pvc, migration, workloads := NewPVCAdapters(client)
	h := NewStorageHandlerWithDependencies(storageservice.NewService(client, nil), nil, nil, pvc, migration, workloads)
	r := gin.New()
	r.GET("/api/k8s/persistent-volume-claims", h.ListPersistentVolumeClaims)
	response := serve(r, httptest.NewRequest(http.MethodGet, "/api/k8s/persistent-volume-claims?page=1&size=1", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"total":2`) || !strings.Contains(response.Body.String(), `"items":[`) {
		t.Fatalf("unexpected paged PVC inventory: %d %s", response.Code, response.Body.String())
	}
}

func TestPersistentVolumeClaimCanBeCreatedInNamespaceWithoutEnvironment(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}})}
	pvc, migration, workloads := NewPVCAdapters(client)
	h := NewStorageHandlerWithDependencies(storageservice.NewService(client, st, st), st, nil, pvc, migration, workloads)
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
