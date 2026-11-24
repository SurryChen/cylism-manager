package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
)

type ComponentStorageSnapshot struct {
	Component   string                  `json:"component"`
	UID         types.UID               `json:"uid"`
	Node        string                  `json:"node"`
	PVC         string                  `json:"pvc"`
	Path        string                  `json:"path"`
	Deployment  *appsv1.DeploymentSpec  `json:"deployment,omitempty"`
	StatefulSet *appsv1.StatefulSetSpec `json:"stateful_set,omitempty"`
}

func componentWorkload(component string) (name, volume string, err error) {
	switch component {
	case InfrastructureVictoriaMetrics:
		return victoriaMetricsName, "storage", nil
	case InfrastructureLoki:
		return lokiName, "data", nil
	default:
		return "", "", fmt.Errorf("不支持的监控组件")
	}
}

func (c *Client) ComponentStorageSnapshot(ctx context.Context, component string) (*ComponentStorageSnapshot, error) {
	name, volumeName, err := componentWorkload(component)
	if err != nil {
		return nil, err
	}
	snapshot := &ComponentStorageSnapshot{Component: component}
	var pod *corev1.PodSpec
	var replicas *int32
	if component == InfrastructureVictoriaMetrics {
		d, e := c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Get(ctx, name, metav1.GetOptions{})
		if e != nil {
			return nil, e
		}
		if d.Labels[ManagedByLabel] != ManagedByValue {
			return nil, fmt.Errorf("组件未由平台管理")
		}
		snapshot.UID, snapshot.Deployment, pod, replicas = d.UID, d.Spec.DeepCopy(), &d.Spec.Template.Spec, d.Spec.Replicas
		if legacy := c.withContext(ctx).victoriaMetricsMigrationStatus(); legacy != nil {
			return nil, fmt.Errorf("请先完成旧 hostPath 存储迁移")
		}
	} else {
		s, e := c.Clientset.AppsV1().StatefulSets(victoriaMetricsNamespace).Get(ctx, name, metav1.GetOptions{})
		if e != nil {
			return nil, e
		}
		if s.Labels[ManagedByLabel] != ManagedByValue {
			return nil, fmt.Errorf("组件未由平台管理")
		}
		snapshot.UID, snapshot.StatefulSet, pod, replicas = s.UID, s.Spec.DeepCopy(), &s.Spec.Template.Spec, s.Spec.Replicas
	}
	if replicas == nil || *replicas != 1 {
		return nil, fmt.Errorf("仅支持运行中的单副本监控存储迁移")
	}
	snapshot.Node = pod.NodeSelector[corev1.LabelHostname]
	for _, v := range pod.Volumes {
		if v.Name == volumeName && v.PersistentVolumeClaim != nil {
			snapshot.PVC = v.PersistentVolumeClaim.ClaimName
		}
	}
	if snapshot.Node == "" || snapshot.PVC == "" {
		return nil, fmt.Errorf("请先将组件存储转换为受管 PVC")
	}
	claim, err := c.GetPVCInfoContext(ctx, victoriaMetricsNamespace, snapshot.PVC)
	if err != nil {
		return nil, err
	}
	if !claim.IsLocal || claim.BoundNode != snapshot.Node || claim.LocalPath == "" || path.Clean(claim.LocalPath) == "/" || !path.IsAbs(claim.LocalPath) {
		return nil, fmt.Errorf("仅支持绑定到当前数据节点的本地 PVC")
	}
	raw, err := c.Clientset.CoreV1().PersistentVolumeClaims(victoriaMetricsNamespace).Get(ctx, snapshot.PVC, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if raw.Status.Phase != corev1.ClaimBound {
		return nil, fmt.Errorf("源 PVC 尚未绑定")
	}
	if raw.Labels[ManagedByLabel] != ManagedByValue || raw.Labels[InfrastructureLabel] != component {
		return nil, fmt.Errorf("源卷不属于该监控组件")
	}
	snapshot.Path = claim.LocalPath
	return snapshot, nil
}

func migrationLockName(component string) string { return "cylism-" + component + "-migration-lock" }

func (c *Client) CheckComponentMigrationContext(ctx context.Context, component string) error {
	_, err := c.Clientset.CoreV1().ConfigMaps(victoriaMetricsNamespace).Get(ctx, migrationLockName(component), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("组件正在迁移或等待恢复，请先完成迁移任务")
}

func (c *Client) LockComponentMigration(ctx context.Context, component string, id uint) error {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: migrationLockName(component), Namespace: victoriaMetricsNamespace, Labels: infrastructurePVCLabels(component)}, Data: map[string]string{"task_id": strconv.FormatUint(uint64(id), 10)}}
	_, err := c.Clientset.CoreV1().ConfigMaps(victoriaMetricsNamespace).Create(ctx, cm, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		current, e := c.Clientset.CoreV1().ConfigMaps(victoriaMetricsNamespace).Get(ctx, cm.Name, metav1.GetOptions{})
		if e != nil {
			return e
		}
		if current.Data["task_id"] == cm.Data["task_id"] {
			return nil
		}
	}
	return err
}

func (c *Client) UnlockComponentMigration(ctx context.Context, component string, id uint) error {
	cm, err := c.Clientset.CoreV1().ConfigMaps(victoriaMetricsNamespace).Get(ctx, migrationLockName(component), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if cm.Data["task_id"] != strconv.FormatUint(uint64(id), 10) {
		return fmt.Errorf("迁移锁属于另一个任务")
	}
	return c.Clientset.CoreV1().ConfigMaps(victoriaMetricsNamespace).Delete(ctx, cm.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &cm.UID}})
}

func (c *Client) ValidateComponentMigrationNode(ctx context.Context, nodeName string) error {
	node, err := c.Clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if !nodeReady(node) || node.Spec.Unschedulable {
		return fmt.Errorf("节点 %s 未就绪或已禁止调度", nodeName)
	}
	for _, taint := range node.Spec.Taints {
		if taint.Effect == corev1.TaintEffectNoSchedule || taint.Effect == corev1.TaintEffectNoExecute {
			return fmt.Errorf("节点 %s 存在阻止调度的污点", nodeName)
		}
	}
	return nil
}

func (c *Client) ProvisionComponentMigration(ctx context.Context, snapshot *ComponentStorageSnapshot, targetNode, targetPVC string, id uint) (*PersistentVolumeClaimInfo, error) {
	claims := c.Clientset.CoreV1().PersistentVolumeClaims(victoriaMetricsNamespace)
	source, err := claims.Get(ctx, snapshot.PVC, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: targetPVC, Namespace: victoriaMetricsNamespace, Labels: infrastructurePVCLabels(snapshot.Component)}, Spec: corev1.PersistentVolumeClaimSpec{AccessModes: source.Spec.AccessModes, Resources: source.Spec.Resources, StorageClassName: source.Spec.StorageClassName, VolumeMode: source.Spec.VolumeMode}}
	claim.Labels["cylism.io/component-migration"] = strconv.FormatUint(uint64(id), 10)
	if _, err = claims.Create(ctx, claim, metav1.CreateOptions{}); err != nil {
		return nil, err
	}
	if _, err = c.CreatePVCBindingPodContext(ctx, victoriaMetricsNamespace, "obs-"+strconv.FormatUint(uint64(id), 10), targetPVC, targetNode, "registry.k8s.io/pause:3.10"); err != nil {
		return nil, err
	}
	boundCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	bound, err := c.WaitForPVCBound(boundCtx, victoriaMetricsNamespace, targetPVC)
	if err != nil {
		return nil, err
	}
	if !bound.IsLocal || bound.BoundNode != targetNode || !path.IsAbs(bound.LocalPath) || path.Clean(bound.LocalPath) == "/" {
		return nil, fmt.Errorf("目标 PVC 未绑定到所选节点的本地目录")
	}
	return bound, nil
}

// Change only the workload spec; Services, collectors and retention configuration
// keep their identities. The snapshot also permits recovery after a process exit.
func (c *Client) SetComponentStorage(ctx context.Context, snapshot *ComponentStorageSnapshot, pvc, node string, replicas int32, restore bool) error {
	name, volumeName, err := componentWorkload(snapshot.Component)
	if err != nil {
		return err
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		change := func(p *corev1.PodSpec) {
			if p.NodeSelector == nil {
				p.NodeSelector = map[string]string{}
			}
			p.NodeSelector[corev1.LabelHostname] = node
			for i := range p.Volumes {
				if p.Volumes[i].Name == volumeName {
					p.Volumes[i].VolumeSource = corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvc}}
				}
			}
		}
		if snapshot.Component == InfrastructureVictoriaMetrics {
			d, e := c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Get(ctx, name, metav1.GetOptions{})
			if e != nil {
				return e
			}
			if d.UID != snapshot.UID {
				return fmt.Errorf("工作负载已被替换，停止自动修改")
			}
			if restore {
				d.Spec = *snapshot.Deployment.DeepCopy()
			}
			change(&d.Spec.Template.Spec)
			d.Spec.Replicas = &replicas
			_, e = c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Update(ctx, d, metav1.UpdateOptions{})
			return e
		}
		s, e := c.Clientset.AppsV1().StatefulSets(victoriaMetricsNamespace).Get(ctx, name, metav1.GetOptions{})
		if e != nil {
			return e
		}
		if s.UID != snapshot.UID {
			return fmt.Errorf("工作负载已被替换，停止自动修改")
		}
		if restore {
			s.Spec = *snapshot.StatefulSet.DeepCopy()
		}
		change(&s.Spec.Template.Spec)
		s.Spec.Replicas = &replicas
		_, e = c.Clientset.AppsV1().StatefulSets(victoriaMetricsNamespace).Update(ctx, s, metav1.UpdateOptions{})
		return e
	})
}

func (c *Client) WaitComponentStorage(ctx context.Context, snapshot *ComponentStorageSnapshot, node, pvc string, ready bool) error {
	var selector map[string]string
	if snapshot.Deployment != nil {
		selector = snapshot.Deployment.Selector.MatchLabels
	} else {
		selector = snapshot.StatefulSet.Selector.MatchLabels
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		pods, err := c.Clientset.CoreV1().Pods(victoriaMetricsNamespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(selector).String()})
		if err != nil {
			return err
		}
		if !ready && len(pods.Items) == 0 {
			return nil
		}
		if ready && len(pods.Items) == 1 {
			p := pods.Items[0]
			attached := false
			for _, v := range p.Spec.Volumes {
				if v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == pvc {
					attached = true
				}
			}
			if p.Spec.NodeName == node && attached && p.DeletionTimestamp == nil {
				for _, cond := range p.Status.Conditions {
					if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
						return nil
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("等待组件停止或就绪超时: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func EncodeComponentStorageSnapshot(snapshot *ComponentStorageSnapshot) string {
	data, _ := json.Marshal(snapshot)
	return string(data)
}
