package cloud

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestValidateMySQLTerminalPod(t *testing.T) {
	item := &model.MySQLCluster{Namespace: "databases", ResourceName: "mysql-1", Status: "ready"}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "mysql-1-0", Namespace: "databases", Labels: map[string]string{"mysql.oracle.com/cluster": "mysql-1", "component": "mysqld"}},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "mysql"}, {Name: "sidecar"}}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
	}
	if err := validateMySQLTerminalPod(item, pod); err != nil {
		t.Fatalf("ready MySQL pod rejected: %v", err)
	}
	other := pod.DeepCopy()
	other.Labels["mysql.oracle.com/cluster"] = "other"
	if err := validateMySQLTerminalPod(item, other); err == nil {
		t.Fatal("pod from another cluster accepted")
	}
	notReady := pod.DeepCopy()
	notReady.Status.Conditions[0].Status = corev1.ConditionFalse
	if err := validateMySQLTerminalPod(item, notReady); err == nil {
		t.Fatal("unready pod accepted")
	}
	item.Status = "paused"
	if err := validateMySQLTerminalPod(item, pod); err == nil {
		t.Fatal("paused cluster accepted")
	}
}

func TestMySQLTerminalRejectsForeignPodBeforeUpgrade(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:mysql-terminal-handler-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	item := model.MySQLCluster{Name: "test", ClusterID: 1, Namespace: "databases", ResourceName: "mysql-1", Status: "ready"}
	if err := s.DB().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "mysql-1-0", Namespace: "databases", Labels: map[string]string{"mysql.oracle.com/cluster": "other", "component": "mysqld"}},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "mysql"}}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
	})}
	h := NewDatabaseHandler(s, bytes.Repeat([]byte("a"), 32), func(context.Context, uint) (*k8s.Client, error) { return client, nil })
	router := gin.New()
	router.GET("/clusters/:id/pods/:podName/terminal", h.MySQLClusterTerminal)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/clusters/1/pods/mysql-1-0/terminal", nil))
	if response.Code == http.StatusOK || !strings.Contains(response.Body.String(), "不属于当前 MySQL 集群") {
		t.Fatalf("foreign pod accepted: %d %s", response.Code, response.Body.String())
	}
}

func TestMySQLTerminalHandshakeDoesNotExposeCredentials(t *testing.T) {
	if strings.Contains(mysqlTerminalCommand, "private-password") || strings.Contains(mysqlTerminalCommand, "-pprivate") {
		t.Fatal("credentials embedded in exec command")
	}
	output := &mysqlTerminalOutput{ready: make(chan struct{})}
	marker := []byte(mysqlTerminalReadyMarker)
	if _, err := output.Write(marker[:7]); err != nil {
		t.Fatal(err)
	}
	if _, err := output.Write(marker[7:]); err != nil {
		t.Fatal(err)
	}
	select {
	case <-output.ready:
	default:
		t.Fatal("split handshake marker was not recognized")
	}
	if len(output.pending) != 0 {
		t.Fatal("handshake marker remains in terminal output")
	}
}
