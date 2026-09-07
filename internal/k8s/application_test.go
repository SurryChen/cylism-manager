package k8s

import (
	"context"
	"errors"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestApplicationResourceApplierCreatesAndUpdatesTypedResources(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset()}
	applier := NewApplicationResourceApplier(client)
	ctx := context.Background()

	config := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "config", Namespace: "apps", Labels: map[string]string{ManagedByLabel: ManagedByValue}}, Data: map[string]string{"version": "1"}}
	if err := applier.ApplyConfigMap(ctx, config); err != nil {
		t.Fatal(err)
	}
	config.Data["version"] = "2"
	if err := applier.ApplyConfigMap(ctx, config); err != nil {
		t.Fatal(err)
	}
	gotConfig, err := client.Clientset.CoreV1().ConfigMaps("apps").Get(ctx, "config", metav1.GetOptions{})
	if err != nil || gotConfig.Data["version"] != "2" {
		t.Fatalf("configmap update failed: %#v %v", gotConfig, err)
	}

	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "apps", Labels: map[string]string{ManagedByLabel: ManagedByValue}}}
	statefulSet := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "apps", Labels: map[string]string{ManagedByLabel: ManagedByValue}}}
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "apps", Labels: map[string]string{ManagedByLabel: ManagedByValue}}}
	ingress := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "apps", Labels: map[string]string{ManagedByLabel: ManagedByValue}}}
	for name, apply := range map[string]func() error{
		"deployment":  func() error { return applier.ApplyDeployment(ctx, deployment) },
		"statefulset": func() error { return applier.ApplyStatefulSet(ctx, statefulSet) },
		"service":     func() error { return applier.ApplyService(ctx, service) },
		"ingress":     func() error { return applier.ApplyIngress(ctx, ingress) },
	} {
		if err := apply(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	service.Spec.ClusterIP = "10.0.0.10"
	if err := client.Clientset.CoreV1().Services("apps").Delete(ctx, "web", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Clientset.CoreV1().Services("apps").Create(ctx, service, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	service.Spec.ClusterIP = ""
	if err := applier.ApplyService(ctx, service); err != nil {
		t.Fatal(err)
	}
	gotService, err := client.Clientset.CoreV1().Services("apps").Get(ctx, "web", metav1.GetOptions{})
	if err != nil || gotService.Spec.ClusterIP != "10.0.0.10" {
		t.Fatalf("cluster IP was not preserved: %#v %v", gotService, err)
	}
}

func TestApplicationResourceApplierRejectsUnmanagedExistingResource(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "apps", Labels: map[string]string{"owner": "other"}}})}
	err := NewApplicationResourceApplier(client).ApplySecret(context.Background(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "apps"}})
	if err == nil || !strings.Contains(err.Error(), "不受 Cylism Manager 管理") {
		t.Fatalf("expected ownership error, got %v", err)
	}
}

func TestApplicationResourceApplierCertificateUsesDynamicClient(t *testing.T) {
	listKinds := map[schema.GroupVersionResource]string{applicationCertificateGVR: "CertificateList"}
	client := &Client{Clientset: k8sfake.NewSimpleClientset(), DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds)}
	desired := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "cert-manager.io/v1", "kind": "Certificate", "metadata": map[string]interface{}{"name": "site", "namespace": "apps", "labels": map[string]interface{}{ManagedByLabel: ManagedByValue}}}}
	if err := NewApplicationResourceApplier(client).ApplyCertificate(context.Background(), desired); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DynamicClient.Resource(applicationCertificateGVR).Namespace("apps").Get(context.Background(), "site", metav1.GetOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestApplicationResourceApplierRequiresClient(t *testing.T) {
	if err := NewApplicationResourceApplier(nil).ApplyConfigMap(context.Background(), &corev1.ConfigMap{}); err == nil {
		t.Fatal("expected uninitialized client error")
	}
}

func TestApplicationWorkloadAndEndpointAdaptersDelegateToClientset(t *testing.T) {
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "apps"}}
	statefulSet := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "apps"}}
	ingress := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "apps"}}
	client := &Client{Clientset: k8sfake.NewSimpleClientset(deployment, statefulSet, ingress)}
	workloads := NewApplicationWorkloadControllerAdapter(client)
	if _, err := workloads.GetDeployment(context.Background(), "apps", "web"); err != nil {
		t.Fatal(err)
	}
	deployment.Spec.Replicas = int32Ptr(2)
	if _, err := workloads.UpdateDeployment(context.Background(), deployment); err != nil {
		t.Fatal(err)
	}
	if _, err := workloads.GetStatefulSet(context.Background(), "apps", "db"); err != nil {
		t.Fatal(err)
	}
	endpoints := NewApplicationEndpointAdapter(client)
	if _, err := endpoints.GetIngress(context.Background(), "apps", "web"); err != nil {
		t.Fatal(err)
	}
	if err := endpoints.DeleteIngress(context.Background(), "apps", "web"); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseDiagnosticsAdapterPropagatesContext(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	clientset.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.(k8stesting.ListAction).GetListRestrictions().Labels == nil {
			return true, nil, errors.New("missing selector")
		}
		return true, nil, context.Canceled
	})
	_, err := NewReleaseDiagnosticsAdapter(&Client{Clientset: clientset}).ListPods(context.Background(), "apps", "app=web")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context error propagation, got %v", err)
	}
}

func TestApplicationPreflightAdapterReadsCoreResources(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "apps"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "apps"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "apps"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}},
	)}
	adapter := NewApplicationPreflightAdapter(client)
	ctx := context.Background()
	if _, err := adapter.GetNamespace(ctx, "apps"); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.GetSecret(ctx, "apps", "s"); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.GetConfigMap(ctx, "apps", "c"); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.GetNode(ctx, "node-a"); err != nil {
		t.Fatal(err)
	}
}
