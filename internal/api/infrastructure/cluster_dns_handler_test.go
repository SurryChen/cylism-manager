package infrastructure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func setupClusterDNSRouter(t *testing.T) (*gin.Engine, *store.Store, *k8s.Client) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: coreDNSConfigMap, Namespace: coreDNSNamespace}, Data: map[string]string{"Corefile": ".:53 {\n  forward . /etc/resolv.conf\n}\n"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "coredns-a", Namespace: coreDNSNamespace, Labels: map[string]string{"k8s-app": "kube-dns"}}, Spec: corev1.PodSpec{NodeName: "node-a"}, Status: corev1.PodStatus{PodIP: "10.42.0.10", Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}},
	)}
	h := NewClusterDNSHandlerWithAdapter(s, NewClusterDNSAdapter(client))
	router := gin.New()
	group := router.Group("/api/cluster-dns")
	group.GET("", h.Status)
	group.POST("", h.Apply)
	group.DELETE("", h.Reset)
	group.POST("/history/:revision/rollback", h.Rollback)
	return router, s, client
}

func TestClusterDNSApplyReplacesOnlyForwardDirectiveAndRetainsHistory(t *testing.T) {
	router, s, client := setupClusterDNSRouter(t)
	response := serve(router, newJSONRequest(http.MethodPost, "/api/cluster-dns", gin.H{"resolvers": []string{"1.1.1.1", "8.8.8.8"}}))
	if response.Code != http.StatusOK {
		t.Fatalf("apply status = %d: %s", response.Code, response.Body.String())
	}
	configMap, err := client.Clientset.CoreV1().ConfigMaps(coreDNSNamespace).Get(context.Background(), coreDNSConfigMap, metav1.GetOptions{})
	if err != nil || !strings.Contains(configMap.Data["Corefile"], "forward . 1.1.1.1 8.8.8.8") || !strings.Contains(configMap.Data["Corefile"], ".:53 {") {
		t.Fatalf("unexpected Corefile: %v %#v", err, configMap)
	}
	policy, err := s.GetActiveClusterDNSPolicy()
	if err != nil || policy.Revision != 1 || policy.Resolvers != `["1.1.1.1","8.8.8.8"]` {
		t.Fatalf("unexpected policy: %#v err=%v", policy, err)
	}
}

func TestClusterDNSRejectsNonIPResolversWithoutMutation(t *testing.T) {
	router, _, client := setupClusterDNSRouter(t)
	response := serve(router, newJSONRequest(http.MethodPost, "/api/cluster-dns", gin.H{"resolvers": []string{"resolver.example.com"}}))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	configMap, _ := client.Clientset.CoreV1().ConfigMaps(coreDNSNamespace).Get(context.Background(), coreDNSConfigMap, metav1.GetOptions{})
	if !strings.Contains(configMap.Data["Corefile"], "/etc/resolv.conf") {
		t.Fatalf("Corefile changed after invalid request: %s", configMap.Data["Corefile"])
	}
}

func TestClusterDNSResetRestoresHostResolver(t *testing.T) {
	router, s, client := setupClusterDNSRouter(t)
	response := serve(router, newJSONRequest(http.MethodPost, "/api/cluster-dns", gin.H{"resolvers": []string{"1.1.1.1"}}))
	if response.Code != http.StatusOK {
		t.Fatalf("apply status = %d: %s", response.Code, response.Body.String())
	}
	response = serve(router, httptest.NewRequest(http.MethodDelete, "/api/cluster-dns", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("reset status = %d: %s", response.Code, response.Body.String())
	}
	configMap, err := client.Clientset.CoreV1().ConfigMaps(coreDNSNamespace).Get(context.Background(), coreDNSConfigMap, metav1.GetOptions{})
	if err != nil || !strings.Contains(configMap.Data["Corefile"], "forward . /etc/resolv.conf") {
		t.Fatalf("expected host resolver forwarding: %v %#v", err, configMap)
	}
	policy, err := s.GetActiveClusterDNSPolicy()
	if err != nil || policy.Resolvers != "[]" {
		t.Fatalf("expected inherited policy marker: %#v err=%v", policy, err)
	}
}
