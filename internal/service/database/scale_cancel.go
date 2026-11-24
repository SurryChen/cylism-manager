package database

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

// CancelScale restores only the previous replica target. The cluster resource,
// existing workloads, and PVCs remain under Operator ownership.
func CancelScale(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, component string, target int) error {
	if target < 1 {
		return fmt.Errorf("无效的扩容目标")
	}
	path := "/spec/instances"
	if component == "router" {
		path = "/spec/router/instances"
	} else if component != "database" {
		return fmt.Errorf("未知的 MySQL 组件")
	}
	resource := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace)
	current, err := resource.Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("读取 MySQL 集群资源: %w", err)
	}
	if current.GetDeletionTimestamp() != nil || current.GetLabels()[k8s.ManagedByLabel] != k8s.ManagedByValue {
		return fmt.Errorf("MySQL 集群资源正在删除或不受平台管理")
	}
	var live int64
	if component == "database" {
		live, _, err = unstructured.NestedInt64(current.Object, "spec", "instances")
	} else {
		live, _, err = unstructured.NestedInt64(current.Object, "spec", "router", "instances")
	}
	if err != nil || live != int64(target) {
		return fmt.Errorf("Operator 目标实例数已变化，请刷新后重试")
	}
	previous := target - 1
	if component == "database" {
		if previous < 1 {
			return fmt.Errorf("至少保留一个 MySQL 实例")
		}
		set, err := client.Clientset.AppsV1().StatefulSets(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("检查 MySQL 工作负载: %w", err)
		}
		if int(set.Status.ReadyReplicas) > previous {
			return fmt.Errorf("新增 MySQL 实例已就绪，不能作为未完成任务撤回")
		}
	} else {
		deployments, err := client.Clientset.AppsV1().Deployments(item.Namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("检查 Router 工作负载: %w", err)
		}
		for _, deployment := range deployments.Items {
			if (deployment.Name == item.ResourceName+"-router" || deployment.Labels["mysql.oracle.com/cluster"] == item.ResourceName) && int(deployment.Status.ReadyReplicas) > previous {
				return fmt.Errorf("新增 Router 已就绪，不能作为未完成任务撤回")
			}
		}
	}
	operations := []map[string]any{
		{"op": "test", "path": path, "value": target},
		{"op": "replace", "path": path, "value": previous},
	}
	patch, err := json.Marshal(operations)
	if err != nil {
		return err
	}
	if _, err := resource.Patch(ctx, item.ResourceName, types.JSONPatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("撤回本次扩容: %w", err)
	}
	return nil
}
