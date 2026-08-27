package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func setupManagedOCIRegistryRouter(t *testing.T) (*gin.Engine, *store.Store) {
	r, s, _ := setupManagedOCIRegistryRouterWithHandler(t)
	return r, s
}

func setupManagedOCIRegistryRouterWithHandler(t *testing.T) (*gin.Engine, *store.Store, *ManagedOCIRegistryHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	original := K8s
	certificate := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cert-manager.io/v1", "kind": "Certificate",
		"metadata": map[string]interface{}{"name": "registry-cert", "namespace": "cylism-system"},
		"spec":     map[string]interface{}{"dnsNames": []interface{}{"registry.internal"}, "secretName": "registry-tls"},
		"status":   map[string]interface{}{"conditions": []interface{}{map[string]interface{}{"type": "Ready", "status": "True"}}},
	}}
	K8s = &k8s.Client{Clientset: k8sfake.NewSimpleClientset(), DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}: "CertificateList"}, certificate)}
	mode := storagev1.VolumeBindingWaitForFirstConsumer
	_, err = K8s.Clientset.StorageV1().StorageClasses().Create(context.Background(), &storagev1.StorageClass{
		ObjectMeta:        metav1.ObjectMeta{Name: "local-path"},
		Provisioner:       "rancher.io/local-path",
		VolumeBindingMode: &mode,
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = K8s.Clientset.CoreV1().Secrets("cylism-system").Create(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "registry-tls", Namespace: "cylism-system"},
		Type:       corev1.SecretTypeTLS,
		Data:       map[string][]byte{corev1.TLSCertKey: []byte("test-cert"), corev1.TLSPrivateKeyKey: []byte("test-key")},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = K8s.Clientset.CoreV1().Nodes().Create(context.Background(), &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
			Type: corev1.NodeReady, Status: corev1.ConditionTrue,
		}}},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { K8s = original })
	h := NewManagedOCIRegistryHandler(s, []byte("01234567890123456789012345678901"))
	r := gin.New()
	group := r.Group("/api/managed-oci-registries")
	group.GET("", h.List)
	group.GET("/storage-preflight", h.StoragePreflight)
	group.GET("/certificates", h.ListMatchingCertificates)
	group.POST("", h.Create)
	group.GET("/:id", h.Get)
	group.PUT("/:id", h.Update)
	group.POST("/:id/apply-node-access", h.ApplyNodeAccess)
	group.DELETE("/:id", h.Delete)
	return r, s, h
}

func managedRegistryPayload() gin.H {
	return gin.H{
		"name": "内网制品库", "namespace": "cylism-system", "endpoint": "registry.internal:5443",
		"registry_image": "registry:2.8", "data_node": "node-a", "storage_class_name": "local-path", "storage_size": "100Gi",
		"cpu_request": "100m", "cpu_limit": "500m", "memory_request": "256Mi", "memory_limit": "1Gi",
		"certificate_name": "registry-cert", "pull_username": "cylism-pull", "pull_password": "safe-registry-password",
	}
}

func TestManagedOCIRegistryCreateRendersProtectedResources(t *testing.T) {
	r, s := setupManagedOCIRegistryRouter(t)
	response := serve(r, newJSONRequest(http.MethodPost, "/api/managed-oci-registries", managedRegistryPayload()))
	if response.Code != http.StatusOK {
		t.Fatalf("create status = %d: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "safe-registry-password") || !strings.Contains(response.Body.String(), `"credential_configured":true`) {
		t.Fatalf("credential must be redacted: %s", response.Body.String())
	}
	registry, err := s.GetManagedOCIRegistry(1)
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := K8s.Clientset.AppsV1().Deployments(registry.Namespace).Get(context.Background(), registry.ResourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertManagedRegistryDeployment(t, deployment, registry)
	pvc, err := K8s.Clientset.CoreV1().PersistentVolumeClaims(registry.Namespace).Get(context.Background(), registry.PVCName, metav1.GetOptions{})
	if err != nil || pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "local-path" || pvc.Spec.Resources.Requests.Storage().String() != "100Gi" {
		t.Fatalf("expected local-path PVC: %#v err=%v", pvc, err)
	}
	secret, err := K8s.Clientset.CoreV1().Secrets(registry.Namespace).Get(context.Background(), registry.ResourceName+"-auth", metav1.GetOptions{})
	if err != nil || strings.Contains(string(secret.Data["htpasswd"]), "safe-registry-password") {
		t.Fatalf("expected hashed auth Secret, secret=%#v err=%v", secret, err)
	}
	ingress, err := K8s.Clientset.NetworkingV1().Ingresses(registry.Namespace).Get(context.Background(), registry.ResourceName, metav1.GetOptions{})
	if err != nil || ingress.Spec.Rules[0].Host != "registry.internal" || len(ingress.Spec.TLS) != 1 {
		t.Fatalf("expected HTTPS host ingress, ingress=%#v err=%v", ingress, err)
	}
}

func TestManagedOCIRegistryRejectsUnconfirmedHTTP(t *testing.T) {
	r, _ := setupManagedOCIRegistryRouter(t)
	payload := managedRegistryPayload()
	payload["endpoint"] = "registry.internal:80"
	payload["insecure_http"] = true
	response := serve(r, newJSONRequest(http.MethodPost, "/api/managed-oci-registries", payload))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "明确确认") {
		t.Fatalf("expected insecure confirmation rejection: %d %s", response.Code, response.Body.String())
	}
	payload["confirm_insecure_http"] = true
	payload["certificate_name"] = ""
	response = serve(r, newJSONRequest(http.MethodPost, "/api/managed-oci-registries", payload))
	if response.Code != http.StatusOK {
		t.Fatalf("expected confirmed HTTP creation: %d %s", response.Code, response.Body.String())
	}
}

func TestManagedOCIRegistryListsAndRequiresMatchingReadyCertificate(t *testing.T) {
	r, _ := setupManagedOCIRegistryRouter(t)
	options := serve(r, newJSONRequest(http.MethodGet, "/api/managed-oci-registries/certificates?namespace=cylism-system&endpoint=registry.internal:5443", nil))
	if options.Code != http.StatusOK || !strings.Contains(options.Body.String(), `"name":"registry-cert"`) || strings.Contains(options.Body.String(), `"secret_name":"registry-tls"`) {
		t.Fatalf("expected redacted matching certificate option: %d %s", options.Code, options.Body.String())
	}
	payload := managedRegistryPayload()
	delete(payload, "certificate_name")
	response := serve(r, newJSONRequest(http.MethodPost, "/api/managed-oci-registries", payload))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "证书") {
		t.Fatalf("expected matching certificate requirement: %d %s", response.Code, response.Body.String())
	}
	payload = managedRegistryPayload()
	payload["endpoint"] = "other.internal:5443"
	response = serve(r, newJSONRequest(http.MethodPost, "/api/managed-oci-registries", payload))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "未覆盖") {
		t.Fatalf("expected certificate hostname rejection: %d %s", response.Code, response.Body.String())
	}
}

func TestManagedOCIRegistryRejectsUnavailableStorageAndInvalidResources(t *testing.T) {
	r, _ := setupManagedOCIRegistryRouter(t)
	if err := K8s.Clientset.StorageV1().StorageClasses().Delete(context.Background(), "local-path", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	response := serve(r, newJSONRequest(http.MethodPost, "/api/managed-oci-registries", managedRegistryPayload()))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "StorageClass") {
		t.Fatalf("expected StorageClass rejection: %d %s", response.Code, response.Body.String())
	}
	preflight := serve(r, newJSONRequest(http.MethodGet, "/api/managed-oci-registries/storage-preflight", nil))
	if preflight.Code != http.StatusOK || !strings.Contains(preflight.Body.String(), `"ready":false`) {
		t.Fatalf("expected unavailable StorageClass preflight: %d %s", preflight.Code, preflight.Body.String())
	}

	r, _ = setupManagedOCIRegistryRouter(t)
	payload := managedRegistryPayload()
	payload["cpu_request"] = "2"
	payload["cpu_limit"] = "500m"
	response = serve(r, newJSONRequest(http.MethodPost, "/api/managed-oci-registries", payload))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "request") {
		t.Fatalf("expected resource validation rejection: %d %s", response.Code, response.Body.String())
	}
	payload = managedRegistryPayload()
	payload["data_node"] = "missing-node"
	response = serve(r, newJSONRequest(http.MethodPost, "/api/managed-oci-registries", payload))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "数据节点") {
		t.Fatalf("expected unavailable data node rejection: %d %s", response.Code, response.Body.String())
	}
}

func TestManagedOCIRegistryStoragePreflightListsReadyNodesAndClasses(t *testing.T) {
	r, _ := setupManagedOCIRegistryRouter(t)
	response := serve(r, newJSONRequest(http.MethodGet, "/api/managed-oci-registries/storage-preflight", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ready":true`) || !strings.Contains(response.Body.String(), `"data_nodes":["node-a"]`) || !strings.Contains(response.Body.String(), `"storage_classes":["local-path"]`) {
		t.Fatalf("expected ready node and storage class options: %d %s", response.Code, response.Body.String())
	}
}

func TestManagedOCIRegistryRejectsStorageClassWithoutWaitForFirstConsumer(t *testing.T) {
	r, _ := setupManagedOCIRegistryRouter(t)
	storageClass, err := K8s.Clientset.StorageV1().StorageClasses().Get(context.Background(), "local-path", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	immediate := storagev1.VolumeBindingImmediate
	storageClass.VolumeBindingMode = &immediate
	if _, err := K8s.Clientset.StorageV1().StorageClasses().Update(context.Background(), storageClass, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	response := serve(r, newJSONRequest(http.MethodPost, "/api/managed-oci-registries", managedRegistryPayload()))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "WaitForFirstConsumer") {
		t.Fatalf("expected unsafe StorageClass rejection: %d %s", response.Code, response.Body.String())
	}
	preflight := serve(r, newJSONRequest(http.MethodGet, "/api/managed-oci-registries/storage-preflight", nil))
	if preflight.Code != http.StatusOK || !strings.Contains(preflight.Body.String(), `"ready":false`) {
		t.Fatalf("expected unsafe StorageClass preflight: %d %s", preflight.Code, preflight.Body.String())
	}
}

func TestManagedOCIRegistryReportsPendingPVC(t *testing.T) {
	r, s := setupManagedOCIRegistryRouter(t)
	created := serve(r, newJSONRequest(http.MethodPost, "/api/managed-oci-registries", managedRegistryPayload()))
	if created.Code != http.StatusOK {
		t.Fatal(created.Body.String())
	}
	registry, err := s.GetManagedOCIRegistry(1)
	if err != nil {
		t.Fatal(err)
	}
	pvc, err := K8s.Clientset.CoreV1().PersistentVolumeClaims(registry.Namespace).Get(context.Background(), registry.PVCName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pvc.Status.Phase = corev1.ClaimPending
	if _, err := K8s.Clientset.CoreV1().PersistentVolumeClaims(registry.Namespace).UpdateStatus(context.Background(), pvc, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	response := serve(r, newJSONRequest(http.MethodGet, "/api/managed-oci-registries/1", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"pvc_phase":"Pending"`) || !strings.Contains(response.Body.String(), `"status":"pending"`) {
		t.Fatalf("expected pending PVC status: %d %s", response.Code, response.Body.String())
	}
}

func TestManagedOCIRegistryRequiresMigrationForLegacyHostPath(t *testing.T) {
	r, s := setupManagedOCIRegistryRouter(t)
	created := serve(r, newJSONRequest(http.MethodPost, "/api/managed-oci-registries", managedRegistryPayload()))
	if created.Code != http.StatusOK {
		t.Fatal(created.Body.String())
	}
	registry, err := s.GetManagedOCIRegistry(1)
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := K8s.Clientset.AppsV1().Deployments(registry.Namespace).Get(context.Background(), registry.ResourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	hostPathType := corev1.HostPathDirectory
	deployment.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim = nil
	deployment.Spec.Template.Spec.Volumes[0].HostPath = &corev1.HostPathVolumeSource{Path: "/data/cylism-registry", Type: &hostPathType}
	if _, err := K8s.Clientset.AppsV1().Deployments(registry.Namespace).Update(context.Background(), deployment, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	status := serve(r, newJSONRequest(http.MethodGet, "/api/managed-oci-registries/1", nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"status":"migration_required"`) {
		t.Fatalf("expected migration-required status: %d %s", status.Code, status.Body.String())
	}
	payload := managedRegistryPayload()
	delete(payload, "pull_password")
	updated := serve(r, newJSONRequest(http.MethodPut, "/api/managed-oci-registries/1", payload))
	if updated.Code != http.StatusConflict || !strings.Contains(updated.Body.String(), "迁移") {
		t.Fatalf("expected legacy update block: %d %s", updated.Code, updated.Body.String())
	}
}

func TestManagedOCIRegistryDeleteKeepsDataAndBlocksReleaseReference(t *testing.T) {
	r, s := setupManagedOCIRegistryRouter(t)
	response := serve(r, newJSONRequest(http.MethodPost, "/api/managed-oci-registries", managedRegistryPayload()))
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	registry, _ := s.GetManagedOCIRegistry(1)
	if _, _, err := s.CountManagedOCIRegistryReferences(registry.ID); err != nil {
		t.Fatal(err)
	}
	missingConfirm := serve(r, newJSONRequest(http.MethodDelete, "/api/managed-oci-registries/1", gin.H{}))
	if missingConfirm.Code != http.StatusBadRequest {
		t.Fatalf("expected explicit confirmation: %d %s", missingConfirm.Code, missingConfirm.Body.String())
	}
	deleted := serve(r, newJSONRequest(http.MethodDelete, "/api/managed-oci-registries/1", gin.H{"confirm": true}))
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete failed: %d %s", deleted.Code, deleted.Body.String())
	}
	if _, err := K8s.Clientset.AppsV1().Deployments(registry.Namespace).Get(context.Background(), registry.ResourceName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("deployment should be deleted, got %v", err)
	}
	if _, err := K8s.Clientset.CoreV1().PersistentVolumeClaims(registry.Namespace).Get(context.Background(), registry.PVCName, metav1.GetOptions{}); err != nil {
		t.Fatalf("PVC should be retained, got %v", err)
	}
}

func TestManagedOCIRegistryBlocksUnmanagedEndpointCollision(t *testing.T) {
	r, s := setupManagedOCIRegistryRouter(t)
	if err := s.CreateImageRegistry(&model.ImageRegistry{Name: "外部仓库", Endpoint: "registry.internal:5443", VerificationImage: "registry.internal:5443/app:latest", AuthType: registryAuthAnonymous, Enabled: true}, nil); err != nil {
		t.Fatal(err)
	}
	response := serve(r, newJSONRequest(http.MethodPost, "/api/managed-oci-registries", managedRegistryPayload()))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "非受管记录") {
		t.Fatalf("expected unmanaged collision: %d %s", response.Code, response.Body.String())
	}
}

func TestManagedOCIRegistryAppliesSelectedNodesAndKeepsPerNodeStatus(t *testing.T) {
	r, s, handler := setupManagedOCIRegistryRouterWithHandler(t)
	if err := s.CreateServer(&model.Server{Name: "worker-a", Host: "10.0.0.4", ClusterRole: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateServer(&model.Server{Name: "worker-b", Host: "10.0.0.5", ClusterRole: "worker"}); err != nil {
		t.Fatal(err)
	}
	handler.applyNode = func(server *model.Server, _ []byte) (string, string) {
		if server.Host == "10.0.0.5" {
			return "failed", "SSH unavailable"
		}
		return "success", "applied"
	}
	created := serve(r, newJSONRequest(http.MethodPost, "/api/managed-oci-registries", managedRegistryPayload()))
	if created.Code != http.StatusOK {
		t.Fatal(created.Body.String())
	}
	response := serve(r, newJSONRequest(http.MethodPost, "/api/managed-oci-registries/1/apply-node-access", gin.H{"server_ids": []uint{1, 2}}))
	if response.Code != http.StatusOK {
		t.Fatalf("apply failed: %d %s", response.Code, response.Body.String())
	}
	registry, err := s.GetManagedOCIRegistry(1)
	if err != nil || registry.NodeRegistryMirrorID == nil {
		t.Fatalf("missing managed mirror: %#v %v", registry, err)
	}
	mirror, err := s.GetNodeRegistryMirror(*registry.NodeRegistryMirrorID)
	if err != nil || mirror.LastApplyStatus != "failed" || len(mirror.NodeStatuses) != 2 {
		t.Fatalf("expected partial application details: %#v %v", mirror, err)
	}
}

func assertManagedRegistryDeployment(t *testing.T, deployment *appsv1.Deployment, registry *model.ManagedOCIRegistry) {
	t.Helper()
	if deployment.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType || deployment.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] != registry.DataNode {
		t.Fatalf("expected Recreate and node pinning: %#v", deployment.Spec)
	}
	if len(deployment.Spec.Template.Spec.Volumes) != 2 || deployment.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim == nil || deployment.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName != registry.PVCName {
		t.Fatalf("expected PVC storage and auth volume: %#v", deployment.Spec.Template.Spec.Volumes)
	}
	container := deployment.Spec.Template.Spec.Containers[0]
	if deployment.Labels["app.kubernetes.io/managed-by"] != "cylism-manager" || container.Image != "registry:2.8" {
		t.Fatalf("expected managed Registry labels and image: %#v", deployment)
	}
	if container.Resources.Requests.Cpu().String() != "100m" || container.Resources.Limits.Cpu().String() != "500m" || container.Resources.Requests.Memory().String() != "256Mi" || container.Resources.Limits.Memory().String() != "1Gi" {
		t.Fatalf("expected Registry resource configuration: %#v", container.Resources)
	}
}

func TestManagedRegistryEndpointHealthTreatsUnauthorizedAsReady(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", "Basic")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	if !managedRegistryEndpointReady(context.Background(), server.URL) {
		t.Fatal("401 Registry response must be treated as ready")
	}
}
