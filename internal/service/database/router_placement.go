package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// PrepareFirstRouterPlacement updates a zero-replica Deployment that the
// Operator may have created before Router nodes were selected.
func PrepareFirstRouterPlacement(ctx context.Context, client *k8s.Client, item *model.MySQLCluster) (func() error, error) {
	if client == nil || client.Clientset == nil || len(item.RouterNodes) == 0 {
		return nil, fmt.Errorf("Router 候选节点或 Kubernetes 客户端不可用")
	}
	deployments := client.Clientset.AppsV1().Deployments(item.Namespace)
	name := item.ResourceName + "-router"
	deployment, err := deployments.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 Router Deployment: %w", err)
	}
	if deployment.DeletionTimestamp != nil || deployment.Spec.Template.Labels["mysql.oracle.com/cluster"] != item.ResourceName {
		return nil, fmt.Errorf("Router Deployment 正在删除或不属于当前集群")
	}
	if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 0 || deployment.Status.Replicas != 0 {
		return nil, fmt.Errorf("Router Deployment 已有实例，不能按首次部署更新调度规则")
	}
	nodeAffinity, _, err := databaseNodeAffinity(item.RouterNodes)
	if err != nil {
		return nil, err
	}
	var previous *corev1.Affinity
	if deployment.Spec.Template.Spec.Affinity != nil {
		previous = deployment.Spec.Template.Spec.Affinity.DeepCopy()
	} else {
		deployment.Spec.Template.Spec.Affinity = &corev1.Affinity{}
	}
	deployment.Spec.Template.Spec.Affinity.NodeAffinity = nodeAffinity
	if _, err := deployments.Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
		return nil, fmt.Errorf("更新 Router Deployment 候选节点: %w", err)
	}
	return func() error {
		latest, err := deployments.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if latest.Spec.Replicas == nil || *latest.Spec.Replicas != 0 || latest.Spec.Template.Spec.Affinity == nil || !matchesDatabaseNodePool(latest.Spec.Template.Spec.Affinity.NodeAffinity, item.RouterNodes) {
			return fmt.Errorf("Router Deployment 已变化，无法自动回滚候选节点")
		}
		latest.Spec.Template.Spec.Affinity = previous
		_, err = deployments.Update(ctx, latest, metav1.UpdateOptions{})
		return err
	}, nil
}

// RepairRouterPlacement rolls out Router pods only when an existing pod is
// running outside the saved candidate nodes. The CR must already describe the
// same placement, so the Operator cannot later restore a stale template.
func RepairRouterPlacement(ctx context.Context, client *k8s.Client, item *model.MySQLCluster) error {
	if client == nil || client.Clientset == nil || client.DynamicClient == nil || len(item.RouterNodes) == 0 {
		return fmt.Errorf("Router 候选节点或 Kubernetes 客户端不可用")
	}
	if !nodesWithinPool(item.RouterNodes, item.DatabaseNodes) {
		return fmt.Errorf("Router 候选节点已不在资源池中，请先检查资源池")
	}
	resource, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("读取 MySQL 集群资源: %w", err)
	}
	if resource.GetDeletionTimestamp() != nil || resource.GetLabels()[k8s.ManagedByLabel] != k8s.ManagedByValue {
		return fmt.Errorf("MySQL 集群资源正在删除或不受平台管理")
	}
	raw, found, err := unstructured.NestedMap(resource.Object, "spec", "router", "podSpec", "affinity", "nodeAffinity")
	if err != nil || !found {
		return fmt.Errorf("无法读取 InnoDBCluster 的 Router 调度规则")
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	var affinity corev1.NodeAffinity
	if err := json.Unmarshal(data, &affinity); err != nil || !matchesDatabaseNodePool(&affinity, item.RouterNodes) {
		return fmt.Errorf("Operator 资源的 Router 候选节点与平台记录不一致，请先检查集群配置")
	}
	deployments := client.Clientset.AppsV1().Deployments(item.Namespace)
	name := item.ResourceName + "-router"
	deployment, err := deployments.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("读取 Router Deployment: %w", err)
	}
	if deployment.DeletionTimestamp != nil || deployment.Spec.Template.Labels["mysql.oracle.com/cluster"] != item.ResourceName || deployment.Spec.Replicas == nil || *deployment.Spec.Replicas < 1 {
		return fmt.Errorf("Router Deployment 不属于当前集群或没有运行实例")
	}
	pods, err := client.Clientset.CoreV1().Pods(item.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("读取 Router Pod: %w", err)
	}
	selected := make(map[string]bool, len(item.RouterNodes))
	for _, node := range item.RouterNodes {
		selected[node] = true
	}
	wrongNode := false
	for _, pod := range pods.Items {
		if !strings.HasPrefix(pod.Name, name+"-") || (pod.Labels["mysql.oracle.com/cluster"] != "" && pod.Labels["mysql.oracle.com/cluster"] != item.ResourceName) {
			continue
		}
		wrongNode = wrongNode || (pod.Spec.NodeName != "" && !selected[pod.Spec.NodeName])
	}
	if !wrongNode {
		return fmt.Errorf("Router 已在候选节点内，无需修正调度")
	}
	nodeAffinity, _, err := databaseNodeAffinity(item.RouterNodes)
	if err != nil {
		return err
	}
	if deployment.Spec.Template.Spec.Affinity == nil {
		deployment.Spec.Template.Spec.Affinity = &corev1.Affinity{}
	}
	deployment.Spec.Template.Spec.Affinity.NodeAffinity = nodeAffinity
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = map[string]string{}
	}
	deployment.Spec.Template.Annotations["cylism.io/router-placement-repaired-at"] = time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := deployments.Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("更新 Router Deployment 调度规则: %w", err)
	}
	return nil
}
