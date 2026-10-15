package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func databaseNodeAffinity(nodes []string) (*corev1.NodeAffinity, map[string]any, error) {
	affinity := placement(nodes)["affinity"].(map[string]any)["nodeAffinity"].(map[string]any)
	data, err := json.Marshal(affinity)
	if err != nil {
		return nil, nil, err
	}
	var typed corev1.NodeAffinity
	if err := json.Unmarshal(data, &typed); err != nil {
		return nil, nil, err
	}
	return &typed, affinity, nil
}

func matchesDatabaseNodePool(affinity *corev1.NodeAffinity, nodes []string) bool {
	if affinity == nil || affinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return false
	}
	terms := affinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(terms) != len(nodes) {
		return false
	}
	remaining := make(map[string]bool, len(nodes))
	for _, name := range nodes {
		remaining[name] = true
	}
	for _, term := range terms {
		if len(term.MatchExpressions) != 0 || len(term.MatchFields) != 1 {
			return false
		}
		field := term.MatchFields[0]
		if field.Key != "metadata.name" || field.Operator != corev1.NodeSelectorOpIn || len(field.Values) != 1 || !remaining[field.Values[0]] {
			return false
		}
		delete(remaining, field.Values[0])
	}
	return len(remaining) == 0
}

func databaseAffinityNodes(affinity *corev1.NodeAffinity) ([]string, error) {
	if affinity == nil || affinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return nil, fmt.Errorf("数据库节点调度规则缺失")
	}
	terms := affinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(terms) == 0 {
		return nil, fmt.Errorf("数据库节点调度规则为空")
	}
	nodes := make([]string, 0, len(terms))
	for _, term := range terms {
		if len(term.MatchExpressions) != 0 || len(term.MatchFields) != 1 {
			return nil, fmt.Errorf("数据库节点调度规则包含未知条件")
		}
		field := term.MatchFields[0]
		if field.Key != "metadata.name" || field.Operator != corev1.NodeSelectorOpIn || len(field.Values) != 1 {
			return nil, fmt.Errorf("数据库节点调度规则包含未知节点条件")
		}
		nodes = append(nodes, field.Values[0])
	}
	if !matchesDatabaseNodePool(affinity, nodes) {
		return nil, fmt.Errorf("数据库节点调度规则包含重复节点")
	}
	return nodes, nil
}

func nodesWithinPool(nodes, pool []string) bool {
	allowed := make(map[string]bool, len(pool))
	for _, name := range pool {
		allowed[name] = true
	}
	for _, name := range nodes {
		if !allowed[name] {
			return false
		}
	}
	return true
}

func checkRemovedDatabaseNodes(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, nodes []string) error {
	selected := make(map[string]bool, len(nodes))
	for _, name := range nodes {
		selected[name] = true
	}
	removed := map[string]bool{}
	for _, name := range item.DatabaseNodes {
		if !selected[name] {
			removed[name] = true
		}
	}
	if len(removed) == 0 {
		return nil
	}
	pods, err := client.Clientset.CoreV1().Pods(item.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("检查 MySQL 实例节点: %w", err)
	}
	for _, pod := range pods.Items {
		if pod.Labels["mysql.oracle.com/cluster"] != item.ResourceName && !strings.HasPrefix(pod.Name, item.ResourceName+"-") {
			continue
		}
		if pod.Labels["component"] == "mysqlrouter" || strings.Contains(pod.Name, "router") {
			continue
		}
		if removed[pod.Spec.NodeName] {
			return fmt.Errorf("节点 %s 仍承载 MySQL 实例 %s，请先迁移实例", pod.Spec.NodeName, pod.Name)
		}
	}
	claims, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("检查 MySQL 数据卷: %w", err)
	}
	for _, claim := range claims.Items {
		if claim.Labels["mysql.oracle.com/cluster"] != item.ResourceName && !strings.Contains(claim.Name, item.ResourceName+"-") {
			continue
		}
		if claim.Spec.VolumeName == "" {
			continue
		}
		volume, err := client.Clientset.CoreV1().PersistentVolumes().Get(ctx, claim.Spec.VolumeName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("读取数据卷 %s 的节点约束: %w", claim.Name, err)
		}
		if volume.Spec.NodeAffinity == nil || volume.Spec.NodeAffinity.Required == nil {
			continue
		}
		for _, term := range volume.Spec.NodeAffinity.Required.NodeSelectorTerms {
			for _, expression := range term.MatchExpressions {
				if expression.Key == corev1.LabelHostname && expression.Operator == corev1.NodeSelectorOpIn {
					for _, name := range expression.Values {
						if removed[name] {
							return fmt.Errorf("数据卷 %s 仍绑定节点 %s，请先迁移或清理数据卷", claim.Name, name)
						}
					}
				}
			}
		}
	}
	return nil
}

// UpdateDatabaseResourcePool updates both scheduling templates. Existing Pods are never restarted.
// The returned function restores the old templates if persisting the platform record fails.
func UpdateDatabaseResourcePool(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, nodes []string) (func() error, error) {
	if client == nil || client.Clientset == nil || client.DynamicClient == nil {
		return nil, fmt.Errorf("集群 Kubernetes 客户端不可用")
	}
	if err := checkRemovedDatabaseNodes(ctx, client, item, nodes); err != nil {
		return nil, err
	}
	return updateDatabasePlacement(ctx, client, item, nodes, true)
}

// UpdateDatabaseCandidates changes the template used for the next MySQL Pod.
func UpdateDatabaseCandidates(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, nodes []string) (func() error, error) {
	if len(nodes) == 0 || !nodesWithinPool(nodes, item.DatabaseNodes) {
		return nil, fmt.Errorf("MySQL 候选节点必须在资源池内")
	}
	return updateDatabasePlacement(ctx, client, item, nodes, false)
}

func updateDatabasePlacement(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, nodes []string, poolChange bool) (func() error, error) {
	if client == nil || client.Clientset == nil || client.DynamicClient == nil {
		return nil, fmt.Errorf("集群 Kubernetes 客户端不可用")
	}
	resource := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace)
	cluster, err := resource.Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("读取 InnoDBCluster 调度规则: %w", err)
	}
	if cluster.GetDeletionTimestamp() != nil || cluster.GetLabels()[k8s.ManagedByLabel] != k8s.ManagedByValue {
		return nil, fmt.Errorf("MySQL 集群资源正在删除或不受平台管理")
	}
	current, found, err := unstructured.NestedMap(cluster.Object, "spec", "podSpec", "affinity", "nodeAffinity")
	if err != nil || !found {
		return nil, fmt.Errorf("无法读取 InnoDBCluster 的数据库节点调度规则: %v", err)
	}
	data, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}
	var currentAffinity corev1.NodeAffinity
	if err := json.Unmarshal(data, &currentAffinity); err != nil {
		return nil, fmt.Errorf("解析 InnoDBCluster 调度规则: %w", err)
	}
	currentNodes, err := databaseAffinityNodes(&currentAffinity)
	if err != nil || !nodesWithinPool(currentNodes, item.DatabaseNodes) {
		return nil, fmt.Errorf("InnoDBCluster 的节点调度规则与平台资源池不一致，请先检查集群配置")
	}
	sets := client.Clientset.AppsV1().StatefulSets(item.Namespace)
	set, err := sets.Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("读取 MySQL StatefulSet 调度规则: %w", err)
	}
	if set.DeletionTimestamp != nil || set.Spec.Template.Spec.Affinity == nil || !matchesDatabaseNodePool(set.Spec.Template.Spec.Affinity.NodeAffinity, currentNodes) {
		return nil, fmt.Errorf("StatefulSet 的节点调度规则与平台资源池不一致，请先检查集群配置")
	}
	if !poolChange && (set.Spec.Replicas == nil || int(*set.Spec.Replicas) != DatabaseInstanceCount(item)) {
		return nil, fmt.Errorf("StatefulSet 实例数与平台记录不一致，请刷新集群状态后重试")
	}
	if poolChange && !matchesDatabaseNodePool(&currentAffinity, item.DatabaseNodes) {
		retained := make([]string, 0, len(currentNodes))
		for _, name := range currentNodes {
			if nodesWithinPool([]string{name}, nodes) {
				retained = append(retained, name)
			}
		}
		if len(retained) == 0 {
			return nil, fmt.Errorf("当前 MySQL 候选节点均不在新资源池中，请先调整候选节点")
		}
		nodes = retained
	}
	typed, desired, err := databaseNodeAffinity(nodes)
	if err != nil {
		return nil, err
	}
	oldSetAffinity := set.Spec.Template.Spec.Affinity.NodeAffinity.DeepCopy()
	oldStrategy := *set.Spec.UpdateStrategy.DeepCopy()
	if set.Spec.UpdateStrategy.Type == "" || set.Spec.UpdateStrategy.Type == appsv1.RollingUpdateStatefulSetStrategyType {
		if set.Spec.Replicas == nil || *set.Spec.Replicas < 1 {
			return nil, fmt.Errorf("无法确认现有 MySQL 实例数，拒绝修改滚动更新分区")
		}
		partition := *set.Spec.Replicas
		if set.Spec.UpdateStrategy.RollingUpdate != nil && set.Spec.UpdateStrategy.RollingUpdate.Partition != nil && *set.Spec.UpdateStrategy.RollingUpdate.Partition > partition {
			return nil, fmt.Errorf("StatefulSet 已设置更高的滚动更新分区，请先检查调度策略")
		}
		set.Spec.UpdateStrategy.Type = appsv1.RollingUpdateStatefulSetStrategyType
		if set.Spec.UpdateStrategy.RollingUpdate == nil {
			set.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateStatefulSetStrategy{}
		}
		set.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
	} else if set.Spec.UpdateStrategy.Type != appsv1.OnDeleteStatefulSetStrategyType {
		return nil, fmt.Errorf("不支持当前 StatefulSet 更新策略 %s", set.Spec.UpdateStrategy.Type)
	}
	set.Spec.Template.Spec.Affinity.NodeAffinity = typed
	if _, err := sets.Update(ctx, set, metav1.UpdateOptions{}); err != nil {
		return nil, fmt.Errorf("更新 MySQL StatefulSet 调度规则: %w", err)
	}
	rollbackSet := func() error {
		latest, err := sets.Get(ctx, item.ResourceName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if latest.Spec.Template.Spec.Affinity == nil || !matchesDatabaseNodePool(latest.Spec.Template.Spec.Affinity.NodeAffinity, nodes) {
			return fmt.Errorf("StatefulSet 调度规则已再次变化，无法自动回滚")
		}
		latest.Spec.Template.Spec.Affinity.NodeAffinity = oldSetAffinity
		latest.Spec.UpdateStrategy = oldStrategy
		_, err = sets.Update(ctx, latest, metav1.UpdateOptions{})
		return err
	}
	updated := cluster.DeepCopy()
	if err := unstructured.SetNestedMap(updated.Object, desired, "spec", "podSpec", "affinity", "nodeAffinity"); err != nil {
		return nil, fmt.Errorf("准备 InnoDBCluster 调度规则: %w；StatefulSet 回滚: %v", err, rollbackSet())
	}
	if _, err := resource.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return nil, fmt.Errorf("更新 InnoDBCluster 调度规则: %w；StatefulSet 回滚: %v", err, rollbackSet())
	}
	rollback := func() error {
		latest, err := resource.Get(ctx, item.ResourceName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if err := unstructured.SetNestedMap(latest.Object, current, "spec", "podSpec", "affinity", "nodeAffinity"); err != nil {
			return err
		}
		if _, err := resource.Update(ctx, latest, metav1.UpdateOptions{}); err != nil {
			return err
		}
		return rollbackSet()
	}
	return rollback, nil
}
