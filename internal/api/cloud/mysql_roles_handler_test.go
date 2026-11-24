package cloud

import (
	"context"
	"fmt"
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

func TestMySQLRolesEndpointDoesNotSyncTasksOrReturnPodLabels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:mysql-roles-endpoint?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	item := model.MySQLCluster{Name: "roles-test", ClusterID: 1, Namespace: "mysql", ResourceName: "database", Status: "ready"}
	if err := s.DB().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "database-0", Namespace: "mysql", Labels: map[string]string{"mysql.oracle.com/cluster": "database", "component": "mysqld", "mysql.oracle.com/cluster-role": "PRIMARY"}}, Status: corev1.PodStatus{PodIP: "10.42.1.1", Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "database-root", Namespace: "mysql"}, Data: map[string][]byte{"rootPassword": []byte("private-password")}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator-test", Namespace: "mysql-operator"}, Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}},
	)}
	calls := 0
	h := NewDatabaseHandler(s, nil, func(context.Context, uint) (*k8s.Client, error) { calls++; return client, nil })
	router := gin.New()
	router.GET("/:id/roles", h.MySQLClusterRoles)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/%d/roles", item.ID), nil))
	// No exec channel: report unknown instead of returning the PRIMARY label.
	if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "PRIMARY") || strings.Contains(response.Body.String(), "private-password") {
		t.Fatalf("unsafe role response: %d %s", response.Code, response.Body.String())
	}
	if err := s.DB().First(&item, item.ID).Error; err != nil {
		t.Fatal(err)
	}
	if item.Status != "ready" {
		t.Fatal("read-only role query changed cluster status")
	}
	var taskCount int64
	if err := s.DB().Model(&model.MySQLClusterTask{}).Count(&taskCount).Error; err != nil || taskCount != 0 {
		t.Fatal("role query created tasks")
	}
	for _, status := range []string{"draft", "paused"} {
		if err := s.DB().Model(&item).Update("status", status).Error; err != nil {
			t.Fatal(err)
		}
		response = httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/%d/roles", item.ID), nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"data":[]`) || calls != 1 {
			t.Fatalf("inactive cluster queried Kubernetes: %d %s", response.Code, response.Body.String())
		}
	}
}
