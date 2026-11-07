package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

// A stopped cluster keeps its original CR, credentials and data. Oracle does
// not support instances=0: suspending its controller is safe only if there are
// no other InnoDBClusters (including clusters outside the platform).
func exclusiveCluster(ctx context.Context, client *k8s.Client, item *model.MySQLCluster) error {
	list, err := client.DynamicClient.Resource(innoDBClusterGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, cluster := range list.Items {
		if cluster.GetNamespace() != item.Namespace || cluster.GetName() != item.ResourceName {
			return fmt.Errorf("共享 Operator 还管理着集群 %s/%s，不能暂停或恢复共享控制器", cluster.GetNamespace(), cluster.GetName())
		}
	}
	return nil
}

func operatorDeployment(ctx context.Context, client *k8s.Client) (*appsv1.Deployment, error) {
	return client.Clientset.AppsV1().Deployments("mysql-operator").Get(ctx, "mysql-operator", metav1.GetOptions{})
}
func operatorSuspended(ctx context.Context, client *k8s.Client) (bool, error) {
	deployment, err := operatorDeployment(ctx, client)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return deployment.Spec.Replicas != nil && *deployment.Spec.Replicas == 0, nil
}
func setOperatorReplicas(ctx context.Context, client *k8s.Client, count int32) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		deployment, err := operatorDeployment(ctx, client)
		if err != nil {
			return err
		}
		if deployment.DeletionTimestamp != nil {
			return fmt.Errorf("Operator 正在删除")
		}
		deployment.Spec.Replicas = &count
		_, err = client.Clientset.AppsV1().Deployments(deployment.Namespace).Update(ctx, deployment, metav1.UpdateOptions{})
		return err
	})
}

func operatorQuiesced(ctx context.Context, client *k8s.Client) (bool, error) {
	deployment, err := operatorDeployment(ctx, client)
	if err != nil {
		return false, err
	}
	pods, err := client.Clientset.CoreV1().Pods(deployment.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	for _, pod := range pods.Items {
		if strings.HasPrefix(pod.Name, deployment.Name+"-") && !containersExited(pod) {
			return false, nil
		}
	}
	return true, nil
}

// ClusterStopped uses desired replica counts and live Pods, never stale ready
// counters on the StatefulSet or InnoDBCluster status.
func ClusterStopped(ctx context.Context, client *k8s.Client, item *model.MySQLCluster) (bool, error) {
	suspended, err := operatorSuspended(ctx, client)
	if err != nil || !suspended {
		return false, err
	}
	quiesced, err := operatorQuiesced(ctx, client)
	if err != nil || !quiesced {
		return false, err
	}
	state, err := InspectClusterCleanup(ctx, client, item)
	if err != nil || !state.ClusterResource || len(state.Pods) != 0 {
		return false, err
	}
	set, err := client.Clientset.AppsV1().StatefulSets(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if set.Spec.Replicas == nil || *set.Spec.Replicas != 0 {
		return false, nil
	}
	deployments, err := client.Clientset.AppsV1().Deployments(item.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	for _, deployment := range deployments.Items {
		if clusterResourceName(deployment.Name, item.ResourceName) && (deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 0) {
			return false, nil
		}
	}
	return true, nil
}

func lifecycleResource(ctx context.Context, client *k8s.Client, item *model.MySQLCluster) error {
	cluster, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if cluster.GetDeletionTimestamp() != nil || cluster.GetLabels()[k8s.ManagedByLabel] != k8s.ManagedByValue {
		return fmt.Errorf("集群正在删除或不受平台管理")
	}
	return nil
}

func StopClusterStep(ctx context.Context, client *k8s.Client, item *model.MySQLCluster) (bool, string, error) {
	if err := lifecycleResource(ctx, client, item); err != nil {
		return false, "", err
	}
	if err := exclusiveCluster(ctx, client, item); err != nil {
		return false, "", err
	}
	if err := setOperatorReplicas(ctx, client, 0); err != nil {
		return false, "", err
	}
	quiesced, err := operatorQuiesced(ctx, client)
	if err != nil {
		return false, "", err
	}
	if !quiesced {
		return false, "等待 MySQL Operator 退出", nil
	}
	if err := exclusiveCluster(ctx, client, item); err != nil {
		if restoreErr := setOperatorReplicas(ctx, client, 1); restoreErr != nil {
			return false, "", fmt.Errorf("%v；恢复共享 Operator 失败：%w", err, restoreErr)
		}
		return false, "", err
	}
	if err := retainClusterData(ctx, client, item); err != nil {
		return false, "", err
	}
	sets, err := client.Clientset.AppsV1().StatefulSets(item.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, "", err
	}
	zero := int32(0)
	for _, set := range sets.Items {
		if !clusterResourceName(set.Name, item.ResourceName) {
			continue
		}
		set.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{WhenDeleted: appsv1.RetainPersistentVolumeClaimRetentionPolicyType, WhenScaled: appsv1.RetainPersistentVolumeClaimRetentionPolicyType}
		set.Spec.Replicas = &zero
		if _, err := client.Clientset.AppsV1().StatefulSets(item.Namespace).Update(ctx, &set, metav1.UpdateOptions{}); err != nil {
			return false, "", err
		}
	}
	deployments, err := client.Clientset.AppsV1().Deployments(item.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, "", err
	}
	for _, deployment := range deployments.Items {
		if !clusterResourceName(deployment.Name, item.ResourceName) {
			continue
		}
		deployment.Spec.Replicas = &zero
		if _, err := client.Clientset.AppsV1().Deployments(item.Namespace).Update(ctx, &deployment, metav1.UpdateOptions{}); err != nil {
			return false, "", err
		}
	}
	pods, err := client.Clientset.CoreV1().Pods(item.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, "", err
	}
	for _, pod := range pods.Items {
		if !clusterResourceName(pod.Name, item.ResourceName) {
			continue
		}
		// The paused Operator cannot finish its membership finalizers. Release only
		// its known finalizers, only after every container has terminated normally.
		if pod.DeletionTimestamp != nil && containersExited(pod) {
			finalizers := []string{}
			for _, value := range pod.Finalizers {
				if value != "mysql.oracle.com/membership" && value != "kopf.zalando.org/KopfFinalizerMarker" {
					finalizers = append(finalizers, value)
				}
			}
			if len(finalizers) != len(pod.Finalizers) {
				pod.Finalizers = finalizers
				if _, err := client.Clientset.CoreV1().Pods(item.Namespace).Update(ctx, &pod, metav1.UpdateOptions{}); err != nil {
					return false, "", err
				}
			}
		}
	}
	stopped, err := ClusterStopped(ctx, client, item)
	if stopped {
		return true, "集群已停用，原集群资源、凭据和数据卷均保留", err
	}
	return false, "等待 MySQL 和 Router 正常退出，数据卷保留", err
}

func containersExited(pod corev1.Pod) bool {
	if len(pod.Status.ContainerStatuses) != len(pod.Spec.Containers) || len(pod.Spec.Containers) == 0 || len(pod.Status.InitContainerStatuses) != len(pod.Spec.InitContainers) {
		return false
	}
	for _, status := range append(append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...), pod.Status.ContainerStatuses...) {
		if status.State.Terminated == nil || status.State.Terminated.ExitCode != 0 {
			return false
		}
	}
	return true
}

func ResumeCluster(ctx context.Context, client *k8s.Client, item *model.MySQLCluster) error {
	if err := lifecycleResource(ctx, client, item); err != nil {
		return err
	}
	suspended, err := operatorSuspended(ctx, client)
	if err != nil {
		return err
	}
	if suspended {
		if err := exclusiveCluster(ctx, client, item); err != nil {
			return err
		}
	}
	// Restore the original desired counts, without regenerating credentials,
	// creating a new CR or rewriting database topology.
	set, err := client.Clientset.AppsV1().StatefulSets(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	replicas := int32(DatabaseInstanceCount(item))
	set.Spec.Replicas = &replicas
	if _, err := client.Clientset.AppsV1().StatefulSets(item.Namespace).Update(ctx, set, metav1.UpdateOptions{}); err != nil {
		return err
	}
	routers, err := client.Clientset.AppsV1().Deployments(item.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, router := range routers.Items {
		if router.Name != item.ResourceName+"-router" {
			continue
		}
		count := int32(RouterInstanceCount(item))
		router.Spec.Replicas = &count
		if _, err := client.Clientset.AppsV1().Deployments(item.Namespace).Update(ctx, &router, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return setOperatorReplicas(ctx, client, 1)
}

// RetireClusterStep lets the Operator remove its CR and membership safely.
// Suspended clusters are finalized offline only after all containers exited.
// The shared controller is resumed only after the target resource is gone.
func RetireClusterStep(ctx context.Context, client *k8s.Client, item *model.MySQLCluster) (bool, string, error) {
	state, err := InspectClusterCleanup(ctx, client, item)
	if err != nil {
		return false, "", err
	}
	if state.ClusterResource {
		current, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{})
		if err != nil {
			return false, "", err
		}
		if current.GetLabels()[k8s.ManagedByLabel] != k8s.ManagedByValue {
			return false, "", fmt.Errorf("集群资源不受平台管理，不能清退")
		}
		suspended, err := operatorSuspended(ctx, client)
		if err != nil {
			return false, "", err
		}
		if suspended {
			quiesced, err := operatorQuiesced(ctx, client)
			if err != nil {
				return false, "", err
			}
			if !quiesced {
				return false, "等待 MySQL Operator 完全退出后清退资源", nil
			}
			if len(state.Pods) > 0 {
				return false, "", fmt.Errorf("仍有数据库 Pod，不能清退停用的集群")
			}
			if err := exclusiveCluster(ctx, client, item); err != nil {
				return false, "", err
			}
		}
		if err := retainClusterData(ctx, client, item); err != nil {
			return false, "", err
		}
		if err := PauseCluster(ctx, client, item); err != nil {
			return false, "", err
		}
		if suspended {
			// No controller is available to release its finalizer. Only the
			// known Kopf finalizer is removable, after rechecking all workloads
			// are at zero and no Pods exist. Unknown finalizers are never stripped.
			if err := finalizeStoppedCluster(ctx, client, item); err != nil {
				return false, "", err
			}
		}
		return false, "已请求 Operator 移除集群资源，等待清理完成；数据卷保留", nil
	}
	state, err = CleanupPausedCluster(ctx, client, item, false)
	if err != nil {
		return false, "", err
	}
	if len(state.Workloads) > 0 || len(state.Pods) > 0 {
		return false, "等待残留负载退出；数据卷保留", nil
	}
	suspended, err := operatorSuspended(ctx, client)
	if err != nil {
		return false, "", err
	}
	if suspended {
		if err := exclusiveCluster(ctx, client, item); err != nil {
			return false, "", err
		}
		if err := setOperatorReplicas(ctx, client, 1); err != nil {
			return false, "", err
		}
	}
	return true, fmt.Sprintf("集群资源已清退，保留 %d 个数据卷；现在可以删除平台记录", len(state.Claims)), nil
}

// RemoveClusterRecordCheck is intentionally read-only. A record must never
// disappear while its custom resource can still recreate database workloads.
func RemoveClusterRecordCheck(ctx context.Context, client *k8s.Client, item *model.MySQLCluster) error {
	state, err := InspectClusterCleanup(ctx, client, item)
	if err != nil {
		return err
	}
	if state.ClusterResource || len(state.Workloads) > 0 || len(state.Pods) > 0 {
		return fmt.Errorf("请先清退集群资源，等待所有负载退出后再删除记录；数据卷可以保留")
	}
	return nil
}

func ValidateClusterLifecycle(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, action string) error {
	if action == "retire" {
		state, err := InspectClusterCleanup(ctx, client, item)
		if err != nil {
			return err
		}
		if len(state.Pods) > 0 {
			return fmt.Errorf("请先停用集群，等待所有 Pod 退出")
		}
		if state.ClusterResource {
			suspended, err := operatorSuspended(ctx, client)
			if err != nil {
				return err
			}
			if suspended {
				return exclusiveCluster(ctx, client, item)
			}
		}
		return nil
	}
	if err := lifecycleResource(ctx, client, item); err != nil {
		return err
	}
	if action == "stop" {
		if err := exclusiveCluster(ctx, client, item); err != nil {
			return err
		}
	} else {
		suspended, err := operatorSuspended(ctx, client)
		if err != nil {
			return err
		}
		if suspended {
			if err := exclusiveCluster(ctx, client, item); err != nil {
				return err
			}
		}
	}
	if action == "resume" && item.Status == "stopped" {
		stopped, err := ClusterStopped(ctx, client, item)
		if err != nil {
			return err
		}
		if !stopped {
			return fmt.Errorf("集群状态已变化，请刷新后重试")
		}
	}
	return nil
}

// Orphan data and the root credential before deleting their controllers. The
// Kubernetes garbage collector must not turn a retained-volume operation into
// a data deletion even when an older retention policy installed owner refs.
func retainClusterData(ctx context.Context, client *k8s.Client, item *model.MySQLCluster) error {
	sets, err := client.Clientset.AppsV1().StatefulSets(item.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, set := range sets.Items {
		if !clusterResourceName(set.Name, item.ResourceName) {
			continue
		}
		if policy := set.Spec.PersistentVolumeClaimRetentionPolicy; policy != nil && policy.WhenDeleted == appsv1.RetainPersistentVolumeClaimRetentionPolicyType && policy.WhenScaled == appsv1.RetainPersistentVolumeClaimRetentionPolicyType {
			continue
		}
		set.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{WhenDeleted: appsv1.RetainPersistentVolumeClaimRetentionPolicyType, WhenScaled: appsv1.RetainPersistentVolumeClaimRetentionPolicyType}
		if _, err := client.Clientset.AppsV1().StatefulSets(item.Namespace).Update(ctx, &set, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	state, err := InspectClusterCleanup(ctx, client, item)
	if err != nil {
		return err
	}
	retain := func(refs []metav1.OwnerReference) []metav1.OwnerReference {
		result := []metav1.OwnerReference{}
		for _, ref := range refs {
			if (ref.Kind == "InnoDBCluster" && ref.Name == item.ResourceName) || ((ref.Kind == "StatefulSet" || ref.Kind == "Pod") && clusterResourceName(ref.Name, item.ResourceName)) {
				continue
			}
			result = append(result, ref)
		}
		return result
	}
	for _, name := range state.Claims {
		claim, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		refs := retain(claim.OwnerReferences)
		if len(refs) == len(claim.OwnerReferences) {
			continue
		}
		claim.OwnerReferences = refs
		if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Update(ctx, claim, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	secret, err := client.Clientset.CoreV1().Secrets(item.Namespace).Get(ctx, item.ResourceName+"-root", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	refs := retain(secret.OwnerReferences)
	if len(refs) != len(secret.OwnerReferences) {
		if secret.Labels[k8s.ManagedByLabel] != k8s.ManagedByValue {
			return fmt.Errorf("根密钥不受平台管理，无法保证保留凭据")
		}
		secret.OwnerReferences = refs
		_, err = client.Clientset.CoreV1().Secrets(item.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
	}
	return err
}
func finalizeStoppedCluster(ctx context.Context, client *k8s.Client, item *model.MySQLCluster) error {
	resource := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace)
	current, err := resource.Get(ctx, item.ResourceName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	stopped, err := ClusterStopped(ctx, client, item)
	if err != nil {
		return err
	}
	if !stopped {
		return fmt.Errorf("仅允许在 Operator 停用、负载为零且无 Pod 时完成离线清退")
	}
	if current.GetDeletionTimestamp() == nil || current.GetLabels()[k8s.ManagedByLabel] != k8s.ManagedByValue {
		return fmt.Errorf("集群尚未进入受管删除流程")
	}
	for _, finalizer := range current.GetFinalizers() {
		if finalizer != "kopf.zalando.org/KopfFinalizerMarker" {
			return fmt.Errorf("集群仍有未知 finalizer %s，保留资源，请人工检查", finalizer)
		}
	}
	if len(current.GetFinalizers()) == 0 {
		return nil
	}
	current.SetFinalizers(nil)
	_, err = resource.Update(ctx, current, metav1.UpdateOptions{})
	return err
}
