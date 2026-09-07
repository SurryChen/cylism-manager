package k8s

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestIngressStdInfo_FieldsComplete(t *testing.T) {
	i := IngressStdInfo{
		Name:       "web-ingress",
		Namespace:  "default",
		Hosts:      []string{"example.com"},
		Paths:      []string{"/ -> web-svc:80"},
		TLS:        []string{"tls-secret"},
		Controller: "traefik",
		Age:        "3d",
	}
	if i.Controller != "traefik" {
		t.Errorf("expected traefik controller, got %s", i.Controller)
	}
}

func TestIngressControllerStatus_FieldsComplete(t *testing.T) {
	c := IngressControllerStatus{
		Type:      "Traefik",
		Version:   "2.10.7",
		Running:   true,
		CRD:       true,
		Namespace: "kube-system",
	}
	if !c.Running {
		t.Error("expected controller to be running")
	}
}

func TestIngressControllerStatusFromTraefikDeployment(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "traefik", Namespace: "kube-system"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "rancher/mirrored-library-traefik:3.7.4"}}},
			},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 1},
	}

	status := ingressControllerStatusFromDeployment(deployment, true)
	if status.Type != "Traefik" || status.Version != "3.7.4" || !status.Running || !status.CRD {
		t.Fatalf("unexpected Traefik status: %+v", status)
	}
	if status.Namespace != "kube-system" {
		t.Fatalf("expected kube-system namespace, got %q", status.Namespace)
	}
}

func TestCachedIngressControllerStatusReturnsFreshCopyOnly(t *testing.T) {
	now := time.Now()
	cached := &IngressControllerStatus{Type: "Traefik", Version: "3.7.4", Running: true}

	status := cachedIngressControllerStatus(cached, now.Add(time.Minute), now)
	if status == nil || status == cached || status.Version != "3.7.4" {
		t.Fatalf("expected a copied fresh cached status, got %+v", status)
	}
	if expired := cachedIngressControllerStatus(cached, now.Add(-time.Second), now); expired != nil {
		t.Fatalf("expected expired cache to be ignored, got %+v", expired)
	}
}

func TestIngressStdMethods_Exist(t *testing.T) {
	client, _ := newClientFromRestConfig(nil)
	_ = client.ListIngressesContext
	_ = client.GetIngressContext
	_ = client.CreateIngressContext
	_ = client.DeleteIngressContext
	_ = client.DetectIngressControllerContext
}

func TestIngressRouteInfoExtractsRouteAndTLS(t *testing.T) {
	item := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "site", "namespace": "apps", "creationTimestamp": time.Now().UTC().Format(time.RFC3339)},
		"spec":     map[string]interface{}{"routes": []interface{}{map[string]interface{}{"match": "Host(`example.com`)"}}, "tls": map[string]interface{}{"secretName": "site-tls"}},
	}}
	info := ingressRouteToInfo(item)
	if info.Name != "site" || info.Namespace != "apps" || info.Domain != "Host(`example.com`)" || !strings.Contains(info.TLS, "site-tls") {
		t.Fatalf("unexpected ingress route info: %#v", info)
	}
}

func TestIngressRouteClientListsAndDeletesWithSharedDynamicClient(t *testing.T) {
	listKinds := map[schema.GroupVersionResource]string{ingressRouteGVR: "IngressRouteList"}
	item := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "traefik.io/v1alpha1", "kind": "IngressRoute", "metadata": map[string]interface{}{"name": "site", "namespace": "apps"}, "spec": map[string]interface{}{"routes": []interface{}{map[string]interface{}{"match": "Host(`example.com`)"}}}}}
	client := &Client{DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, item)}
	items, err := client.ListIngressRoutesContext(context.Background())
	if err != nil || len(items) != 1 || items[0].Name != "site" {
		t.Fatalf("unexpected list result: %#v %v", items, err)
	}
	if err := client.DeleteIngressRouteContext(context.Background(), "apps", "site"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DynamicClient.Resource(ingressRouteGVR).Namespace("apps").Get(context.Background(), "site", metav1.GetOptions{}); err == nil {
		t.Fatal("expected route to be deleted")
	}
}

func TestIngressRouteClientRequiresDynamicClient(t *testing.T) {
	var nilClient *Client
	if _, err := nilClient.ListIngressRoutesContext(context.Background()); err == nil {
		t.Fatal("expected nil client error")
	}
	client := &Client{}
	if _, err := client.ListIngressRoutesContext(context.Background()); err == nil {
		t.Fatal("expected dynamic client initialization error")
	}
	if err := client.DeleteIngressRouteContext(context.Background(), "apps", "site"); err == nil {
		t.Fatal("expected dynamic client initialization error")
	}
}
