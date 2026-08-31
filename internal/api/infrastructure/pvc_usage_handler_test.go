package infrastructure

import (
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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestPersistentVolumeClaimUsageReadsBoundLocalVolume(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateServer(&model.Server{Name: "storage-node", Host: "100.64.0.8", SSHUser: "root", K8sNodeName: "node-a"}); err != nil {
		t.Fatal(err)
	}

	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "app-data", Namespace: "default", Labels: map[string]string{k8sclient.ManagedByLabel: k8sclient.ManagedByValue}},
			Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: "pv-app-data"},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound, Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("5Gi")}},
		},
		localPV("pv-app-data", "/var/lib/rancher/k3s/storage/pv-app-data", "node-a"),
	)}

	originalReader := ReadPersistentVolumeUsage
	ReadPersistentVolumeUsage = func(_ *model.Server, localPath string, _ []byte) (int64, error) {
		if localPath != "/var/lib/rancher/k3s/storage/pv-app-data" {
			t.Fatalf("unexpected local path: %s", localPath)
		}
		return 1024 * 1024 * 512, nil
	}
	defer func() { ReadPersistentVolumeUsage = originalReader }()

	router := gin.New()
	router.GET("/api/k8s/persistent-volume-claims/usage", NewStorageHandlerWithClient(storageservice.NewService(client, st, st), st, nil, client).ListPersistentVolumeClaimUsage)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/k8s/persistent-volume-claims/usage", nil))

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"used_bytes":536870912`) || !strings.Contains(response.Body.String(), `"status":"available"`) {
		t.Fatalf("unexpected usage response: %d %s", response.Code, response.Body.String())
	}
}

func TestPersistentVolumeClaimUsageReportsUnsupportedVolume(t *testing.T) {
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "remote-data", Namespace: "default"}})}

	router := gin.New()
	router.GET("/api/k8s/persistent-volume-claims/usage", NewStorageHandlerWithClient(storageservice.NewService(client, nil), nil, nil, client).ListPersistentVolumeClaimUsage)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/k8s/persistent-volume-claims/usage", nil))

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"unavailable"`) {
		t.Fatalf("unexpected unsupported usage response: %d %s", response.Code, response.Body.String())
	}
}

func localPV(name, localPath, nodeName string) *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: localPath}},
			NodeAffinity:           &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: corev1.LabelHostname, Operator: corev1.NodeSelectorOpIn, Values: []string{nodeName}}}}}}},
		},
	}
}

func TestParsePersistentVolumeUsage(t *testing.T) {
	used, err := parsePersistentVolumeUsage([]byte("Warning: Permanently added host\n536870912\n"))
	if err != nil || used != 536870912 {
		t.Fatalf("unexpected parsed usage: %d, %v", used, err)
	}
	_, err = parsePersistentVolumeUsage([]byte("permission denied"))
	if err == nil {
		t.Fatal("expected parsing error")
	}
}
