package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

var innoDBClusterGVR = schema.GroupVersionResource{Group: "mysql.oracle.com", Version: "v2", Resource: "innodbclusters"}
var mysqlOperatorHelmGVR = schema.GroupVersionResource{Group: "helm.cattle.io", Version: "v1", Resource: "helmcharts"}

const mysqlOperatorHelmName = "cylism-mysql-operator"
const databaseNoScheduleTaintKey = "cylism.io/default-no-schedule"

func OperatorState(ctx context.Context, client *k8s.Client) (string, error) {
	if client == nil || client.Clientset == nil || client.DynamicClient == nil {
		return "unavailable", fmt.Errorf("集群 Kubernetes 客户端不可用")
	}
	chart, chartErr := client.DynamicClient.Resource(mysqlOperatorHelmGVR).Namespace("kube-system").Get(ctx, mysqlOperatorHelmName, metav1.GetOptions{})
	if chartErr != nil && !apierrors.IsNotFound(chartErr) {
		return "unavailable", chartErr
	}
	if chartErr == nil && chart.GetDeletionTimestamp() != nil {
		return "cancelling", nil
	}
	_, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace("").List(ctx, metav1.ListOptions{Limit: 1})
	if apierrors.IsNotFound(err) {
		if chartErr == nil {
			return "installing", nil
		}
		return "missing", nil
	}
	if err != nil {
		return "unavailable", err
	}
	deployment, err := client.Clientset.AppsV1().Deployments("mysql-operator").Get(ctx, "mysql-operator", metav1.GetOptions{})
	deploymentExists := err == nil
	if err == nil && deployment.Status.ReadyReplicas > 0 {
		return "ready", nil
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return "unavailable", err
	}
	deployments, err := client.Clientset.AppsV1().Deployments("mysql-operator").List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=mysql-operator"})
	if err != nil {
		return "unavailable", err
	}
	for _, deployment := range deployments.Items {
		if deployment.Status.ReadyReplicas > 0 {
			return "ready", nil
		}
	}
	if chartErr == nil || deploymentExists || len(deployments.Items) > 0 {
		return "installing", nil
	}
	return "missing", nil
}

func InstallOperator(ctx context.Context, client *k8s.Client) error {
	if client == nil || client.DynamicClient == nil {
		return fmt.Errorf("集群 Kubernetes 客户端不可用")
	}
	state, err := OperatorState(ctx, client)
	if err != nil {
		return err
	}
	if state == "ready" || state == "installing" {
		return nil
	}
	if state == "cancelling" {
		return fmt.Errorf("正在清理上次安装资源，请等待中止完成后再安装")
	}
	resource := client.DynamicClient.Resource(mysqlOperatorHelmGVR).Namespace("kube-system")
	chart := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "helm.cattle.io/v1", "kind": "HelmChart",
		"metadata": map[string]any{"name": mysqlOperatorHelmName, "namespace": "kube-system", "labels": map[string]any{k8s.ManagedByLabel: k8s.ManagedByValue}},
		"spec":     map[string]any{"chart": "mysql-operator", "repo": "https://mysql.github.io/mysql-operator/", "version": "2.3.0", "targetNamespace": "mysql-operator", "createNamespace": true},
	}}
	if _, err := resource.Create(ctx, chart, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("创建 MySQL Operator 安装任务: %w", err)
	}
	return nil
}

// UninstallOperator removes the platform Helm release only when no database
// cluster still depends on the shared controller.
func UninstallOperator(ctx context.Context, client *k8s.Client) error {
	state, err := OperatorState(ctx, client)
	if err != nil {
		return err
	}
	if state != "ready" {
		return fmt.Errorf("MySQL Operator 未就绪，不能卸载")
	}
	resource := client.DynamicClient.Resource(mysqlOperatorHelmGVR).Namespace("kube-system")
	chart, err := resource.Get(ctx, mysqlOperatorHelmName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("未找到平台管理的 MySQL Operator Helm 安装记录，不能卸载")
	}
	if err != nil {
		return fmt.Errorf("读取 MySQL Operator Helm 安装记录：%w", err)
	}
	if chart.GetLabels()[k8s.ManagedByLabel] != k8s.ManagedByValue {
		return fmt.Errorf("MySQL Operator 不受平台管理，不能卸载")
	}
	clusters, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace("").List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("检查 MySQL 集群使用情况：%w", err)
	}
	if len(clusters.Items) > 0 {
		return fmt.Errorf("该 Kubernetes 集群仍有 MySQL 集群资源，请先停止并清理所有数据库集群")
	}
	policy := metav1.DeletePropagationBackground
	if err := resource.Delete(ctx, mysqlOperatorHelmName, metav1.DeleteOptions{PropagationPolicy: &policy}); err != nil {
		return fmt.Errorf("请求 Helm 卸载 MySQL Operator：%w", err)
	}
	return nil
}

// Cancellation uses the Helm controller's normal uninstall path. It never
// manually deletes databases, PVCs, namespaces, or CRDs.
func CancelOperatorInstallation(ctx context.Context, client *k8s.Client) error {
	state, err := OperatorState(ctx, client)
	if err != nil {
		return err
	}
	if state == "ready" {
		return fmt.Errorf("MySQL Operator 已就绪，不能中止安装")
	}
	clusters, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace("").List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("检查 Operator 使用情况：%w", err)
	}
	if clusters != nil && len(clusters.Items) > 0 {
		return fmt.Errorf("该 Kubernetes 集群已有 MySQL 集群使用 Operator，不能中止安装")
	}
	resource := client.DynamicClient.Resource(mysqlOperatorHelmGVR).Namespace("kube-system")
	chart, err := resource.Get(ctx, mysqlOperatorHelmName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if chart.GetLabels()[k8s.ManagedByLabel] != k8s.ManagedByValue {
		return fmt.Errorf("该 Operator 安装资源不受平台管理，不能中止")
	}
	// Keep dependents alive until the Helm finalizer has run its uninstall job.
	background := metav1.DeletePropagationBackground
	if err := resource.Delete(ctx, mysqlOperatorHelmName, metav1.DeleteOptions{PropagationPolicy: &background}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("中止 Helm 安装：%w", err)
	}
	// The uninstall job belongs to Helm's cleanup and must be left running.
	jobs := client.Clientset.BatchV1().Jobs("kube-system")
	job, err := jobs.Get(ctx, "helm-install-"+mysqlOperatorHelmName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取 Helm 安装任务：%w", err)
	}
	owned := false
	for _, owner := range job.OwnerReferences {
		if owner.UID == chart.GetUID() && owner.Kind == "HelmChart" && owner.Name == chart.GetName() {
			owned = true
		}
	}
	if !owned && job.Labels["helmcharts.helm.cattle.io/chart"] != mysqlOperatorHelmName {
		return nil
	}
	foreground := metav1.DeletePropagationForeground
	if err := jobs.Delete(ctx, job.Name, metav1.DeleteOptions{PropagationPolicy: &foreground}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("停止 Helm 安装任务：%w", err)
	}
	return nil
}

func ClusterResourceName(id uint) string { return fmt.Sprintf("cylism-mysql-cluster-%d", id) }

func ClusterServiceHost(item *model.MySQLCluster) string {
	return item.ResourceName + "." + item.Namespace + ".svc.cluster.local"
}

func CheckOperator(ctx context.Context, client *k8s.Client, namespace string) error {
	state, err := OperatorState(ctx, client)
	if err != nil {
		return fmt.Errorf("检查 MySQL Operator: %w", err)
	}
	if state != "ready" {
		return fmt.Errorf("MySQL Operator 尚未就绪（当前状态：%s）", state)
	}
	return nil
}

func ValidateClusterNodes(ctx context.Context, client *k8s.Client, databaseNodes, routerNodes []string) error {
	if len(databaseNodes) < 1 || len(databaseNodes) > 9 || len(routerNodes) > 9 {
		return fmt.Errorf("数据库候选节点须为 1–9 台，Router 候选节点最多 9 台")
	}
	if err := validateNodeGroup(ctx, client, databaseNodes, true); err != nil {
		return err
	}
	return validateNodeGroup(ctx, client, routerNodes, false)
}

func ValidateRouterNodes(ctx context.Context, client *k8s.Client, routerNodes []string) error {
	if len(routerNodes) < 1 || len(routerNodes) > 9 {
		return fmt.Errorf("Router 候选节点须为 1–9 台")
	}
	return validateNodeGroup(ctx, client, routerNodes, false)
}

func validateNodeGroup(ctx context.Context, client *k8s.Client, names []string, database bool) error {
	seen := map[string]bool{}
	for _, name := range names {
		if name == "" || seen[name] {
			return fmt.Errorf("节点名称不能为空，且同一组中不能重复")
		}
		seen[name] = true
		node, err := client.Clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("读取节点 %s: %w", name, err)
		}
		if node.Spec.Unschedulable {
			return fmt.Errorf("节点 %s 已暂停调度", name)
		}
		ready := false
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				ready = true
			}
			if condition.Type == corev1.NodeDiskPressure && condition.Status == corev1.ConditionTrue {
				return fmt.Errorf("节点 %s 磁盘压力过高", name)
			}
		}
		if !ready {
			return fmt.Errorf("节点 %s 未就绪", name)
		}
		for _, taint := range node.Spec.Taints {
			if database && tolerableDatabaseTaint(taint) {
				continue
			}
			if taint.Effect == corev1.TaintEffectNoSchedule || taint.Effect == corev1.TaintEffectNoExecute {
				return fmt.Errorf("节点 %s 有禁止调度污点 %s", name, taint.Key)
			}
		}
	}
	return nil
}

func DatabaseInstanceCount(item *model.MySQLCluster) int {
	if item.DatabaseInstances > 0 {
		return item.DatabaseInstances
	}
	return len(item.DatabaseNodes)
}

func RouterInstanceCount(item *model.MySQLCluster) int {
	if item.RouterInstances != nil {
		return *item.RouterInstances
	}
	return len(item.RouterNodes)
}

func ValidateDatabaseInstances(instances int, nodes []string) error {
	if instances < 1 || instances > 9 {
		return fmt.Errorf("数据库实例数须为 1–9 个")
	}
	return nil
}

func placement(nodes []string) map[string]any {
	terms := make([]any, 0, len(nodes))
	for _, name := range nodes {
		terms = append(terms, map[string]any{"matchFields": []any{map[string]any{"key": "metadata.name", "operator": "In", "values": []any{name}}}})
	}
	return map[string]any{"affinity": map[string]any{
		"nodeAffinity": map[string]any{"requiredDuringSchedulingIgnoredDuringExecution": map[string]any{
			"nodeSelectorTerms": terms,
		}},
	}}
}

// ClusterObject exposes only the supported InnoDBCluster fields; the Operator
// owns StatefulSets, Services, failover, and Router deployments.
func ClusterObject(item *model.MySQLCluster) *unstructured.Unstructured {
	labels := map[string]any{k8s.ManagedByLabel: k8s.ManagedByValue}
	claim := map[string]any{"accessModes": []any{"ReadWriteOnce"}, "resources": map[string]any{"requests": map[string]any{"storage": item.StorageSize}}}
	if item.StorageClass != "" {
		claim["storageClassName"] = item.StorageClass
	}
	dbSpec := placement(item.DatabaseNodes)
	dbSpec["tolerations"] = []any{map[string]any{"key": databaseNoScheduleTaintKey, "operator": "Equal", "value": "true", "effect": "NoSchedule"}}
	routerSpec := placement(item.RouterNodes)
	setAntiAffinity(dbSpec, item.ResourceName, "mysqld", true)
	if len(item.RouterNodes) > 0 {
		setAntiAffinity(routerSpec, item.ResourceName, "mysqlrouter")
	}
	volumeClaim := map[string]any{"accessModes": claim["accessModes"], "resources": claim["resources"]}
	if item.StorageClass != "" {
		volumeClaim["storageClassName"] = item.StorageClass
	}
	router := map[string]any{"instances": int64(RouterInstanceCount(item))}
	if len(item.RouterNodes) > 0 {
		router["podSpec"] = routerSpec
	}
	object := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "mysql.oracle.com/v2", "kind": "InnoDBCluster",
		"metadata": map[string]any{"name": item.ResourceName, "namespace": item.Namespace, "labels": labels},
		"spec": map[string]any{
			"secretName": item.ResourceName + "-root", "instances": int64(DatabaseInstanceCount(item)),
			"tlsUseSelfSigned":           true,
			"datadirVolumeClaimTemplate": volumeClaim,
			"podSpec":                    dbSpec,
			"router":                     router,
		},
	}}
	if item.MyCNF != "" {
		object.Object["spec"].(map[string]any)["mycnf"] = item.MyCNF
	}
	return object
}

func setAntiAffinity(spec map[string]any, name, component string, allowColocation ...bool) {
	affinity := spec["affinity"].(map[string]any)
	term := map[string]any{
		"labelSelector": map[string]any{"matchLabels": map[string]any{"mysql.oracle.com/cluster": name, "component": component}},
		"topologyKey":   "kubernetes.io/hostname",
	}
	if len(allowColocation) > 0 && allowColocation[0] {
		affinity["podAntiAffinity"] = map[string]any{"preferredDuringSchedulingIgnoredDuringExecution": []any{map[string]any{"weight": int64(100), "podAffinityTerm": term}}}
		return
	}
	affinity["podAntiAffinity"] = map[string]any{"requiredDuringSchedulingIgnoredDuringExecution": []any{term}}
}

func ApplyCluster(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, rootPassword string) error {
	if err := CheckOperator(ctx, client, item.Namespace); err != nil {
		return err
	}
	if err := ValidateClusterNodes(ctx, client, item.DatabaseNodes, item.RouterNodes); err != nil {
		return err
	}
	if err := ValidateDatabaseInstances(DatabaseInstanceCount(item), item.DatabaseNodes); err != nil {
		return err
	}
	namespaces := client.Clientset.CoreV1().Namespaces()
	if _, err := namespaces.Get(ctx, item.Namespace, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		if _, err = namespaces.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: item.Namespace}}, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	} else if err != nil {
		return err
	}
	secretName := item.ResourceName + "-root"
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: item.Namespace, Labels: map[string]string{k8s.ManagedByLabel: k8s.ManagedByValue}}, Type: corev1.SecretTypeOpaque, StringData: map[string]string{"rootUser": "root", "rootHost": "%", "rootPassword": rootPassword}}
	secrets := client.Clientset.CoreV1().Secrets(item.Namespace)
	if current, err := secrets.Get(ctx, secretName, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		if _, err = secrets.Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("创建 MySQL 根密钥: %w", err)
		}
	} else if err != nil {
		return err
	} else if current.Labels[k8s.ManagedByLabel] != k8s.ManagedByValue {
		return fmt.Errorf("同名 MySQL 根密钥不受平台管理")
	} else {
		secret.ResourceVersion = current.ResourceVersion
		if _, err = secrets.Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	resource := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace)
	desired := ClusterObject(item)
	current, err := resource.Get(ctx, item.ResourceName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = resource.Create(ctx, desired, metav1.CreateOptions{})
	} else if err == nil {
		if current.GetLabels()[k8s.ManagedByLabel] != k8s.ManagedByValue {
			return fmt.Errorf("同名 MySQL 集群不受平台管理")
		}
		current.Object["spec"] = desired.Object["spec"]
		_, err = resource.Update(ctx, current, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("提交 MySQL 集群: %w", err)
	}
	return nil
}

func ClusterStatus(ctx context.Context, client *k8s.Client, item *model.MySQLCluster) (string, int, int, error) {
	if err := CheckOperator(ctx, client, item.Namespace); err != nil {
		return "error", 0, 0, err
	}
	_, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "missing", 0, 0, nil
	}
	if err != nil {
		return "error", 0, 0, err
	}
	stateful, err := client.Clientset.AppsV1().StatefulSets(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "starting", 0, 0, nil
	}
	if err != nil {
		return "error", 0, 0, err
	}
	online := int(stateful.Status.ReadyReplicas)
	routerReady := 0
	deployments, err := client.Clientset.AppsV1().Deployments(item.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "error", online, 0, err
	}
	for _, deployment := range deployments.Items {
		if (deployment.Labels["mysql.oracle.com/cluster"] == item.ResourceName || strings.HasPrefix(deployment.Name, item.ResourceName+"-")) && strings.Contains(deployment.Name, "router") {
			routerReady += int(deployment.Status.ReadyReplicas)
		}
	}
	if online >= DatabaseInstanceCount(item) && routerReady >= RouterInstanceCount(item) {
		return "ready", online, routerReady, nil
	}
	return "starting", online, routerReady, nil
}

// ScaleCluster changes only the replica count. The caller updates both placement
// templates before scaling database instances because the Operator does not
// reconcile later podSpec changes into the StatefulSet.
func ScaleCluster(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, component string, currentCount int) error {
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
	var liveCount int64
	var found bool
	if component == "router" {
		liveCount, found, err = unstructured.NestedInt64(current.Object, "spec", "router", "instances")
	} else {
		liveCount, found, err = unstructured.NestedInt64(current.Object, "spec", "instances")
	}
	if err != nil || !found || liveCount != int64(currentCount) {
		return fmt.Errorf("集群实例数已变化，请刷新后重试")
	}
	if component == "database" && currentCount >= len(item.DatabaseNodes) {
		strict, _, err := unstructured.NestedSlice(current.Object, "spec", "podSpec", "affinity", "podAntiAffinity", "requiredDuringSchedulingIgnoredDuringExecution")
		if err != nil {
			return fmt.Errorf("读取数据库调度规则: %w", err)
		}
		if len(strict) > 0 {
			return fmt.Errorf("现有集群启用了严格跨节点调度，已达到 %d 台候选节点上限；需先调整现有 StatefulSet 调度规则", len(item.DatabaseNodes))
		}
	}
	operations := []map[string]any{{"op": "test", "path": path, "value": currentCount}}
	if component == "router" && currentCount == 0 {
		if len(item.RouterNodes) == 0 {
			return fmt.Errorf("请先选择 Router 候选节点")
		}
		routerSpec := placement(item.RouterNodes)
		setAntiAffinity(routerSpec, item.ResourceName, "mysqlrouter")
		operation := "add"
		if _, found, _ := unstructured.NestedMap(current.Object, "spec", "router", "podSpec"); found {
			operation = "replace"
		}
		operations = append(operations, map[string]any{"op": operation, "path": "/spec/router/podSpec", "value": routerSpec})
	}
	operations = append(operations, map[string]any{"op": "replace", "path": path, "value": currentCount + 1})
	patch, err := json.Marshal(operations)
	if err != nil {
		return err
	}
	if _, err := resource.Patch(ctx, item.ResourceName, types.JSONPatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("追加 MySQL 实例: %w", err)
	}
	return nil
}

func ScaleRouterDown(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, currentCount int) error {
	if currentCount < 1 {
		return fmt.Errorf("当前没有可清退的 Router 实例")
	}
	resource := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace)
	current, err := resource.Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("读取 MySQL 集群资源: %w", err)
	}
	if current.GetDeletionTimestamp() != nil || current.GetLabels()[k8s.ManagedByLabel] != k8s.ManagedByValue {
		return fmt.Errorf("MySQL 集群资源正在删除或不受平台管理")
	}
	liveCount, found, err := unstructured.NestedInt64(current.Object, "spec", "router", "instances")
	if err != nil || !found || liveCount != int64(currentCount) {
		return fmt.Errorf("Router 实例数已变化，请刷新后重试")
	}
	deployment, err := client.Clientset.AppsV1().Deployments(item.Namespace).Get(ctx, item.ResourceName+"-router", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("读取 Router Deployment: %w", err)
	}
	if deployment.DeletionTimestamp != nil || deployment.Spec.Replicas == nil || int(*deployment.Spec.Replicas) != currentCount || deployment.Spec.Template.Labels["mysql.oracle.com/cluster"] != item.ResourceName {
		return fmt.Errorf("Router Deployment 实例数与平台记录不一致，请刷新后重试")
	}
	patch, err := json.Marshal([]map[string]any{
		{"op": "test", "path": "/spec/router/instances", "value": currentCount},
		{"op": "replace", "path": "/spec/router/instances", "value": currentCount - 1},
	})
	if err != nil {
		return err
	}
	if _, err := resource.Patch(ctx, item.ResourceName, types.JSONPatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("清退 Router 实例: %w", err)
	}
	return nil
}

func RouterScaleDownComplete(ctx context.Context, client *k8s.Client, item *model.MySQLCluster) (bool, error) {
	target := RouterInstanceCount(item)
	resource, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	liveCount, found, err := unstructured.NestedInt64(resource.Object, "spec", "router", "instances")
	if err != nil || !found || liveCount != int64(target) {
		return false, nil
	}
	deployment, err := client.Clientset.AppsV1().Deployments(item.Namespace).Get(ctx, item.ResourceName+"-router", metav1.GetOptions{})
	if apierrors.IsNotFound(err) && target == 0 {
		deployment = nil
	} else if err != nil {
		return false, err
	}
	if deployment != nil && (deployment.Spec.Replicas == nil || int(*deployment.Spec.Replicas) != target || int(deployment.Status.Replicas) != target || int(deployment.Status.ReadyReplicas) != target) {
		return false, nil
	}
	pods, err := client.Clientset.CoreV1().Pods(item.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	count := 0
	for _, pod := range pods.Items {
		if strings.HasPrefix(pod.Name, item.ResourceName+"-router-") && (pod.Labels["mysql.oracle.com/cluster"] == "" || pod.Labels["mysql.oracle.com/cluster"] == item.ResourceName) {
			count++
		}
	}
	return count == target, nil
}
