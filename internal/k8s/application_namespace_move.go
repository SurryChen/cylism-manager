package k8s

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const applicationMoveAnnotation = "cylism.io/namespace-move"

type ApplicationNamespaceMove struct {
	ApplicationID, SourceSequence, SourceProjectID, TargetProjectID, TargetEnvironmentID uint
	Name, SourceNamespace, TargetNamespace                                               string
}

type ApplicationNamespaceMoveStatus struct {
	Prepared bool `json:"prepared"`
	Ready    bool `json:"ready"`
}

func (c *Client) StageApplicationNamespace(ctx context.Context, move ApplicationNamespaceMove, prepare bool) (ApplicationNamespaceMoveStatus, error) {
	var status ApplicationNamespaceMoveStatus
	source, err := c.Clientset.AppsV1().Deployments(move.SourceNamespace).Get(ctx, move.Name, metav1.GetOptions{})
	if err != nil {
		return status, fmt.Errorf("读取源 Deployment: %w", err)
	}
	sequence := strconv.FormatUint(uint64(move.SourceSequence), 10)
	if err := ensureApplicationManaged(source.Labels); err != nil {
		return status, err
	}
	if source.Labels["app.kubernetes.io/part-of"] != fmt.Sprintf("project-%d", move.SourceProjectID) {
		return status, fmt.Errorf("源 Deployment 的项目标签与应用不一致")
	}
	if source.Spec.Template.Labels["cylism.io/release"] != sequence {
		return status, fmt.Errorf("源 Deployment 运行的版本与成功发布记录不一致")
	}
	if err := validateNamespaceMoveDeployment(source); err != nil {
		return status, err
	}
	sourceService, err := c.Clientset.CoreV1().Services(move.SourceNamespace).Get(ctx, move.Name, metav1.GetOptions{})
	if err != nil {
		return status, fmt.Errorf("读取源 Service: %w", err)
	}
	if err := ensureApplicationManaged(sourceService.Labels); err != nil {
		return status, err
	}
	if sourceService.Spec.Type != corev1.ServiceTypeClusterIP || len(sourceService.Spec.Selector) == 0 || sourceService.Spec.ClusterIP == corev1.ClusterIPNone {
		return status, fmt.Errorf("只支持使用普通 ClusterIP Service 的 Deployment")
	}
	marker := fmt.Sprintf("%d/%s/%d/%s/%s", move.ApplicationID, source.UID, source.Generation, sourceService.UID, sourceService.ResourceVersion)
	destLabel := fmt.Sprintf("project-%d", move.TargetProjectID)
	envLabel := fmt.Sprintf("environment-%d", move.TargetEnvironmentID)
	deployment := source.DeepCopy()
	deployment.ObjectMeta = moveMetadata(source.ObjectMeta, move.TargetNamespace, marker, destLabel, envLabel)
	deployment.Status = appsv1.DeploymentStatus{}
	deployment.Spec.Template.Labels["app.kubernetes.io/part-of"] = destLabel
	deployment.Spec.Template.Labels["cylism.io/environment"] = envLabel
	service := &corev1.Service{
		ObjectMeta: moveMetadata(sourceService.ObjectMeta, move.TargetNamespace, marker, destLabel, envLabel),
		Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, Ports: sourceService.Spec.Ports,
			Selector: sourceService.Spec.Selector, SessionAffinity: sourceService.Spec.SessionAffinity, SessionAffinityConfig: sourceService.Spec.SessionAffinityConfig},
	}
	service.Spec.Selector = make(map[string]string, len(sourceService.Spec.Selector))
	for key := range sourceService.Spec.Selector {
		if value, ok := deployment.Spec.Template.Labels[key]; ok && source.Spec.Template.Labels[key] == sourceService.Spec.Selector[key] {
			service.Spec.Selector[key] = value
		} else {
			return status, fmt.Errorf("源 Service selector 与当前 Deployment 不匹配")
		}
	}
	services := c.Clientset.CoreV1().Services(move.TargetNamespace)
	deployments := c.Clientset.AppsV1().Deployments(move.TargetNamespace)
	targetService, svcErr := services.Get(ctx, move.Name, metav1.GetOptions{})
	target, depErr := deployments.Get(ctx, move.Name, metav1.GetOptions{})
	if err := checkNamespaceMoveResource(targetService, svcErr, marker); err != nil {
		return status, fmt.Errorf("目标 Service: %w", err)
	}
	if err := checkNamespaceMoveResource(target, depErr, marker); err != nil {
		return status, fmt.Errorf("目标 Deployment: %w", err)
	}
	if prepare {
		if apierrors.IsNotFound(svcErr) {
			if targetService, svcErr = services.Create(ctx, service, metav1.CreateOptions{}); svcErr != nil {
				return status, fmt.Errorf("创建目标 Service: %w", svcErr)
			}
		}
		if apierrors.IsNotFound(depErr) {
			if target, depErr = deployments.Create(ctx, deployment, metav1.CreateOptions{}); depErr != nil {
				return status, fmt.Errorf("创建目标 Deployment: %w", depErr)
			}
		}
	}
	status.Prepared = svcErr == nil && depErr == nil
	if status.Prepared && (len(target.Spec.Template.Spec.Containers) == 0 || len(targetService.Spec.Selector) == 0 ||
		target.Spec.Template.Labels["cylism.io/release"] != sequence ||
		target.Spec.Template.Spec.Containers[0].Image != deployment.Spec.Template.Spec.Containers[0].Image ||
		!labels.SelectorFromSet(targetService.Spec.Selector).Matches(labels.Set(target.Spec.Template.Labels))) {
		return status, fmt.Errorf("目标 Deployment 或 Service 已变更，请重新核对")
	}
	if !status.Prepared || !namespaceMoveDeploymentReady(target) {
		return status, nil
	}
	pods := c.Clientset.CoreV1().Pods
	selector := metav1.ListOptions{LabelSelector: labels.Set(sourceService.Spec.Selector).String()}
	sourcePods, err := pods(move.SourceNamespace).List(ctx, selector)
	if err != nil {
		return status, fmt.Errorf("读取源 Pod 镜像: %w", err)
	}
	containerName := source.Spec.Template.Spec.Containers[0].Name
	sourceImages := make(map[string]bool)
	for _, pod := range sourcePods.Items {
		if image := namespaceMovePodImage(pod, sequence, containerName); image != "" {
			sourceImages[image] = true
		}
	}
	if len(sourceImages) != 1 {
		return status, fmt.Errorf("无法确认源 Pod 使用同一镜像")
	}
	slices, err := c.Clientset.DiscoveryV1().EndpointSlices(move.TargetNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set{discoveryv1.LabelServiceName: targetService.Name}.String(),
	})
	if err != nil {
		return status, fmt.Errorf("读取目标 Service 就绪端点: %w", err)
	}
	readyPods := map[string]bool{}
	for _, slice := range slices.Items {
		owner := metav1.GetControllerOf(&slice)
		if owner == nil || owner.Kind != "Service" || owner.UID != targetService.UID {
			continue
		}
		for _, endpoint := range slice.Endpoints {
			if endpoint.Conditions.Ready != nil && *endpoint.Conditions.Ready && endpoint.TargetRef != nil &&
				endpoint.TargetRef.Kind == "Pod" && endpoint.TargetRef.Namespace == move.TargetNamespace {
				pod, err := pods(move.TargetNamespace).Get(ctx, endpoint.TargetRef.Name, metav1.GetOptions{})
				if err != nil {
					return status, fmt.Errorf("读取目标 Pod 镜像: %w", err)
				}
				if !sourceImages[namespaceMovePodImage(*pod, sequence, containerName)] {
					return status, fmt.Errorf("目标 Pod 镜像与源不一致，请核对镜像标签")
				}
				readyPods[pod.Name] = true
			}
		}
	}
	status.Ready = len(readyPods) >= int(*target.Spec.Replicas)
	return status, nil
}

func namespaceMovePodImage(pod corev1.Pod, sequence, name string) string {
	if pod.Labels["cylism.io/release"] != sequence || pod.DeletionTimestamp != nil {
		return ""
	}
	for _, container := range pod.Status.ContainerStatuses {
		if container.Name == name && container.Ready {
			return container.ImageID
		}
	}
	return ""
}

// CheckApplicationNamespaceRoutes verifies the operator's cutover without
// publishing duplicate routes or changing DNS.
func (c *Client) CheckApplicationNamespaceRoutes(ctx context.Context, move ApplicationNamespaceMove, endpoints []model.ApplicationEndpoint) error {
	_, err := c.Clientset.NetworkingV1().Ingresses(move.SourceNamespace).Get(ctx, move.Name, metav1.GetOptions{})
	if err == nil {
		return fmt.Errorf("源命名空间仍存在同名 Ingress，请在验证入口切换后移除旧路由")
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	for _, endpoint := range endpoints {
		if endpoint.IngressMode == "metadata" || (endpoint.Exposure != "" && endpoint.Exposure != "public") {
			continue
		}
		ingress, err := c.Clientset.NetworkingV1().Ingresses(move.TargetNamespace).Get(ctx, move.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("读取目标 Ingress: %w", err)
		}
		if err := ensureApplicationManaged(ingress.Labels); err != nil {
			return err
		}
		path := endpoint.Path
		if path == "" {
			path = "/"
		}
		found := false
		for _, rule := range ingress.Spec.Rules {
			if rule.Host != endpoint.Domain || rule.HTTP == nil {
				continue
			}
			for _, route := range rule.HTTP.Paths {
				if route.Path == path && route.Backend.Service != nil && route.Backend.Service.Name == move.Name &&
					route.Backend.Service.Port.Number == endpoint.ServicePort {
					found = true
				}
			}
		}
		if !found {
			return fmt.Errorf("目标 Ingress 缺少 %s%s 的原路由", endpoint.Domain, path)
		}
		if endpoint.TLSEnabled {
			tlsReady := false
			for _, tls := range ingress.Spec.TLS {
				for _, host := range tls.Hosts {
					if strings.EqualFold(host, endpoint.Domain) && tls.SecretName == endpoint.TLSSecretName && tls.SecretName != "" {
						tlsReady = true
					}
				}
			}
			if !tlsReady {
				return fmt.Errorf("目标 Ingress 缺少 %s 的 TLS 配置", endpoint.Domain)
			}
		}
	}
	return nil
}

func checkNamespaceMoveResource(object metav1.Object, err error, marker string) error {
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if object.GetAnnotations()[applicationMoveAnnotation] != marker || object.GetDeletionTimestamp() != nil {
		return fmt.Errorf("已有同名资源，或准备后源版本已变更")
	}
	return ensureApplicationManaged(object.GetLabels())
}

func moveMetadata(source metav1.ObjectMeta, namespace, marker, project, environment string) metav1.ObjectMeta {
	meta := metav1.ObjectMeta{Name: source.Name, Namespace: namespace, Labels: source.Labels, Annotations: source.Annotations}
	if meta.Annotations == nil {
		meta.Annotations = map[string]string{}
	}
	delete(meta.Annotations, "deployment.kubernetes.io/revision")
	delete(meta.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
	meta.Annotations[applicationMoveAnnotation] = marker
	meta.Labels["app.kubernetes.io/part-of"] = project
	meta.Labels["cylism.io/environment"] = environment
	return meta
}

func validateNamespaceMoveDeployment(deployment *appsv1.Deployment) error {
	pod := deployment.Spec.Template.Spec
	if pod.HostNetwork || pod.HostPID || pod.HostIPC || deployment.Spec.Paused || len(pod.Containers) != 1 ||
		(pod.ServiceAccountName != "" && pod.ServiceAccountName != "default") || len(pod.InitContainers) != 0 {
		return fmt.Errorf("源 Deployment 的运行配置不支持并行启动")
	}
	for _, volume := range pod.Volumes {
		if volume.PersistentVolumeClaim != nil || volume.Ephemeral != nil || volume.HostPath != nil || volume.CSI != nil {
			return fmt.Errorf("应用使用需要单独迁移的数据卷")
		}
	}
	for _, container := range append(append([]corev1.Container{}, pod.Containers...), pod.InitContainers...) {
		if container.SecurityContext != nil && container.SecurityContext.Privileged != nil && *container.SecurityContext.Privileged {
			return fmt.Errorf("应用使用特权容器，需要单独迁移")
		}
		for _, port := range container.Ports {
			if port.HostPort != 0 {
				return fmt.Errorf("应用占用宿主机端口，无法安全并行启动")
			}
		}
	}
	for _, container := range pod.Containers {
		if container.ReadinessProbe == nil {
			return fmt.Errorf("请先为应用配置有效的就绪探针")
		}
	}
	if !namespaceMoveDeploymentReady(deployment) {
		return fmt.Errorf("源 Deployment 尚未就绪")
	}
	return nil
}

func namespaceMoveDeploymentReady(deployment *appsv1.Deployment) bool {
	return deployment.Spec.Replicas != nil && *deployment.Spec.Replicas > 0 &&
		deployment.Status.ObservedGeneration >= deployment.Generation &&
		deployment.Status.UpdatedReplicas == *deployment.Spec.Replicas &&
		deployment.Status.AvailableReplicas == *deployment.Spec.Replicas
}
