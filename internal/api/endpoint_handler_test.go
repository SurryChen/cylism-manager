package api

import (
	"github.com/cylism/cylism-manager/internal/application"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestSyncApplicationEndpointsRemovesHTTPIngressForUDPService(t *testing.T) {
	_, s := setupApplicationRouter()
	app := createApplicationForReleaseRuntimeTest(t, s)
	if err := s.CreateApplicationEndpoint(&model.ApplicationEndpoint{ApplicationID: app.ID, Exposure: application.ExposurePublic, Domain: "udp.example.com", Path: "/", ServicePort: 443}); err != nil {
		t.Fatal(err)
	}
	originalK8s := K8s
	clientset := k8sfake.NewSimpleClientset()
	K8s = &k8sclient.Client{Clientset: clientset}
	defer func() { K8s = originalK8s }()
	handler := NewApplicationHandler(s, []byte("01234567890123456789012345678901"))
	if err := handler.syncApplicationEndpoints(t.Context(), app, application.ServiceSpec{Port: 443}); err != nil {
		t.Fatalf("create TCP Ingress: %v", err)
	}
	if _, err := clientset.NetworkingV1().Ingresses(app.Environment.Namespace).Get(t.Context(), app.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("expected TCP ingress: %v", err)
	}
	if err := handler.syncApplicationEndpoints(t.Context(), app, application.ServiceSpec{Port: 443, Protocol: application.ServiceProtocolUDP}); err != nil {
		t.Fatalf("remove UDP ingress: %v", err)
	}
	if _, err := clientset.NetworkingV1().Ingresses(app.Environment.Namespace).Get(t.Context(), app.Name, metav1.GetOptions{}); err == nil {
		t.Fatal("UDP Service must not retain an HTTP Ingress")
	}
}

func TestSyncApplicationEndpointsUsesFirstTCPPortForMultiPortService(t *testing.T) {
	_, s := setupApplicationRouter()
	app := createApplicationForReleaseRuntimeTest(t, s)
	if err := s.CreateApplicationEndpoint(&model.ApplicationEndpoint{ApplicationID: app.ID, Exposure: application.ExposurePublic, Domain: "api.example.com", Path: "/", ServicePort: 80}); err != nil {
		t.Fatal(err)
	}
	originalK8s := K8s
	clientset := k8sfake.NewSimpleClientset()
	K8s = &k8sclient.Client{Clientset: clientset}
	defer func() { K8s = originalK8s }()

	handler := NewApplicationHandler(s, []byte("01234567890123456789012345678901"))
	service := application.ServiceSpec{Ports: []application.ServicePortSpec{
		{Name: "proxy", Port: 443, TargetPort: 443, Protocol: application.ServiceProtocolUDP},
		{Name: "api", Port: 8080, TargetPort: 8080, Protocol: application.ServiceProtocolTCP},
	}}
	if err := handler.syncApplicationEndpoints(t.Context(), app, service); err != nil {
		t.Fatalf("sync multi-port ingress: %v", err)
	}
	ingress, err := clientset.NetworkingV1().Ingresses(app.Environment.Namespace).Get(t.Context(), app.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if port := ingress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Port.Number; port != 8080 {
		t.Fatalf("expected first TCP Service port, got %d", port)
	}
	endpoints, err := s.ListApplicationEndpoints(app.ID)
	if err != nil || endpoints[0].ServicePort != 8080 {
		t.Fatalf("expected endpoint to track TCP Service port, endpoints=%#v err=%v", endpoints, err)
	}
}

func TestApplicationHandlerBindsDomainIndependentlyFromReleaseTemplate(t *testing.T) {
	r, s := setupApplicationRouter()
	if err := s.CreateProject(&model.Project{Name: "commerce", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateEnvironment(&model.Environment{ProjectID: 1, Name: "production", Namespace: "commerce-prod"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateApplication(&model.Application{ProjectID: 1, EnvironmentID: 1, Name: "order-api", WorkloadKind: "deployment", CreatedBy: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateManagedDomain(&model.ManagedDomain{Hostname: "api.example.com", EnvironmentID: 1, Namespace: "commerce-prod", CertificateName: "api-cert", TLSSecretName: "api-tls", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateManagedDomain(&model.ManagedDomain{Hostname: "admin.example.com", EnvironmentID: 1, Namespace: "commerce-prod", CertificateName: "admin-cert", TLSSecretName: "admin-tls", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "commerce-prod"}})}
	defer func() { K8s = originalK8s }()

	bind := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/endpoints", gin.H{"domain_id": 1, "path": "/", "tls_enabled": false}))
	if bind.Code != http.StatusOK || !strings.Contains(bind.Body.String(), "api.example.com") {
		t.Fatalf("bind endpoint: %d %s", bind.Code, bind.Body.String())
	}
	firstID := responseID(t, bind.Body.Bytes())
	second := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/endpoints", gin.H{"domain_id": 2, "path": "/console", "tls_enabled": false}))
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), "admin.example.com") {
		t.Fatalf("bind second endpoint: %d %s", second.Code, second.Body.String())
	}
	endpoints := serve(r, newJSONRequest(http.MethodGet, "/api/applications/1/endpoints", nil))
	if endpoints.Code != http.StatusOK || !strings.Contains(endpoints.Body.String(), "api.example.com") || !strings.Contains(endpoints.Body.String(), "admin.example.com") {
		t.Fatalf("list endpoints: %d %s", endpoints.Code, endpoints.Body.String())
	}
	updated := serve(r, newJSONRequest(http.MethodPut, "/api/applications/1/endpoints/"+strconv.Itoa(int(firstID)), gin.H{"domain_id": 1, "path": "/v2", "tls_enabled": false}))
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), "/v2") {
		t.Fatalf("update endpoint: %d %s", updated.Code, updated.Body.String())
	}
	remove := serve(r, newJSONRequest(http.MethodDelete, "/api/applications/1/endpoints/"+strconv.Itoa(int(firstID)), nil))
	if remove.Code != http.StatusOK {
		t.Fatalf("delete endpoint: %d %s", remove.Code, remove.Body.String())
	}
}

func TestApplicationHandlerRejectsHTTPIngressBindingForUDPService(t *testing.T) {
	r, s := setupApplicationRouter()
	if err := s.CreateProject(&model.Project{Name: "edge", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateEnvironment(&model.Environment{ProjectID: 1, Name: "production", Namespace: "edge-prod"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateApplication(&model.Application{ProjectID: 1, EnvironmentID: 1, Name: "udp-server", WorkloadKind: "deployment", CreatedBy: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateManagedDomain(&model.ManagedDomain{Hostname: "edge.example.com", EnvironmentID: 1, Namespace: "edge-prod", CertificateName: "edge-cert", TLSSecretName: "edge-tls", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRelease(&model.Release{ApplicationID: 1, Sequence: 1, Image: "registry.example.com/udp-server:1.0.0", DesiredSpec: `{"service":{"port":443,"protocol":"UDP"}}`, Status: model.ReleaseStatusSucceeded, CreatedBy: 1}); err != nil {
		t.Fatal(err)
	}
	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "edge-prod"}})}
	defer func() { K8s = originalK8s }()

	response := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/endpoints", gin.H{"domain_id": 1, "path": "/", "tls_enabled": false}))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "UDP Service 不支持 HTTP Ingress") {
		t.Fatalf("expected UDP Ingress rejection, got %d %s", response.Code, response.Body.String())
	}
	metadata := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/endpoints", gin.H{"domain_id": 1, "path": "/", "tls_enabled": false, "ingress_enabled": false}))
	if metadata.Code != http.StatusOK || !strings.Contains(metadata.Body.String(), `"ingress_enabled":false`) {
		t.Fatalf("expected metadata-only UDP binding, got %d %s", metadata.Code, metadata.Body.String())
	}
	duplicateMetadata := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/endpoints", gin.H{"domain_id": 1, "path": "/", "tls_enabled": false, "ingress_enabled": false}))
	if duplicateMetadata.Code != http.StatusOK {
		t.Fatalf("metadata-only bindings should not conflict, got %d %s", duplicateMetadata.Code, duplicateMetadata.Body.String())
	}
}
