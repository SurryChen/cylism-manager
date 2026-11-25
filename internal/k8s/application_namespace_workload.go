package k8s

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

type ApplicationNamespaceMaintenancePlan struct {
	Kind, SourceUID, SourceSpecHash, SourcePinnedSpecHash, ImageID, PinnedImage, Node string
	Replicas                                                                          int32
	ManualValidation                                                                  bool
	Claims                                                                            []NamespaceMoveClaim
	ConfigMaps, Secrets                                                               []string
	Dependencies, Service, Ingress                                                    string
}

type NamespaceMoveClaim struct {
	Name, UID, Volume, Class, Size, Node, Path string
}

type namespaceWorkload struct {
	meta     *metav1.ObjectMeta
	replicas *int32
	template *corev1.PodTemplateSpec
	selector *metav1.LabelSelector
	observed int64
	ready    int32
	specHash string
	dep      *appsv1.Deployment
	sts      *appsv1.StatefulSet
}

func namespaceMoveHash(value interface{}) string {
	data, _ := json.Marshal(value)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func (c *Client) namespaceWorkload(ctx context.Context, namespace, name, kind string) (*namespaceWorkload, error) {
	if kind == "statefulset" {
		obj, err := c.Clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		spec := obj.Spec.DeepCopy()
		spec.Replicas = nil
		return &namespaceWorkload{meta: &obj.ObjectMeta, replicas: obj.Spec.Replicas, template: &obj.Spec.Template, selector: obj.Spec.Selector,
			observed: obj.Status.ObservedGeneration, ready: obj.Status.ReadyReplicas, specHash: namespaceMoveHash(spec), sts: obj}, nil
	}
	obj, err := c.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	spec := obj.Spec.DeepCopy()
	spec.Replicas = nil
	return &namespaceWorkload{meta: &obj.ObjectMeta, replicas: obj.Spec.Replicas, template: &obj.Spec.Template, selector: obj.Spec.Selector,
		observed: obj.Status.ObservedGeneration, ready: obj.Status.AvailableReplicas, specHash: namespaceMoveHash(spec), dep: obj}, nil
}

func (c *Client) namespaceWorkloadPods(ctx context.Context, workload *namespaceWorkload) ([]corev1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(workload.selector)
	if err != nil || selector.Empty() {
		return nil, fmt.Errorf("工作负载 selector 无效")
	}
	pods, err := c.Clientset.CoreV1().Pods(workload.meta.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	for _, pod := range pods.Items {
		owner := metav1.GetControllerOf(&pod)
		if owner != nil && workload.sts != nil && owner.Kind == "StatefulSet" && owner.UID == workload.meta.UID {
			continue
		}
		if owner != nil && workload.dep != nil && owner.Kind == "ReplicaSet" {
			rs, err := c.Clientset.AppsV1().ReplicaSets(workload.meta.Namespace).Get(ctx, owner.Name, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			parent := metav1.GetControllerOf(rs)
			if rs.UID == owner.UID && parent != nil && parent.Kind == "Deployment" && parent.UID == workload.meta.UID {
				continue
			}
		}
		return nil, fmt.Errorf("工作负载 selector 匹配了不受其管理的 Pod %s", pod.Name)
	}
	return pods.Items, nil
}

func (c *Client) PlanApplicationNamespaceMaintenance(ctx context.Context, move ApplicationNamespaceMove, kind string) (*ApplicationNamespaceMaintenancePlan, error) {
	workload, err := c.namespaceWorkload(ctx, move.SourceNamespace, move.Name, kind)
	if err != nil {
		return nil, err
	}
	sequence := strconv.FormatUint(uint64(move.SourceSequence), 10)
	if ensureApplicationManaged(workload.meta.Labels) != nil || workload.meta.Labels["app.kubernetes.io/part-of"] != fmt.Sprintf("project-%d", move.SourceProjectID) ||
		workload.template.Labels["cylism.io/release"] != sequence || workload.meta.DeletionTimestamp != nil {
		return nil, fmt.Errorf("源工作负载与应用的成功发布记录不一致")
	}
	spec := workload.template.Spec
	if spec.HostPID || spec.HostIPC || len(spec.Containers) != 1 || len(spec.InitContainers) != 0 ||
		(spec.ServiceAccountName != "" && spec.ServiceAccountName != "default") ||
		(spec.Containers[0].SecurityContext != nil && spec.Containers[0].SecurityContext.Privileged != nil && *spec.Containers[0].SecurityContext.Privileged) ||
		(workload.dep != nil && workload.dep.Spec.Paused) ||
		(workload.sts != nil && (len(workload.sts.Spec.VolumeClaimTemplates) != 0 || workload.sts.Spec.ServiceName != move.Name)) {
		return nil, fmt.Errorf("源工作负载包含需要单独迁移的运行配置")
	}
	if workload.replicas == nil || *workload.replicas < 1 || workload.observed < workload.meta.Generation || workload.ready != *workload.replicas {
		return nil, fmt.Errorf("源工作负载尚未全部就绪")
	}
	plan := &ApplicationNamespaceMaintenancePlan{Kind: kind, SourceUID: string(workload.meta.UID), SourceSpecHash: workload.specHash,
		Replicas: *workload.replicas, ManualValidation: spec.Containers[0].ReadinessProbe == nil}
	pods, err := c.namespaceWorkloadPods(ctx, workload)
	if err != nil {
		return nil, err
	}
	if len(pods) != int(plan.Replicas) {
		return nil, fmt.Errorf("源 Pod 正在变更")
	}
	fixedNode := spec.HostNetwork
	for _, port := range spec.Containers[0].Ports {
		fixedNode = fixedNode || port.HostPort != 0
	}
	for _, pod := range pods {
		if !slices.Equal(pod.Spec.ImagePullSecrets, spec.ImagePullSecrets) {
			return nil, fmt.Errorf("源 Pod 使用工作负载之外注入的镜像拉取凭据")
		}
		image := namespaceMovePodImage(pod, sequence, spec.Containers[0].Name)
		if image == "" || (plan.ImageID != "" && plan.ImageID != image) {
			return nil, fmt.Errorf("无法确认源 Pod 使用同一镜像")
		}
		plan.ImageID = image
		if fixedNode {
			if pod.Spec.NodeName == "" || (plan.Node != "" && plan.Node != pod.Spec.NodeName) {
				return nil, fmt.Errorf("宿主机端口迁移要求源 Pod 位于同一节点")
			}
			plan.Node = pod.Spec.NodeName
		}
	}
	// A repository digest pins the image even when the release used a mutable tag.
	image := plan.ImageID
	if _, rest, ok := strings.Cut(image, "://"); ok {
		image = rest
	}
	_, digest, found := strings.Cut(image, "@sha256:")
	if !found || len(digest) != 64 {
		return nil, fmt.Errorf("源容器运行时未提供可固定的镜像仓库摘要")
	}
	plan.PinnedImage = image
	if workload.dep != nil {
		pinned := workload.dep.Spec.DeepCopy()
		pinned.Replicas = nil
		pinned.Template.Spec.Containers[0].Image = image
		plan.SourcePinnedSpecHash = namespaceMoveHash(pinned)
	} else {
		pinned := workload.sts.Spec.DeepCopy()
		pinned.Replicas = nil
		pinned.Template.Spec.Containers[0].Image = image
		plan.SourcePinnedSpecHash = namespaceMoveHash(pinned)
	}

	if err := c.planNamespaceMoveClaims(ctx, move, workload, plan, pods); err != nil {
		return nil, err
	}
	if err := c.planNamespaceMoveResources(ctx, move, workload, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func (c *Client) sourceNamespaceMaintenanceWorkload(ctx context.Context, move ApplicationNamespaceMove, plan *ApplicationNamespaceMaintenancePlan) (*namespaceWorkload, error) {
	source, err := c.namespaceWorkload(ctx, move.SourceNamespace, move.Name, plan.Kind)
	if err != nil {
		return nil, err
	}
	if string(source.meta.UID) != plan.SourceUID || (source.specHash != plan.SourceSpecHash && source.specHash != plan.SourcePinnedSpecHash) || source.replicas == nil ||
		(*source.replicas != 0 && *source.replicas != plan.Replicas) || source.meta.DeletionTimestamp != nil {
		return nil, fmt.Errorf("源工作负载已变更")
	}
	return source, nil
}

func (c *Client) prepareNamespaceMaintenanceWorkload(ctx context.Context, move ApplicationNamespaceMove, source *namespaceWorkload, plan *ApplicationNamespaceMaintenancePlan, marker string) error {
	if _, err := c.targetNamespaceMaintenanceWorkload(ctx, move, source, plan, marker); !apierrors.IsNotFound(err) {
		return err
	}
	project := fmt.Sprintf("project-%d", move.TargetProjectID)
	environment := EnvironmentLabelValue(move.TargetEnvironmentID)
	meta := moveMetadata(*source.meta.DeepCopy(), move.TargetNamespace, marker, project, environment)
	template := source.template.DeepCopy()
	template.Labels["app.kubernetes.io/part-of"] = project
	template.Labels[EnvironmentLabel] = environment
	template.Spec.Containers[0].Image = plan.PinnedImage
	if plan.Node != "" {
		if template.Spec.NodeSelector == nil {
			template.Spec.NodeSelector = map[string]string{}
		}
		template.Spec.NodeSelector[corev1.LabelHostname] = plan.Node
	}
	var zero int32
	if source.dep != nil {
		obj := &appsv1.Deployment{ObjectMeta: meta, Spec: *source.dep.Spec.DeepCopy()}
		obj.Spec.Replicas, obj.Spec.Template = &zero, *template
		obj.Annotations[namespaceMoveSpecAnnotation] = namespaceMoveHash(obj.Spec)
		_, err := c.Clientset.AppsV1().Deployments(move.TargetNamespace).Create(ctx, obj, metav1.CreateOptions{})
		return err
	}
	obj := &appsv1.StatefulSet{ObjectMeta: meta, Spec: *source.sts.Spec.DeepCopy()}
	obj.Spec.Replicas, obj.Spec.Template = &zero, *template
	obj.Annotations[namespaceMoveSpecAnnotation] = namespaceMoveHash(obj.Spec)
	_, err := c.Clientset.AppsV1().StatefulSets(move.TargetNamespace).Create(ctx, obj, metav1.CreateOptions{})
	return err
}

const namespaceMoveSpecAnnotation = "cylism.io/namespace-move-spec"

func (c *Client) targetNamespaceMaintenanceWorkload(ctx context.Context, move ApplicationNamespaceMove, source *namespaceWorkload, plan *ApplicationNamespaceMaintenancePlan, marker string) (*namespaceWorkload, error) {
	target, err := c.namespaceWorkload(ctx, move.TargetNamespace, move.Name, plan.Kind)
	if err != nil {
		return nil, err
	}
	if err := checkNamespaceMoveResource(target.meta, nil, marker); err != nil {
		return nil, err
	}
	var zero int32
	var hash string
	if target.dep != nil {
		spec := target.dep.Spec.DeepCopy()
		spec.Replicas = &zero
		hash = namespaceMoveHash(spec)
	} else {
		spec := target.sts.Spec.DeepCopy()
		spec.Replicas = &zero
		hash = namespaceMoveHash(spec)
	}
	if hash != target.meta.Annotations[namespaceMoveSpecAnnotation] || target.replicas == nil ||
		(*target.replicas != 0 && *target.replicas != plan.Replicas) || target.meta.UID == source.meta.UID {
		return nil, fmt.Errorf("目标工作负载已变更")
	}
	return target, nil
}

func (c *Client) scaleNamespaceMaintenanceWorkload(ctx context.Context, workload *namespaceWorkload, replicas int32) error {
	return c.updateNamespaceMaintenanceWorkload(ctx, workload, replicas, "")
}

func (c *Client) updateNamespaceMaintenanceWorkload(ctx context.Context, workload *namespaceWorkload, replicas int32, image string) error {
	if *workload.replicas == replicas {
		return nil
	}
	spec := map[string]any{"replicas": replicas}
	if image != "" {
		containers := workload.template.DeepCopy().Spec.Containers
		containers[0].Image = image
		spec["template"] = map[string]any{"spec": map[string]any{"containers": containers}}
	}
	patch, err := json.Marshal(map[string]any{
		"metadata": map[string]string{"resourceVersion": workload.meta.ResourceVersion},
		"spec":     spec,
	})
	if err != nil {
		return err
	}
	if workload.dep != nil {
		_, err := c.Clientset.AppsV1().Deployments(workload.meta.Namespace).Patch(ctx, workload.meta.Name, types.MergePatchType, patch, metav1.PatchOptions{})
		return err
	}
	_, err = c.Clientset.AppsV1().StatefulSets(workload.meta.Namespace).Patch(ctx, workload.meta.Name, types.MergePatchType, patch, metav1.PatchOptions{})
	return err
}

func (c *Client) waitNamespaceMaintenanceStopped(ctx context.Context, workload *namespaceWorkload) error {
	return wait.PollUntilContextTimeout(ctx, time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		kind := "deployment"
		if workload.sts != nil {
			kind = "statefulset"
		}
		current, err := c.namespaceWorkload(ctx, workload.meta.Namespace, workload.meta.Name, kind)
		if err != nil {
			return false, err
		}
		if current.meta.UID != workload.meta.UID || current.specHash != workload.specHash || current.replicas == nil || *current.replicas != 0 {
			return false, fmt.Errorf("等待退出时工作负载已变更")
		}
		pods, err := c.namespaceWorkloadPods(ctx, current)
		return err == nil && len(pods) == 0 && current.observed >= current.meta.Generation, err
	})
}

func (c *Client) namespaceMaintenanceReady(ctx context.Context, move ApplicationNamespaceMove, workload *namespaceWorkload, plan *ApplicationNamespaceMaintenancePlan) (bool, error) {
	if *workload.replicas != plan.Replicas || workload.ready != plan.Replicas || workload.observed < workload.meta.Generation {
		return false, nil
	}
	pods, err := c.namespaceWorkloadPods(ctx, workload)
	if err != nil || len(pods) != int(plan.Replicas) {
		return false, err
	}
	ready := map[types.UID]bool{}
	sequence := strconv.FormatUint(uint64(move.SourceSequence), 10)
	for _, pod := range pods {
		if namespaceMovePodImage(pod, sequence, workload.template.Spec.Containers[0].Name) != plan.ImageID ||
			(plan.Node != "" && pod.Spec.NodeName != plan.Node) {
			return false, fmt.Errorf("Pod 镜像或所在节点与迁移预检不一致")
		}
		ready[pod.UID] = true
	}
	service, err := c.Clientset.CoreV1().Services(workload.meta.Namespace).Get(ctx, move.Name, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	slices, err := c.Clientset.DiscoveryV1().EndpointSlices(workload.meta.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set{discoveryv1.LabelServiceName: move.Name}.String(),
	})
	if err != nil {
		return false, err
	}
	for _, slice := range slices.Items {
		owner := metav1.GetControllerOf(&slice)
		if owner == nil || owner.Kind != "Service" || owner.UID != service.UID {
			continue
		}
		for _, endpoint := range slice.Endpoints {
			if endpoint.Conditions.Ready != nil && *endpoint.Conditions.Ready && endpoint.TargetRef != nil &&
				endpoint.TargetRef.Kind == "Pod" && endpoint.TargetRef.Namespace == workload.meta.Namespace {
				delete(ready, endpoint.TargetRef.UID)
			}
		}
	}
	return len(ready) == 0, nil
}
