package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func itoa(id uint) string { return strconv.FormatUint(uint64(id), 10) }

func TestCrossServiceCreateCheckAndDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	st, err := store.New("file:cross-service-roundtrip?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	source := &model.Cluster{Name: "source", Distribution: "k3s", Status: "ready"}
	target := &model.Cluster{Name: "target", Distribution: "k3s", Status: "ready"}
	if err := st.CreateCluster(source); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateCluster(target); err != nil {
		t.Fatal(err)
	}
	sourceNode := &model.Server{Name: "source-node", Host: "source.example.test", ClusterID: &source.ID, ClusterRole: "control-plane"}
	targetNode := &model.Server{Name: "target-node", Host: "target.example.test", ClusterID: &target.ID, ClusterRole: "control-plane"}
	if err := st.CreateServer(sourceNode); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateServer(targetNode); err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Model(source).Update("control_server_id", sourceNode.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Model(target).Update("control_server_id", targetNode.ID).Error; err != nil {
		t.Fatal(err)
	}
	sourceClient := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}})
	targetClient := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"}, Spec: corev1.ServiceSpec{ClusterIP: "10.43.0.25", Ports: []corev1.ServicePort{{Port: 8080, Protocol: corev1.ProtocolTCP}}}})
	clients := map[uint]*k8s.Client{source.ID: {Clientset: sourceClient}, target.ID: {Clientset: targetClient}}
	h := NewCrossServiceHandler(st, nil, func(_ context.Context, id uint) (*k8s.Client, error) { return clients[id], nil })
	h.status = func(_ context.Context, server *model.Server, _ []byte) tailscaleStatus {
		ip := "100.64.0.1"
		if server.ID == targetNode.ID {
			ip = "100.64.0.2"
		}
		return tailscaleStatus{Status: "online", IP: ip}
	}
	commands := []string{}
	h.ssh = func(_ context.Context, _ time.Duration, _ *model.Server, _ []byte, command string) ([]byte, error) {
		commands = append(commands, command)
		if strings.Contains(command, "command -v systemd-socket-proxyd") {
			return []byte("/usr/lib/systemd/systemd-socket-proxyd"), nil
		}
		return nil, nil
	}
	router := gin.New()
	router.GET("/cross-cluster-services", h.ListAll)
	router.POST("/clusters/:id/cross-services", h.Create)
	router.GET("/clusters/:id/cross-services/:serviceID/check", h.Check)
	router.DELETE("/clusters/:id/cross-services/:serviceID", h.Delete)
	request := map[string]any{"target_cluster_id": target.ID, "source_namespace": "default", "source_name": "orders-b", "source_port": 80, "target_namespace": "default", "target_name": "orders", "target_service_port": 8080}
	body, _ := json.Marshal(request)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/clusters/"+itoa(source.ID)+"/cross-services", bytes.NewReader(body)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"code":0`) {
		t.Fatalf("create: %d %s", response.Code, response.Body.String())
	}
	items, err := st.ListCrossClusterServices(source.ID)
	if err != nil || len(items) != 1 || items[0].Status != "active" {
		t.Fatalf("stored items: %+v, %v", items, err)
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/cross-cluster-services", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"source_cluster_id":`+itoa(source.ID)) || !strings.Contains(response.Body.String(), `"target_cluster_id":`+itoa(target.ID)) {
		t.Fatalf("global list: %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(strings.Join(commands, "\n"), "ListenStream=100.64.0.2:20001") || !strings.Contains(strings.Join(commands, "\n"), "10.43.0.25:8080") {
		t.Fatalf("gateway not bound to Tailscale and target service: %v", commands)
	}
	service, err := sourceClient.CoreV1().Services("default").Get(t.Context(), "orders-b", metav1.GetOptions{})
	if err != nil || service.Annotations["cylism.io/cross-service-id"] != itoa(items[0].ID) {
		t.Fatalf("source service: %+v, %v", service, err)
	}
	slice, err := sourceClient.DiscoveryV1().EndpointSlices("default").Get(t.Context(), "orders-b-cross", metav1.GetOptions{})
	if err != nil || slice.Endpoints[0].Addresses[0] != "100.64.0.2" || *slice.Ports[0].Port != items[0].GatewayPort {
		t.Fatalf("source endpoint: %+v, %v", slice, err)
	}
	path := "/clusters/" + itoa(source.ID) + "/cross-services/" + itoa(items[0].ID)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path+"/check", nil))
	if !strings.Contains(response.Body.String(), "node_reachable") {
		t.Fatalf("check: %s", response.Body.String())
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, path, nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"code":0`) {
		t.Fatalf("delete: %d %s", response.Code, response.Body.String())
	}
	items, err = st.ListCrossClusterServices(source.ID)
	if err != nil || len(items) != 0 {
		t.Fatalf("records not deleted: %+v, %v", items, err)
	}
	if _, err := sourceClient.CoreV1().Services("default").Get(t.Context(), "orders-b", metav1.GetOptions{}); err == nil {
		t.Fatal("source service not deleted")
	}
	failureCommands := []string{}
	h.ssh = func(_ context.Context, _ time.Duration, _ *model.Server, _ []byte, command string) ([]byte, error) {
		failureCommands = append(failureCommands, command)
		if strings.Contains(command, "command -v systemd-socket-proxyd") {
			return []byte("/usr/lib/systemd/systemd-socket-proxyd"), nil
		}
		if strings.Contains(command, "ListenStream=") {
			return []byte("failed to bind"), context.DeadlineExceeded
		}
		return nil, nil
	}
	request["source_name"] = "orders-c"
	body, _ = json.Marshal(request)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/clusters/"+itoa(source.ID)+"/cross-services", bytes.NewReader(body)))
	if !strings.Contains(response.Body.String(), "目标节点启动私网入口失败") {
		t.Fatalf("expected gateway failure: %s; commands: %v", response.Body.String(), failureCommands)
	}
	items, err = st.ListCrossClusterServices(source.ID)
	if err != nil || len(items) != 1 || items[0].Status != "failed" || items[0].LastError == "" {
		t.Fatalf("expected failed cleanup record: %+v, %v", items, err)
	}
	if _, err := sourceClient.CoreV1().Services("default").Get(t.Context(), "orders-c", metav1.GetOptions{}); err == nil {
		t.Fatal("failed gateway left an active source Service")
	}
}

func TestCrossServiceCleanupDoesNotDeleteUnownedService(t *testing.T) {
	clientset := fake.NewSimpleClientset(&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "orders-b", Namespace: "default"}})
	h := &CrossServiceHandler{}
	err := h.removeSourceResources(context.Background(), &k8s.Client{Clientset: clientset}, &model.CrossClusterService{ID: 7, SourceNamespace: "default", SourceName: "orders-b"})
	if err == nil || !strings.Contains(err.Error(), "所有权") {
		t.Fatalf("expected ownership error, got %v", err)
	}
	if _, err := clientset.CoreV1().Services("default").Get(t.Context(), "orders-b", metav1.GetOptions{}); err != nil {
		t.Fatal(err)
	}
}
