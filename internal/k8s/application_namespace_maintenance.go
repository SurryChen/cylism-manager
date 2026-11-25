package k8s

import (
	"context"
	"fmt"
	"strconv"

	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NamespaceMoveCopy func(ctx context.Context, source, target string, copyData bool) error

func (c *Client) CheckApplicationNamespaceMaintenance(ctx context.Context, move ApplicationNamespaceMove, plan *ApplicationNamespaceMaintenancePlan, marker string) error {
	for _, kind := range []string{"deployment", "statefulset"} {
		obj, err := c.namespaceWorkload(ctx, move.TargetNamespace, move.Name, kind)
		if !apierrors.IsNotFound(err) {
			if err != nil {
				return err
			}
			if kind != plan.Kind || marker == "" || checkNamespaceMoveResource(obj.meta, nil, marker) != nil {
				return fmt.Errorf("目标环境已有同名工作负载")
			}
		}
	}
	check := func(object metav1.Object, err error) error {
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if marker == "" || checkNamespaceMoveResource(object, nil, marker) != nil {
			return fmt.Errorf("目标环境已有同名资源 %s", object.GetName())
		}
		return nil
	}
	core := c.Clientset.CoreV1()
	if err := check(core.Services(move.TargetNamespace).Get(ctx, move.Name, metav1.GetOptions{})); err != nil {
		return err
	}
	if err := check(c.Clientset.NetworkingV1().Ingresses(move.TargetNamespace).Get(ctx, move.Name, metav1.GetOptions{})); err != nil {
		return err
	}
	for _, item := range plan.Claims {
		if err := check(core.PersistentVolumeClaims(move.TargetNamespace).Get(ctx, item.Name, metav1.GetOptions{})); err != nil {
			return err
		}
	}
	for _, name := range plan.ConfigMaps {
		if err := check(core.ConfigMaps(move.TargetNamespace).Get(ctx, name, metav1.GetOptions{})); err != nil {
			return err
		}
	}
	for _, name := range plan.Secrets {
		obj, err := core.Secrets(move.TargetNamespace).Get(ctx, name, metav1.GetOptions{})
		if err := check(obj, err); err != nil {
			if obj == nil || !c.reusableNamespaceMoveTLS(ctx, move, obj) {
				return err
			}
		}
	}
	return nil
}

func (c *Client) PrepareApplicationNamespaceMaintenance(ctx context.Context, move ApplicationNamespaceMove, plan *ApplicationNamespaceMaintenancePlan, marker string, copyVolume NamespaceMoveCopy) error {
	source, err := c.sourceNamespaceMaintenanceWorkload(ctx, move, plan)
	if err != nil {
		return err
	}
	if *source.replicas != plan.Replicas {
		return fmt.Errorf("预配前源工作负载需要保持运行")
	}
	if err := c.CheckApplicationNamespaceMaintenance(ctx, move, plan, marker); err != nil {
		return err
	}
	if err := c.prepareNamespaceMoveResources(ctx, move, source, plan, marker, true); err != nil {
		return err
	}
	if err := c.prepareNamespaceMoveClaims(ctx, move, plan, marker); err != nil {
		return err
	}
	if err := c.prepareNamespaceMaintenanceWorkload(ctx, move, source, plan, marker); err != nil {
		return err
	}
	target, err := c.targetNamespaceMaintenanceWorkload(ctx, move, source, plan, marker)
	if err != nil {
		return err
	}
	if err := c.waitNamespaceMaintenanceStopped(ctx, target); err != nil {
		return err
	}
	return c.checkNamespaceMaintenanceVolumes(ctx, move, plan, marker, source, copyVolume)
}

func (c *Client) checkNamespaceMaintenanceVolumes(ctx context.Context, move ApplicationNamespaceMove, plan *ApplicationNamespaceMaintenancePlan, marker string, source *namespaceWorkload, copyVolume NamespaceMoveCopy) error {
	pods, err := c.namespaceWorkloadPods(ctx, source)
	if err != nil {
		return err
	}
	for _, item := range plan.Claims {
		from, to, err := c.namespaceMoveClaimPaths(ctx, move, item, marker, pods)
		if err != nil {
			return err
		}
		if err := copyVolume(ctx, from, to, false); err != nil {
			return fmt.Errorf("卷 %s 的目录预检失败: %w", item.Name, err)
		}
	}
	return nil
}

func (c *Client) CopyApplicationNamespaceMaintenance(ctx context.Context, move ApplicationNamespaceMove, plan *ApplicationNamespaceMaintenancePlan, marker string, endpoints []model.ApplicationEndpoint, copyVolume NamespaceMoveCopy) error {
	source, err := c.sourceNamespaceMaintenanceWorkload(ctx, move, plan)
	if err != nil {
		return err
	}
	target, err := c.targetNamespaceMaintenanceWorkload(ctx, move, source, plan, marker)
	if err != nil {
		return err
	}
	if err := c.waitNamespaceMaintenanceStopped(ctx, target); err != nil {
		return err
	}
	if err := c.prepareNamespaceMoveResources(ctx, move, source, plan, marker, false); err != nil {
		return err
	}
	if err := c.SwitchApplicationNamespaceMaintenanceRoutes(ctx, move, plan, marker, endpoints, false); err != nil {
		return err
	}
	if err := c.checkNamespaceMaintenanceVolumes(ctx, move, plan, marker, source, copyVolume); err != nil {
		return err
	}
	if *source.replicas != 0 {
		pods, err := c.namespaceWorkloadPods(ctx, source)
		if err != nil {
			return err
		}
		if len(pods) != int(plan.Replicas) {
			return fmt.Errorf("源 Pod 数量自预检后已变更")
		}
		for _, pod := range pods {
			if namespaceMovePodImage(pod, strconv.FormatUint(uint64(move.SourceSequence), 10), source.template.Spec.Containers[0].Name) != plan.ImageID ||
				(plan.Node != "" && pod.Spec.NodeName != plan.Node) {
				return fmt.Errorf("源 Pod 镜像或节点自预检后已变更")
			}
		}
	}
	if err := c.scaleNamespaceMaintenanceWorkload(ctx, source, 0); err != nil {
		return err
	}
	if err := c.waitNamespaceMaintenanceStopped(ctx, source); err != nil {
		return err
	}
	for _, item := range plan.Claims {
		from, to, err := c.namespaceMoveClaimPaths(ctx, move, item, marker, nil)
		if err != nil {
			return err
		}
		if err := copyVolume(ctx, from, to, true); err != nil {
			return fmt.Errorf("卷 %s 复制或校验失败: %w", item.Name, err)
		}
	}
	return nil
}

func (c *Client) RunApplicationNamespaceMaintenanceTarget(ctx context.Context, move ApplicationNamespaceMove, plan *ApplicationNamespaceMaintenancePlan, marker string, start bool) (bool, error) {
	source, err := c.sourceNamespaceMaintenanceWorkload(ctx, move, plan)
	if err != nil {
		return false, err
	}
	target, err := c.targetNamespaceMaintenanceWorkload(ctx, move, source, plan, marker)
	if err != nil {
		return false, err
	}
	if *source.replicas != 0 {
		return false, fmt.Errorf("源工作负载仍在运行")
	}
	if start {
		if *target.replicas == 0 {
			if err := c.prepareNamespaceMoveResources(ctx, move, source, plan, marker, false); err != nil {
				return false, err
			}
			if err := c.checkNamespaceMaintenanceVolumeBindings(ctx, move, plan, marker); err != nil {
				return false, err
			}
		}
		if err := c.waitNamespaceMaintenanceStopped(ctx, source); err != nil {
			return false, err
		}
		if err := c.scaleNamespaceMaintenanceWorkload(ctx, target, plan.Replicas); err != nil {
			return false, err
		}
		return false, nil
	}
	service, err := c.Clientset.CoreV1().Services(move.TargetNamespace).Get(ctx, move.Name, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	if err := checkNamespaceMoveResource(service, nil, marker); err != nil {
		return false, err
	}
	return c.namespaceMaintenanceReady(ctx, move, target, plan)
}

func (c *Client) checkNamespaceMaintenanceVolumeBindings(ctx context.Context, move ApplicationNamespaceMove, plan *ApplicationNamespaceMaintenancePlan, marker string) error {
	for _, item := range plan.Claims {
		if _, _, err := c.namespaceMoveClaimPaths(ctx, move, item, marker, nil); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) RestoreApplicationNamespaceMaintenanceSource(ctx context.Context, move ApplicationNamespaceMove, plan *ApplicationNamespaceMaintenancePlan, marker string) error {
	source, err := c.sourceNamespaceMaintenanceWorkload(ctx, move, plan)
	if err != nil {
		return err
	}
	target, err := c.targetNamespaceMaintenanceWorkload(ctx, move, source, plan, marker)
	if apierrors.IsNotFound(err) && *source.replicas == plan.Replicas {
		return nil
	}
	if err != nil {
		return err
	}
	if *target.replicas != 0 && len(plan.Claims) == 0 {
		if err := c.scaleNamespaceMaintenanceWorkload(ctx, target, 0); err != nil {
			return err
		}
	}
	if err := c.waitNamespaceMaintenanceStopped(ctx, target); err != nil {
		return err
	}
	return c.updateNamespaceMaintenanceWorkload(ctx, source, plan.Replicas, plan.PinnedImage)
}

func (c *Client) CleanupApplicationNamespaceMaintenance(ctx context.Context, move ApplicationNamespaceMove, plan *ApplicationNamespaceMaintenancePlan, marker string) error {
	source, err := c.sourceNamespaceMaintenanceWorkload(ctx, move, plan)
	if err != nil {
		return err
	}
	if *source.replicas != plan.Replicas {
		return fmt.Errorf("源工作负载尚未恢复")
	}
	target, err := c.targetNamespaceMaintenanceWorkload(ctx, move, source, plan, marker)
	if err == nil {
		if err := c.waitNamespaceMaintenanceStopped(ctx, target); err != nil {
			return err
		}
		uid := target.meta.UID
		options := metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}
		if target.dep != nil {
			err = c.Clientset.AppsV1().Deployments(move.TargetNamespace).Delete(ctx, move.Name, options)
		} else {
			err = c.Clientset.AppsV1().StatefulSets(move.TargetNamespace).Delete(ctx, move.Name, options)
		}
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	// All resources below were created exclusively for this attempt. The
	// maintenance checkpoint must still precede any target application start.
	core := c.Clientset.CoreV1()
	ns := move.TargetNamespace
	for index, claim := range plan.Claims {
		if err := deleteNamespaceMoveResource(ctx, core.Pods(ns), fmt.Sprintf("cylism-move-%s-%d", marker, index), marker); err != nil {
			return err
		}
		if err := c.ensureNamespaceMoveClaimUsers(ctx, ns, claim.Name, nil); err != nil {
			return err
		}
		if err := deleteNamespaceMoveResource(ctx, core.PersistentVolumeClaims(ns), claim.Name, marker); err != nil {
			return err
		}
	}
	for _, name := range plan.ConfigMaps {
		if err := deleteNamespaceMoveResource(ctx, core.ConfigMaps(ns), name, marker); err != nil {
			return err
		}
	}
	for _, name := range plan.Secrets {
		if err := deleteNamespaceMoveResource(ctx, core.Secrets(ns), name, marker); err != nil {
			return err
		}
	}
	return deleteNamespaceMoveResource(ctx, core.Services(ns), move.Name, marker)
}

type namespaceMoveDeleter[T metav1.Object] interface {
	Get(context.Context, string, metav1.GetOptions) (T, error)
	Delete(context.Context, string, metav1.DeleteOptions) error
}

func deleteNamespaceMoveResource[T metav1.Object](ctx context.Context, client namespaceMoveDeleter[T], name, marker string) error {
	item, err := client.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if item.GetAnnotations()[applicationMoveAnnotation] != marker {
		return nil
	}
	uid := item.GetUID()
	err = client.Delete(ctx, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func namespaceMoveTLSSecretReady(secret *corev1.Secret) bool {
	return secret.Type == corev1.SecretTypeTLS && len(secret.Data[corev1.TLSCertKey]) > 0 && len(secret.Data[corev1.TLSPrivateKeyKey]) > 0
}
