package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

const shutdownRetryWait = 3 * time.Minute
const shutdownRetryCooldown = 5 * time.Minute
const operatorRestartAnnotation = "kubectl.kubernetes.io/restartedAt"

type ClusterCleanupState struct {
	ClusterResource   bool         `json:"cluster_resource"`
	DeletionStartedAt *metav1.Time `json:"deletion_started_at,omitempty"`
	Finalizers        []string     `json:"finalizers"`
	RetryAvailable    bool         `json:"retry_available"`
	RecentRestart     bool         `json:"recent_restart"`
	Workloads         []string     `json:"workloads"`
	Pods              []string     `json:"pods"`
	Claims            []string     `json:"claims"`
}

func InspectClusterCleanup(ctx context.Context, client *k8s.Client, item *model.MySQLCluster) (*ClusterCleanupState, error) {
	state := &ClusterCleanupState{Finalizers: []string{}, Workloads: []string{}, Pods: []string{}, Claims: []string{}}
	resource := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace)
	if cluster, err := resource.Get(ctx, item.ResourceName, metav1.GetOptions{}); err == nil {
		state.ClusterResource = true
		state.DeletionStartedAt = cluster.GetDeletionTimestamp()
		state.Finalizers = cluster.GetFinalizers()
		if state.DeletionStartedAt != nil && len(state.Finalizers) > 0 && time.Since(state.DeletionStartedAt.Time) >= shutdownRetryWait {
			deployment, err := client.Clientset.AppsV1().Deployments("mysql-operator").Get(ctx, "mysql-operator", metav1.GetOptions{})
			if err == nil {
				state.RecentRestart = recentOperatorRestart(deployment.Spec.Template.Annotations[operatorRestartAnnotation], time.Now())
				state.RetryAvailable = !state.RecentRestart
			} else if !apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("检查 MySQL Operator 状态失败：%w", err)
			}
		}
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("检查 MySQL 集群资源失败：%w", err)
	}
	sets, err := client.Clientset.AppsV1().StatefulSets(item.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("检查数据库工作负载失败：%w", err)
	}
	for _, set := range sets.Items {
		if clusterResourceName(set.Name, item.ResourceName) || set.Labels["mysql.oracle.com/cluster"] == item.ResourceName {
			state.Workloads = append(state.Workloads, set.Name)
		}
	}
	deployments, err := client.Clientset.AppsV1().Deployments(item.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("检查 Router 工作负载失败：%w", err)
	}
	for _, deployment := range deployments.Items {
		if clusterResourceName(deployment.Name, item.ResourceName) || deployment.Labels["mysql.oracle.com/cluster"] == item.ResourceName {
			state.Workloads = append(state.Workloads, deployment.Name)
		}
	}
	pods, err := client.Clientset.CoreV1().Pods(item.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("检查数据库 Pod 失败：%w", err)
	}
	for _, pod := range pods.Items {
		if clusterResourceName(pod.Name, item.ResourceName) || pod.Labels["mysql.oracle.com/cluster"] == item.ResourceName {
			state.Pods = append(state.Pods, pod.Name)
		}
	}
	claims, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("检查数据库数据卷失败：%w", err)
	}
	for _, claim := range claims.Items {
		if strings.HasPrefix(claim.Name, "datadir-"+item.ResourceName+"-") || strings.HasPrefix(claim.Name, "data-"+item.ResourceName+"-") || claim.Labels["mysql.oracle.com/cluster"] == item.ResourceName {
			state.Claims = append(state.Claims, claim.Name)
		}
	}
	return state, nil
}

func recentOperatorRestart(value string, now time.Time) bool {
	started, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && now.Sub(started) < shutdownRetryCooldown
}

// RetryClusterShutdown restarts the shared Operator after a stalled deletion.
// The Operator still owns its finalizer and the database PVCs remain untouched.
func RetryClusterShutdown(ctx context.Context, client *k8s.Client, item *model.MySQLCluster) error {
	cluster, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("读取待清理 MySQL 集群失败：%w", err)
	}
	if cluster.GetLabels()[k8s.ManagedByLabel] != k8s.ManagedByValue {
		return fmt.Errorf("MySQL 集群资源不受平台管理，不能重试清理")
	}
	if cluster.GetDeletionTimestamp() == nil || len(cluster.GetFinalizers()) == 0 || time.Since(cluster.GetDeletionTimestamp().Time) < shutdownRetryWait {
		return fmt.Errorf("Operator 尚在正常清理等待期，请稍后再试")
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		deployment, err := client.Clientset.AppsV1().Deployments("mysql-operator").Get(ctx, "mysql-operator", metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("读取 MySQL Operator 工作负载失败：%w", err)
		}
		if recentOperatorRestart(deployment.Spec.Template.Annotations[operatorRestartAnnotation], time.Now()) {
			return fmt.Errorf("MySQL Operator 最近已重启，请等待清理结果")
		}
		deployment = deployment.DeepCopy()
		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = map[string]string{}
		}
		deployment.Spec.Template.Annotations[operatorRestartAnnotation] = time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := client.Clientset.AppsV1().Deployments("mysql-operator").Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("重启 MySQL Operator 失败：%w", err)
		}
		return nil
	})
}

func PauseCluster(ctx context.Context, client *k8s.Client, item *model.MySQLCluster) error {
	resource := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace)
	current, err := resource.Get(ctx, item.ResourceName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("检查 MySQL 集群资源失败：%w", err)
	}
	if current.GetLabels()[k8s.ManagedByLabel] != k8s.ManagedByValue {
		return fmt.Errorf("MySQL 集群资源不受平台管理，不能暂停")
	}
	policy := metav1.DeletePropagationBackground
	if err := resource.Delete(ctx, item.ResourceName, metav1.DeleteOptions{PropagationPolicy: &policy}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("停止 MySQL Operator 重试失败：%w", err)
	}
	return nil
}

// CleanupPausedCluster deletes only resources named or labelled for this
// cluster. PVCs are removed only after every controller and Pod has exited.
func CleanupPausedCluster(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, deleteClaims bool) (*ClusterCleanupState, error) {
	state, err := InspectClusterCleanup(ctx, client, item)
	if err != nil {
		return nil, err
	}
	if state.ClusterResource {
		return state, nil
	}
	policy := metav1.DeletePropagationBackground
	for _, name := range state.Workloads {
		if name == item.ResourceName {
			err = client.Clientset.AppsV1().StatefulSets(item.Namespace).Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &policy})
		} else {
			// Router deployments and read-replica StatefulSets can share the prefix.
			if _, getErr := client.Clientset.AppsV1().StatefulSets(item.Namespace).Get(ctx, name, metav1.GetOptions{}); getErr == nil {
				err = client.Clientset.AppsV1().StatefulSets(item.Namespace).Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &policy})
			} else if apierrors.IsNotFound(getErr) {
				err = client.Clientset.AppsV1().Deployments(item.Namespace).Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &policy})
			} else {
				err = getErr
			}
		}
		if err != nil && !apierrors.IsNotFound(err) {
			return state, fmt.Errorf("删除残留工作负载 %s: %w", name, err)
		}
	}
	if len(state.Workloads) > 0 {
		return state, nil
	}
	for _, name := range state.Pods {
		if err := client.Clientset.CoreV1().Pods(item.Namespace).Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &policy}); err != nil && !apierrors.IsNotFound(err) {
			return state, fmt.Errorf("删除残留 Pod %s: %w", name, err)
		}
	}
	if len(state.Pods) > 0 {
		return state, nil
	}
	if !deleteClaims {
		return state, nil
	}
	for _, name := range state.Claims {
		if err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return state, fmt.Errorf("删除数据卷 %s: %w", name, err)
		}
	}
	return InspectClusterCleanup(ctx, client, item)
}

func clusterResourceName(name, resourceName string) bool {
	return name == resourceName || strings.HasPrefix(name, resourceName+"-")
}
