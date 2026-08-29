package k8s

import (
	"context"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestRegistryProxyReconcilerApplyCreatesNodePortResources(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}})
	reconciler := NewRegistryProxyReconciler(&Client{Clientset: clientset})
	proxy := &model.RegistryProxy{ID: 1, ResourceName: "cylism-registry-proxy-1", Registry: "docker.io", NodeName: "node-a", NodePort: 30500, CacheLimitGi: 2, DNSResolvers: `["10.0.0.2"]`}
	if err := reconciler.EnsureNode(context.Background(), proxy.NodeName); err != nil {
		t.Fatal(err)
	}
	if err := reconciler.Apply(context.Background(), proxy, map[string]string{"REGISTRY_PROXY_REMOTEURL": "https://registry-1.docker.io", "REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY": "/var/lib/registry"}); err != nil {
		t.Fatal(err)
	}
	deployment, err := clientset.AppsV1().Deployments(RegistryProxyNamespace).Get(context.Background(), proxy.ResourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if deployment.Spec.Template.Spec.DNSPolicy != corev1.DNSNone || deployment.Spec.Template.Spec.DNSConfig == nil {
		t.Fatalf("expected configured pod DNS, got %#v", deployment.Spec.Template.Spec)
	}
	service, err := clientset.CoreV1().Services(RegistryProxyNamespace).Get(context.Background(), proxy.ResourceName, metav1.GetOptions{})
	if err != nil || service.Spec.Ports[0].NodePort != 30500 {
		t.Fatalf("unexpected service: %#v, %v", service, err)
	}
}

func TestParseRegistryProxyDiagnosticClassifiesProbeResults(t *testing.T) {
	tests := []struct {
		name, output, status string
	}{
		{"busybox wget registry response", "status=healthy;tool=wget;ips=52.4.106.100,;http=401;elapsed=35", "healthy"},
		{"connect timeout", "status=upstream_connect_timeout;tool=wget;ips=52.4.106.100,;http=;elapsed=10020", "upstream_connect_timeout"},
		{"missing command", "status=command_missing;tool=missing;ips=52.4.106.100,;http=;elapsed=1", "command_missing"},
		{"invalid output", "not a diagnostic", "diagnostic_failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := parseRegistryProxyDiagnostic(test.output, 999)
			if result.Status != test.status {
				t.Fatalf("status = %q, want %q: %#v", result.Status, test.status, result)
			}
		})
	}
}

func TestRegistryProxyDiagnosticCommandRecordsCurlStatus(t *testing.T) {
	command := registryProxyDiagnosticCommand("registry-1.docker.io")
	if !strings.Contains(command[2], `http=$output;`) {
		t.Fatalf("curl result must be recorded as HTTP status: %q", command[2])
	}
}
