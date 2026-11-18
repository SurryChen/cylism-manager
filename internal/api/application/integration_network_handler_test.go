package applicationapi

import (
	"context"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/service/application"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

type serverAddressFixture []model.Server

func (s serverAddressFixture) ListServers() ([]model.Server, error) { return s, nil }

func TestApplicationPublicHostRejectsOverlayAndUsesScopedInventory(t *testing.T) {
	clusterID, otherClusterID := uint(7), uint(8)
	app := &model.Application{Environment: model.Environment{ClusterID: clusterID}}
	spec := application.ReleaseSpec{HostNetwork: true, NodeName: "cloudzone"}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: spec.NodeName}, Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "100.64.128.76"}}}}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(node)}
	h := (&ApplicationHandler{}).WithClusterClientFactory(func(context.Context, uint) (*k8s.Client, error) { return client, nil })
	if got := h.applicationPublicHost(context.Background(), app, spec); got != "" {
		t.Fatalf("overlay address exported: %q", got)
	}
	h.WithServerAddressReader(serverAddressFixture{
		{Host: "8.8.8.8", ClusterID: &otherClusterID, K8sNodeName: spec.NodeName},
		{Host: "1.1.1.1", ClusterID: &clusterID, K8sNodeName: "other-node"},
		{Host: "117.55.235.170", ClusterID: &clusterID, K8sNodeName: spec.NodeName},
	})
	if got := h.applicationPublicHost(context.Background(), app, spec); got != "117.55.235.170" {
		t.Fatalf("public server host=%q", got)
	}
	h.WithServerAddressReader(serverAddressFixture{
		{Host: "117.55.235.170", ClusterID: &clusterID, K8sNodeName: spec.NodeName},
		{Host: "9.9.9.9", ClusterID: &clusterID, K8sNodeName: spec.NodeName},
	})
	if got := h.applicationPublicHost(context.Background(), app, spec); got != "" {
		t.Fatalf("conflicting inventory returned %q", got)
	}
	node.Status.Addresses = append(node.Status.Addresses, corev1.NodeAddress{Type: corev1.NodeExternalIP, Address: "9.9.9.9"})
	if _, err := client.Clientset.CoreV1().Nodes().UpdateStatus(context.Background(), node, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := h.applicationPublicHost(context.Background(), app, spec); got != "9.9.9.9" {
		t.Fatalf("external IP not preferred: %q", got)
	}
	spec.HostNetwork = false
	if got := h.applicationPublicHost(context.Background(), app, spec); got != "" {
		t.Fatalf("non-host-network endpoint exported: %q", got)
	}
}

func TestApplicationPublicIPRejectsSharedAddressSpace(t *testing.T) {
	for _, value := range []string{"100.64.128.76", "100.64.0.0", "100.127.255.255", "::ffff:100.64.128.76", "10.0.0.1", "127.0.0.1", "169.254.1.1", "fd00::1", "::1", "not-an-ip"} {
		if isPublicApplicationIP(value) {
			t.Errorf("non-public address accepted: %s", value)
		}
	}
	for _, value := range []string{"117.55.235.170", "100.63.255.255", "100.128.0.0", "2606:4700:4700::1111"} {
		if !isPublicApplicationIP(value) {
			t.Errorf("public address rejected: %s", value)
		}
	}
}
