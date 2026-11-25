package k8s

import (
	"context"
	"fmt"
	"path"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func (c *Client) namespaceMoveClaimInfo(ctx context.Context, claim *corev1.PersistentVolumeClaim) (PersistentVolumeClaimInfo, error) {
	info, err := c.pvcInfo(ctx, claim)
	if err != nil {
		return info, err
	}
	if claim.Spec.VolumeName == "" || claim.DeletionTimestamp != nil {
		return info, fmt.Errorf("卷 %s 尚未绑定或正在删除", claim.Name)
	}
	pv, err := c.Clientset.CoreV1().PersistentVolumes().Get(ctx, claim.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return info, err
	}
	ref := pv.Spec.ClaimRef
	if pv.DeletionTimestamp != nil || pv.Annotations["pv.kubernetes.io/provisioned-by"] != "rancher.io/local-path" ||
		ref == nil || ref.UID != claim.UID || ref.Namespace != claim.Namespace || ref.Name != claim.Name ||
		info.BoundNode != persistentVolumeNodeName(pv) {
		return info, fmt.Errorf("卷 %s 的 PV 与本地卷声明不一致", claim.Name)
	}
	localPath, local := persistentVolumeLocalPath(pv)
	if !local || localPath != info.LocalPath {
		return info, fmt.Errorf("卷 %s 的目录已变更", claim.Name)
	}
	return info, nil
}

func (c *Client) planNamespaceMoveClaims(ctx context.Context, move ApplicationNamespaceMove, source *namespaceWorkload, plan *ApplicationNamespaceMaintenancePlan, pods []corev1.Pod) error {
	seen := map[string]bool{}
	for _, volume := range source.template.Spec.Volumes {
		switch {
		case volume.ConfigMap != nil, volume.Secret != nil, volume.EmptyDir != nil, volume.DownwardAPI != nil:
			continue
		case volume.PersistentVolumeClaim == nil:
			return fmt.Errorf("卷 %s 的类型需要单独迁移", volume.Name)
		}
		name := volume.PersistentVolumeClaim.ClaimName
		if seen[name] {
			continue
		}
		seen[name] = true
		claim, err := c.Clientset.CoreV1().PersistentVolumeClaims(move.SourceNamespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		info, err := c.namespaceMoveClaimInfo(ctx, claim)
		if err != nil {
			return err
		}
		if !isManagedPVC(claim, move.SourceEnvironmentID) || info.ReadOnly || !info.IsLocal || !info.WaitForFirstConsumer ||
			info.Phase != string(corev1.ClaimBound) || info.BoundNode == "" || !path.IsAbs(info.LocalPath) || info.LocalPath == "/" ||
			!slices.Equal(claim.Spec.AccessModes, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}) ||
			(claim.Spec.VolumeMode != nil && *claim.Spec.VolumeMode != corev1.PersistentVolumeFilesystem) {
			return fmt.Errorf("卷 %s 需要是平台管理的本地 RWO 文件系统卷", name)
		}
		class, err := c.Clientset.StorageV1().StorageClasses().Get(ctx, info.StorageClassName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if class.Provisioner != "rancher.io/local-path" || class.VolumeBindingMode == nil || *class.VolumeBindingMode != storagev1.VolumeBindingWaitForFirstConsumer {
			return fmt.Errorf("卷 %s 的 StorageClass 不支持跨命名空间复制", name)
		}
		if plan.Node != "" && plan.Node != info.BoundNode {
			return fmt.Errorf("应用的数据卷或宿主机端口位于不同节点")
		}
		for _, pod := range pods {
			if pod.Spec.NodeName != info.BoundNode {
				return fmt.Errorf("源 Pod 未运行在数据卷所在节点")
			}
		}
		plan.Node = info.BoundNode
		plan.Claims = append(plan.Claims, NamespaceMoveClaim{Name: name, UID: string(claim.UID), Volume: info.VolumeName,
			Class: info.StorageClassName, Size: info.Storage, Node: info.BoundNode, Path: info.LocalPath})
		if err := c.ensureNamespaceMoveClaimUsers(ctx, move.SourceNamespace, name, pods); err != nil {
			return err
		}
	}
	if plan.Node != "" {
		node, err := c.Clientset.CoreV1().Nodes().Get(ctx, plan.Node, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if node.Spec.Unschedulable || node.Labels[corev1.LabelHostname] != plan.Node ||
			!slices.ContainsFunc(node.Status.Conditions, func(condition corev1.NodeCondition) bool {
				return condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue
			}) {
			return fmt.Errorf("目标节点 %s 当前不可调度", plan.Node)
		}
	}
	return nil
}

func (c *Client) ensureNamespaceMoveClaimUsers(ctx context.Context, namespace, name string, allowed []corev1.Pod) error {
	pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed ||
			slices.ContainsFunc(allowed, func(item corev1.Pod) bool { return item.UID == pod.UID }) {
			continue
		}
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == name {
				return fmt.Errorf("卷 %s 仍被 Pod %s 使用", name, pod.Name)
			}
		}
	}
	return nil
}

func (c *Client) namespaceMoveTargetClaim(ctx context.Context, move ApplicationNamespaceMove, item NamespaceMoveClaim, marker string, create bool) (*corev1.PersistentVolumeClaim, error) {
	claims := c.Clientset.CoreV1().PersistentVolumeClaims(move.TargetNamespace)
	claim, err := claims.Get(ctx, item.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) && create {
		return claims.Create(ctx, &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: item.Name, Labels: map[string]string{ManagedByLabel: ManagedByValue, EnvironmentLabel: EnvironmentLabelValue(move.TargetEnvironmentID)},
				Annotations: map[string]string{applicationMoveAnnotation: marker}},
			Spec: corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				StorageClassName: &item.Class, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(item.Size)}}},
		}, metav1.CreateOptions{})
	}
	if err != nil {
		return nil, err
	}
	return claim, checkNamespaceMoveResource(claim, nil, marker)
}

func (c *Client) prepareNamespaceMoveClaims(ctx context.Context, move ApplicationNamespaceMove, plan *ApplicationNamespaceMaintenancePlan, marker string) error {
	for index, item := range plan.Claims {
		claim, err := c.namespaceMoveTargetClaim(ctx, move, item, marker, true)
		if err != nil {
			return err
		}
		name := fmt.Sprintf("cylism-move-%s-%d", marker, index)
		pods := c.Clientset.CoreV1().Pods(move.TargetNamespace)
		if claim.Status.Phase != corev1.ClaimBound {
			automount := false
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{ManagedByLabel: ManagedByValue},
				Annotations: map[string]string{applicationMoveAnnotation: marker}},
				Spec: corev1.PodSpec{RestartPolicy: corev1.RestartPolicyNever, AutomountServiceAccountToken: &automount,
					NodeSelector: map[string]string{corev1.LabelHostname: item.Node},
					Containers: []corev1.Container{{Name: "holder", Image: "registry.k8s.io/pause:3.10",
						VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: "/data"}}}},
					Volumes: []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: item.Name}}}},
				}}
			_, err := pods.Create(ctx, pod, metav1.CreateOptions{})
			if apierrors.IsAlreadyExists(err) {
				current, getErr := pods.Get(ctx, name, metav1.GetOptions{})
				err = checkNamespaceMoveResource(current, getErr, marker)
				if getErr != nil {
					err = getErr
				}
			}
			if err != nil {
				return err
			}
			if _, err := c.WaitForManagedPVCBound(ctx, move.TargetNamespace, item.Name, move.TargetEnvironmentID); err != nil {
				return err
			}
		}
		pod, err := pods.Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			if pod.Annotations[applicationMoveAnnotation] != marker || pod.Labels[ManagedByLabel] != ManagedByValue {
				return fmt.Errorf("目标已有同名绑定 Pod")
			}
			uid := pod.UID
			if err := pods.Delete(ctx, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		} else if !apierrors.IsNotFound(err) {
			return err
		}
		if err := wait.PollUntilContextTimeout(ctx, time.Second, time.Minute, true, func(ctx context.Context) (bool, error) {
			_, err := pods.Get(ctx, name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) namespaceMoveClaimPaths(ctx context.Context, move ApplicationNamespaceMove, item NamespaceMoveClaim, marker string, allowed []corev1.Pod) (string, string, error) {
	source, err := c.Clientset.CoreV1().PersistentVolumeClaims(move.SourceNamespace).Get(ctx, item.Name, metav1.GetOptions{})
	if err != nil {
		return "", "", err
	}
	si, err := c.namespaceMoveClaimInfo(ctx, source)
	if err != nil {
		return "", "", err
	}
	if string(source.UID) != item.UID || si.VolumeName != item.Volume || si.LocalPath != item.Path || si.StorageClassName != item.Class || si.BoundNode != item.Node || si.Phase != string(corev1.ClaimBound) {
		return "", "", fmt.Errorf("源卷 %s 已变更", item.Name)
	}
	target, err := c.namespaceMoveTargetClaim(ctx, move, item, marker, false)
	if err != nil {
		return "", "", err
	}
	ti, err := c.namespaceMoveClaimInfo(ctx, target)
	if err != nil {
		return "", "", err
	}
	if !ti.IsLocal || ti.Phase != string(corev1.ClaimBound) || ti.BoundNode != item.Node || ti.StorageClassName != item.Class ||
		ti.VolumeName == si.VolumeName || !path.IsAbs(ti.LocalPath) || path.Clean(ti.LocalPath) == "/" {
		return "", "", fmt.Errorf("目标卷 %s 未绑定到预检节点上的独立本地卷", item.Name)
	}
	want, parseErr := resource.ParseQuantity(item.Size)
	got, gotErr := resource.ParseQuantity(ti.Storage)
	if parseErr != nil || gotErr != nil || got.Cmp(want) < 0 {
		return "", "", fmt.Errorf("目标卷 %s 容量不足", item.Name)
	}
	if err := c.ensureNamespaceMoveClaimUsers(ctx, move.SourceNamespace, item.Name, allowed); err != nil {
		return "", "", err
	}
	if err := c.ensureNamespaceMoveClaimUsers(ctx, move.TargetNamespace, item.Name, nil); err != nil {
		return "", "", err
	}
	return si.LocalPath, ti.LocalPath, nil
}
