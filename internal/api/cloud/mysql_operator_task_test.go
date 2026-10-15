package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func operatorTaskHandler(t *testing.T, existing bool) (*DatabaseHandler, *k8s.Client, *model.MySQLCluster) {
	t.Helper()
	s, err := store.New(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	kube := model.Cluster{Name: "target", Distribution: "k3s", Status: "ready"}
	if err := s.DB().Create(&kube).Error; err != nil {
		t.Fatal(err)
	}
	item := &model.MySQLCluster{Name: "mysql", ClusterID: kube.ID, Namespace: "mysql", Status: "draft"}
	if err := s.DB().Create(item).Error; err != nil {
		t.Fatal(err)
	}
	objects := []runtime.Object{}
	if existing {
		objects = append(objects, &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "helm.cattle.io/v1", "kind": "HelmChart",
			"metadata": map[string]any{"name": "cylism-mysql-operator", "namespace": "kube-system", "uid": "operator-chart", "labels": map[string]any{k8s.ManagedByLabel: k8s.ManagedByValue}, "creationTimestamp": time.Now().UTC().Format(time.RFC3339)},
		}})
	}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(), DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		{Group: "mysql.oracle.com", Version: "v2", Resource: "innodbclusters"}: "InnoDBClusterList",
	}, objects...)}
	h := NewDatabaseHandler(s, nil, func(context.Context, uint) (*k8s.Client, error) { return client, nil })
	return h, client, item
}

func TestCancelOperatorTracksCleanupAndAllowsNewInstallation(t *testing.T) {
	h, client, item := operatorTaskHandler(t, true)
	ctx := context.Background()
	if _, err := h.syncMySQLOperatorTask(ctx, item, false); err != nil {
		t.Fatal(err)
	}
	var original model.MySQLClusterTask
	h.store.DB().First(&original)
	_, err := client.Clientset.BatchV1().Jobs("kube-system").Create(ctx, &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "helm-install-cylism-mysql-operator", Namespace: "kube-system", Labels: map[string]string{"helmcharts.helm.cattle.io/chart": "cylism-mysql-operator"}},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Real HelmChart deletion waits on the Helm controller's finalizer.
	dynamic := client.DynamicClient.(*dynamicfake.FakeDynamicClient)
	chartGVR := schema.GroupVersionResource{Group: "helm.cattle.io", Version: "v1", Resource: "helmcharts"}
	dynamic.PrependReactor("delete", "helmcharts", func(action k8stesting.Action) (bool, runtime.Object, error) {
		chart, err := dynamic.Tracker().Get(chartGVR, "kube-system", "cylism-mysql-operator")
		if err != nil {
			return true, nil, err
		}
		value := chart.(*unstructured.Unstructured).DeepCopy()
		now := metav1.Now()
		value.SetDeletionTimestamp(&now)
		return true, nil, dynamic.Tracker().Update(chartGVR, value, "kube-system")
	})
	r := gin.New()
	r.POST("/:id/cancel", h.CancelClusterOperator)
	r.POST("/:id/install", h.InstallClusterOperator)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("POST", fmt.Sprintf("/%d/cancel", item.ID), nil))
	var result struct {
		Code int
		Data struct{ State string }
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Code != 0 || result.Data.State != "cancelling" {
		t.Fatalf("cancel response: %s, %v", response.Body.String(), err)
	}
	if _, err := client.Clientset.BatchV1().Jobs("kube-system").Get(ctx, "helm-install-cylism-mysql-operator", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("installer job was not stopped: %v", err)
	}
	if err := dynamic.Tracker().Delete(chartGVR, "kube-system", "cylism-mysql-operator"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := h.syncMySQLOperatorTask(ctx, item, false); err != nil {
			t.Fatal(err)
		}
	}
	var cancelled model.MySQLClusterTask
	h.store.DB().First(&cancelled, original.ID)
	if cancelled.Status != "cancelled" {
		t.Fatalf("task was not cancelled: %+v", cancelled)
	}
	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("POST", fmt.Sprintf("/%d/install", item.ID), nil))
	var install struct {
		Code int
		Data struct{ Task model.MySQLClusterTask }
	}
	if err := json.Unmarshal(response.Body.Bytes(), &install); err != nil || install.Code != 0 || install.Data.Task.ID == original.ID || install.Data.Task.Status != "running" {
		t.Fatalf("reinstall did not create a fresh task: %s, %v", response.Body.String(), err)
	}
	h.store.DB().First(&cancelled, original.ID)
	if cancelled.Status != "cancelled" {
		t.Fatal("reinstall overwrote cancelled history")
	}
}

func TestCancelOperatorRejectsReadyInUseAndUnmanagedInstallations(t *testing.T) {
	for _, test := range []string{"ready", "in_use", "unmanaged"} {
		t.Run(test, func(t *testing.T) {
			h, client, item := operatorTaskHandler(t, true)
			ctx := context.Background()
			charts := client.DynamicClient.Resource(schema.GroupVersionResource{Group: "helm.cattle.io", Version: "v1", Resource: "helmcharts"}).Namespace("kube-system")
			switch test {
			case "ready":
				_, _ = client.Clientset.AppsV1().Deployments("mysql-operator").Create(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator"}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1}}, metav1.CreateOptions{})
			case "in_use":
				_, _ = client.DynamicClient.Resource(schema.GroupVersionResource{Group: "mysql.oracle.com", Version: "v2", Resource: "innodbclusters"}).Namespace("mysql").Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "mysql.oracle.com/v2", "kind": "InnoDBCluster", "metadata": map[string]any{"name": "existing", "namespace": "mysql"}}}, metav1.CreateOptions{})
			case "unmanaged":
				chart, _ := charts.Get(ctx, "cylism-mysql-operator", metav1.GetOptions{})
				chart.SetLabels(nil)
				_, _ = charts.Update(ctx, chart, metav1.UpdateOptions{})
			}
			r := gin.New()
			r.POST("/:id/cancel", h.CancelClusterOperator)
			response := httptest.NewRecorder()
			r.ServeHTTP(response, httptest.NewRequest("POST", fmt.Sprintf("/%d/cancel", item.ID), nil))
			var result struct{ Code int }
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Code == 0 {
				t.Fatalf("unsafe cancellation accepted: %s", response.Body.String())
			}
			if _, err := charts.Get(ctx, "cylism-mysql-operator", metav1.GetOptions{}); err != nil {
				t.Fatalf("rejected cancellation deleted chart: %v", err)
			}
		})
	}
}

func TestInstallOperatorCreatesTaskAndTracksReadiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, client, item := operatorTaskHandler(t, false)
	r := gin.New()
	r.POST("/:id/install", h.InstallClusterOperator)
	r.GET("/:id/tasks", h.ListMySQLClusterTasks)
	for i := 0; i < 2; i++ {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest("POST", fmt.Sprintf("/%d/install", item.ID), nil))
		var result struct {
			Code int
			Data struct {
				State string
				Task  model.MySQLClusterTask
			}
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Code != 0 || result.Data.State != "installing" || result.Data.Task.Kind != "install_operator" || result.Data.Task.ID == 0 {
			t.Fatalf("invalid install response: %s", response.Body.String())
		}
	}
	var count int64
	h.store.DB().Model(&model.MySQLClusterTask{}).Count(&count)
	if count != 1 {
		t.Fatalf("repeated install created %d tasks", count)
	}
	_, err := client.Clientset.AppsV1().Deployments("mysql-operator").Create(context.Background(), &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator", Namespace: "mysql-operator"}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("GET", fmt.Sprintf("/%d/tasks", item.ID), nil))
	var task model.MySQLClusterTask
	if err := h.store.DB().First(&task).Error; err != nil {
		t.Fatal(err)
	}
	if task.Status != "success" {
		t.Fatalf("installation did not complete: %s", response.Body.String())
	}
	var saved model.MySQLCluster
	h.store.DB().First(&saved, item.ID)
	if saved.Status != "draft" {
		t.Fatalf("Operator install changed database deployment state: %s", saved.Status)
	}
	chartGVR := schema.GroupVersionResource{Group: "helm.cattle.io", Version: "v1", Resource: "helmcharts"}
	if err := client.DynamicClient.Resource(chartGVR).Namespace("kube-system").Delete(context.Background(), "cylism-mysql-operator", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := client.Clientset.AppsV1().Deployments("mysql-operator").Delete(context.Background(), "mysql-operator", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("POST", fmt.Sprintf("/%d/install", item.ID), nil))
	var reinstall struct {
		Code int
		Data struct{ Task model.MySQLClusterTask }
	}
	if err := json.Unmarshal(response.Body.Bytes(), &reinstall); err != nil || reinstall.Code != 0 || reinstall.Data.Task.ID == task.ID {
		t.Fatalf("reinstall did not create a fresh task: %s, %v", response.Body.String(), err)
	}
	if err := h.store.DB().First(&task, task.ID).Error; err != nil || task.Status != "success" {
		t.Fatalf("reinstall changed the completed task: %+v, %v", task, err)
	}
	if err := h.store.DB().Model(&model.MySQLClusterTask{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("reinstall task count = %d, error = %v", count, err)
	}
}

func TestOperatorTaskRecoversExistingInstallAndRecordsJobFailure(t *testing.T) {
	h, client, item := operatorTaskHandler(t, true)
	if _, err := h.syncMySQLOperatorTask(context.Background(), item, false); err != nil {
		t.Fatal(err)
	}
	_, err := client.Clientset.BatchV1().Jobs("kube-system").Create(context.Background(), &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "helm-install-cylism-mysql-operator", Namespace: "kube-system"},
		Status:     batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "BackoffLimitExceeded", Message: "install failed"}}},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.syncMySQLOperatorTask(context.Background(), item, false); err != nil {
		t.Fatal(err)
	}
	var task model.MySQLClusterTask
	h.store.DB().First(&task)
	if task.Status != "failed" || task.Kind != "install_operator" {
		t.Fatalf("failed job not tracked: %+v", task)
	}
	var logs []model.MySQLClusterTaskLog
	h.store.DB().Where("task_id = ?", task.ID).Find(&logs)
	if len(logs) < 2 || logs[len(logs)-1].Message != "Helm 安装失败：BackoffLimitExceeded install failed" {
		t.Fatalf("failure log missing: %+v", logs)
	}
}

func TestUninstallOperatorTracksHelmCleanup(t *testing.T) {
	h, client, item := operatorTaskHandler(t, true)
	ctx := context.Background()
	_, err := client.Clientset.AppsV1().Deployments("mysql-operator").Create(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator", Namespace: "mysql-operator"}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.POST("/:id/uninstall", h.UninstallClusterOperator)
	r.GET("/:id/operator", h.ClusterOperator)
	r.GET("/:id/tasks", h.ListMySQLClusterTasks)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("POST", fmt.Sprintf("/%d/uninstall", item.ID), nil))
	var result struct {
		Code int
		Data struct{ Task model.MySQLClusterTask }
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Code != 0 || result.Data.Task.Kind != "uninstall_operator" || result.Data.Task.Status != "running" {
		t.Fatalf("uninstall response: %s, %v", response.Body.String(), err)
	}
	chartGVR := schema.GroupVersionResource{Group: "helm.cattle.io", Version: "v1", Resource: "helmcharts"}
	if _, err := client.DynamicClient.Resource(chartGVR).Namespace("kube-system").Get(ctx, "cylism-mysql-operator", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("HelmChart still exists: %v", err)
	}
	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("GET", fmt.Sprintf("/%d/operator", item.ID), nil))
	var status struct {
		Code int
		Data struct {
			State        string `json:"state"`
			CanUninstall bool   `json:"can_uninstall"`
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil || status.Code != 0 || status.Data.State != "cancelling" || status.Data.CanUninstall {
		t.Fatalf("pending uninstall status: %s, %v", response.Body.String(), err)
	}
	other := model.MySQLCluster{Name: "another-mysql", ClusterID: item.ClusterID, Namespace: "mysql", Status: "draft"}
	if err := h.store.DB().Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("GET", fmt.Sprintf("/%d/operator", other.ID), nil))
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil || status.Code != 0 || status.Data.State != "cancelling" {
		t.Fatalf("shared operator status during uninstall: %s, %v", response.Body.String(), err)
	}
	if err := client.Clientset.AppsV1().Deployments("mysql-operator").Delete(ctx, "mysql-operator", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("GET", fmt.Sprintf("/%d/tasks", item.ID), nil))
	var task model.MySQLClusterTask
	if err := h.store.DB().First(&task, result.Data.Task.ID).Error; err != nil || task.Status != "success" || task.LastProgress != "MySQL Operator 已卸载" {
		t.Fatalf("uninstall task did not finish: %+v, %v", task, err)
	}
	var logs []model.MySQLClusterTaskLog
	h.store.DB().Where("task_id = ?", task.ID).Find(&logs)
	if len(logs) != 2 {
		t.Fatalf("uninstall logs: %+v", logs)
	}
}

func TestOperatorUninstallAvailabilityFollowsClusterResources(t *testing.T) {
	h, client, item := operatorTaskHandler(t, true)
	ctx := context.Background()
	_, err := client.Clientset.AppsV1().Deployments("mysql-operator").Create(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator", Namespace: "mysql-operator"}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.GET("/:id/operator", h.ClusterOperator)
	check := func(want bool) {
		t.Helper()
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest("GET", fmt.Sprintf("/%d/operator", item.ID), nil))
		var status struct {
			Code int
			Data struct {
				CanUninstall bool `json:"can_uninstall"`
			}
		}
		if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil || status.Code != 0 || status.Data.CanUninstall != want {
			t.Fatalf("operator availability: %s, want can_uninstall=%t, error=%v", response.Body.String(), want, err)
		}
	}
	check(true)
	clusters := client.DynamicClient.Resource(schema.GroupVersionResource{Group: "mysql.oracle.com", Version: "v2", Resource: "innodbclusters"}).Namespace("another-namespace")
	_, err = clusters.Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "mysql.oracle.com/v2", "kind": "InnoDBCluster", "metadata": map[string]any{"name": "in-use", "namespace": "another-namespace"}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	check(false)
	if err := clusters.Delete(ctx, "in-use", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	check(true)
}

func TestUninstallOperatorRejectsActiveClustersAndUnmanagedChart(t *testing.T) {
	for _, name := range []string{"active_cluster", "unmanaged_chart"} {
		t.Run(name, func(t *testing.T) {
			h, client, item := operatorTaskHandler(t, true)
			ctx := context.Background()
			_, err := client.Clientset.AppsV1().Deployments("mysql-operator").Create(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator", Namespace: "mysql-operator"}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1}}, metav1.CreateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if name == "active_cluster" {
				_, err = client.DynamicClient.Resource(schema.GroupVersionResource{Group: "mysql.oracle.com", Version: "v2", Resource: "innodbclusters"}).Namespace("another-namespace").Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "mysql.oracle.com/v2", "kind": "InnoDBCluster", "metadata": map[string]any{"name": "in-use", "namespace": "another-namespace"}}}, metav1.CreateOptions{})
			} else {
				charts := client.DynamicClient.Resource(schema.GroupVersionResource{Group: "helm.cattle.io", Version: "v1", Resource: "helmcharts"}).Namespace("kube-system")
				chart, getErr := charts.Get(ctx, "cylism-mysql-operator", metav1.GetOptions{})
				if getErr != nil {
					t.Fatal(getErr)
				}
				chart.SetLabels(nil)
				_, err = charts.Update(ctx, chart, metav1.UpdateOptions{})
			}
			if err != nil {
				t.Fatal(err)
			}
			r := gin.New()
			r.POST("/:id/uninstall", h.UninstallClusterOperator)
			response := httptest.NewRecorder()
			r.ServeHTTP(response, httptest.NewRequest("POST", fmt.Sprintf("/%d/uninstall", item.ID), nil))
			var result struct{ Code int }
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Code == 0 {
				t.Fatalf("unsafe uninstall accepted: %s, %v", response.Body.String(), err)
			}
			var count int64
			h.store.DB().Model(&model.MySQLClusterTask{}).Where("kind = ?", "uninstall_operator").Count(&count)
			if count != 0 {
				t.Fatalf("rejected uninstall created %d tasks", count)
			}
		})
	}
}
