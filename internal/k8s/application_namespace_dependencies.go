package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type namespaceMoveResources struct {
	service *corev1.Service
	configs []*corev1.ConfigMap
	secrets []*corev1.Secret
}

func namespaceMoveReferences(pod corev1.PodSpec) (configs, secrets []string) {
	configNames, secretNames := map[string]bool{}, map[string]bool{}
	for _, volume := range pod.Volumes {
		if volume.ConfigMap != nil {
			configNames[volume.ConfigMap.Name] = true
		}
		if volume.Secret != nil {
			secretNames[volume.Secret.SecretName] = true
		}
	}
	for _, container := range pod.Containers {
		for _, field := range container.EnvFrom {
			if field.ConfigMapRef != nil {
				configNames[field.ConfigMapRef.Name] = true
			}
			if field.SecretRef != nil {
				secretNames[field.SecretRef.Name] = true
			}
		}
		for _, field := range container.Env {
			if field.ValueFrom == nil {
				continue
			}
			if ref := field.ValueFrom.ConfigMapKeyRef; ref != nil {
				configNames[ref.Name] = true
			}
			if ref := field.ValueFrom.SecretKeyRef; ref != nil {
				secretNames[ref.Name] = true
			}
		}
	}
	for _, pull := range pod.ImagePullSecrets {
		secretNames[pull.Name] = true
	}
	return slices.Sorted(maps.Keys(configNames)), slices.Sorted(maps.Keys(secretNames))
}

func (c *Client) namespaceMoveDependencies(ctx context.Context, move ApplicationNamespaceMove, source *namespaceWorkload, plan *ApplicationNamespaceMaintenancePlan) (*namespaceMoveResources, string, error) {
	configs, secrets := c.Clientset.CoreV1().ConfigMaps(move.SourceNamespace), c.Clientset.CoreV1().Secrets(move.SourceNamespace)
	var resources namespaceMoveResources
	var versions []string
	for _, name := range plan.ConfigMaps {
		obj, err := configs.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, "", fmt.Errorf("无法读取源 ConfigMap %s", name)
		}
		resources.configs = append(resources.configs, obj)
		versions = append(versions, string(obj.UID), obj.ResourceVersion)
	}
	for _, name := range plan.Secrets {
		obj, err := secrets.Get(ctx, name, metav1.GetOptions{})
		if err != nil || (obj.Type != corev1.SecretTypeOpaque && obj.Type != corev1.SecretTypeDockerConfigJson && obj.Type != corev1.SecretTypeDockercfg && obj.Type != corev1.SecretTypeTLS) {
			return nil, "", fmt.Errorf("无法迁移源 Secret %s", name)
		}
		resources.secrets = append(resources.secrets, obj)
		// Certificate renewal can update a TLS Secret during staging.
		versions = append(versions, string(obj.UID))
		if obj.Type != corev1.SecretTypeTLS {
			versions = append(versions, obj.ResourceVersion)
		}
	}
	service, err := c.Clientset.CoreV1().Services(move.SourceNamespace).Get(ctx, move.Name, metav1.GetOptions{})
	if err != nil {
		return nil, "", err
	}
	if ensureApplicationManaged(service.Labels) != nil || service.Spec.Type != corev1.ServiceTypeClusterIP || len(service.Spec.Selector) == 0 ||
		service.Spec.PublishNotReadyAddresses || !labels.SelectorFromSet(service.Spec.Selector).Matches(labels.Set(source.template.Labels)) {
		return nil, "", fmt.Errorf("源 Service 需要是应用管理的 ClusterIP 服务，且 selector 匹配源 Pod")
	}
	resources.service = service
	return &resources, namespaceMoveHash(versions), nil
}

func (c *Client) planNamespaceMoveResources(ctx context.Context, move ApplicationNamespaceMove, source *namespaceWorkload, plan *ApplicationNamespaceMaintenancePlan) error {
	plan.ConfigMaps, plan.Secrets = namespaceMoveReferences(source.template.Spec)
	ingress, err := c.Clientset.NetworkingV1().Ingresses(move.SourceNamespace).Get(ctx, move.Name, metav1.GetOptions{})
	if err == nil {
		if ensureApplicationManaged(ingress.Labels) != nil || ingress.Spec.DefaultBackend != nil || len(ingress.OwnerReferences) != 0 {
			return fmt.Errorf("源 Ingress 包含需要单独迁移的路由")
		}
		for key := range ingress.Annotations {
			if key == "kubectl.kubernetes.io/last-applied-configuration" || key == "kubernetes.io/ingress.class" || key == applicationMoveAnnotation ||
				key == "traefik.ingress.kubernetes.io/router.entrypoints" || key == "traefik.ingress.kubernetes.io/router.tls" {
				continue
			}
			return fmt.Errorf("源 Ingress 注解 %s 需要单独验证", key)
		}
		if ingress.Labels["app.kubernetes.io/part-of"] != fmt.Sprintf("project-%d", move.SourceProjectID) {
			return fmt.Errorf("源 Ingress 包含需要单独迁移的路由")
		}
		for _, rule := range ingress.Spec.Rules {
			if rule.HTTP == nil {
				return fmt.Errorf("源 Ingress 路由无效")
			}
			for _, route := range rule.HTTP.Paths {
				if route.Backend.Service == nil || route.Backend.Service.Name != move.Name {
					return fmt.Errorf("源 Ingress 使用了其他服务")
				}
			}
		}
		for _, tls := range ingress.Spec.TLS {
			if tls.SecretName != "" && !slices.Contains(plan.Secrets, tls.SecretName) {
				plan.Secrets = append(plan.Secrets, tls.SecretName)
			}
		}
		// Persist the source identity and route definition so cutover can resume
		// after deletion of the original Ingress.
		snapshot := networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: ingress.Name, UID: ingress.UID,
			Labels: ingress.Labels, Annotations: ingress.Annotations}, Spec: ingress.Spec}
		delete(snapshot.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
		data, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		plan.Ingress = string(data)
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	resources, versions, err := c.namespaceMoveDependencies(ctx, move, source, plan)
	if err != nil {
		return err
	}
	plan.Service = namespaceMoveHash([]interface{}{resources.service.UID, resources.service.Spec})
	plan.Dependencies = versions
	return nil
}

func (c *Client) prepareNamespaceMoveResources(ctx context.Context, move ApplicationNamespaceMove, source *namespaceWorkload, plan *ApplicationNamespaceMaintenancePlan, marker string, create bool) error {
	resources, versions, err := c.namespaceMoveDependencies(ctx, move, source, plan)
	if err != nil {
		return err
	}
	if versions != plan.Dependencies || namespaceMoveHash([]interface{}{resources.service.UID, resources.service.Spec}) != plan.Service {
		return fmt.Errorf("源配置、Secret 或 Service 自预检后已变更")
	}
	meta := func(name string) metav1.ObjectMeta {
		return moveMetadata(metav1.ObjectMeta{Name: name, Labels: map[string]string{ManagedByLabel: ManagedByValue}},
			move.TargetNamespace, marker, fmt.Sprintf("project-%d", move.TargetProjectID), EnvironmentLabelValue(move.TargetEnvironmentID))
	}
	for _, obj := range resources.configs {
		target := c.Clientset.CoreV1().ConfigMaps(move.TargetNamespace)
		current, err := target.Get(ctx, obj.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) && create {
			current, err = target.Create(ctx, &corev1.ConfigMap{ObjectMeta: meta(obj.Name), Data: obj.Data, BinaryData: obj.BinaryData, Immutable: obj.Immutable}, metav1.CreateOptions{})
		}
		if err != nil || checkNamespaceMoveResource(current, err, marker) != nil ||
			!maps.Equal(current.Data, obj.Data) || !maps.EqualFunc(current.BinaryData, obj.BinaryData, slices.Equal[[]byte]) {
			return fmt.Errorf("目标 ConfigMap %s 缺失、已变更或同名占用", obj.Name)
		}
	}
	for _, obj := range resources.secrets {
		target := c.Clientset.CoreV1().Secrets(move.TargetNamespace)
		current, err := target.Get(ctx, obj.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) && create {
			current, err = target.Create(ctx, &corev1.Secret{ObjectMeta: meta(obj.Name), Type: obj.Type, Data: obj.Data, Immutable: obj.Immutable}, metav1.CreateOptions{})
		}
		if err != nil || (checkNamespaceMoveResource(current, err, marker) != nil && !c.reusableNamespaceMoveTLS(ctx, move, current)) ||
			current.Type != obj.Type || (obj.Type == corev1.SecretTypeTLS && !namespaceMoveTLSSecretReady(current)) ||
			(obj.Type != corev1.SecretTypeTLS && !maps.EqualFunc(current.Data, obj.Data, slices.Equal[[]byte])) {
			return fmt.Errorf("目标 Secret %s 缺失、已变更或同名占用", obj.Name)
		}
	}
	svc := resources.service.Spec.DeepCopy()
	svc.ClusterIPs, svc.IPFamilies, svc.ClusterIP = nil, nil, ""
	if resources.service.Spec.ClusterIP == corev1.ClusterIPNone {
		svc.ClusterIP = corev1.ClusterIPNone
	}
	for key := range svc.Selector {
		switch key {
		case "app.kubernetes.io/part-of", EnvironmentLabel:
			svc.Selector[key] = meta(move.Name).Labels[key]
		}
	}
	services := c.Clientset.CoreV1().Services(move.TargetNamespace)
	target, err := services.Get(ctx, move.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) && create {
		target, err = services.Create(ctx, &corev1.Service{ObjectMeta: meta(move.Name), Spec: *svc}, metav1.CreateOptions{})
	}
	if err != nil {
		return err
	}
	if checkNamespaceMoveResource(target, nil, marker) != nil || !maps.Equal(target.Spec.Selector, svc.Selector) ||
		!equality.Semantic.DeepEqual(target.Spec.Ports, svc.Ports) || target.Spec.Type != svc.Type || target.Spec.PublishNotReadyAddresses {
		return fmt.Errorf("目标 Service 已变更或同名占用")
	}
	return nil
}

func (c *Client) SwitchApplicationNamespaceMaintenanceRoutes(ctx context.Context, move ApplicationNamespaceMove, plan *ApplicationNamespaceMaintenancePlan, marker string, endpoints []model.ApplicationEndpoint, switchRoutes bool) error {
	if plan.Ingress == "" {
		return c.CheckApplicationNamespaceRoutes(ctx, move, endpoints)
	}
	var original networkingv1.Ingress
	if err := json.Unmarshal([]byte(plan.Ingress), &original); err != nil {
		return err
	}
	source := c.Clientset.NetworkingV1().Ingresses(move.SourceNamespace)
	target := c.Clientset.NetworkingV1().Ingresses(move.TargetNamespace)
	old, oldErr := source.Get(ctx, original.Name, metav1.GetOptions{})
	if oldErr != nil && !apierrors.IsNotFound(oldErr) {
		return oldErr
	}
	if oldErr == nil && (old.UID != original.UID || namespaceMoveHash(old.Spec) != namespaceMoveHash(original.Spec) || !sameNamespaceMoveIngressAnnotations(old.Annotations, original.Annotations)) {
		return fmt.Errorf("源 Ingress 自预检后已变更")
	}
	current, targetErr := target.Get(ctx, original.Name, metav1.GetOptions{})
	if targetErr != nil && !apierrors.IsNotFound(targetErr) {
		return targetErr
	}
	if targetErr == nil && (checkNamespaceMoveResource(current, nil, marker) != nil || !sameNamespaceMoveTargetIngressSpec(current.Spec, original.Spec) || !sameNamespaceMoveIngressAnnotations(current.Annotations, original.Annotations)) {
		return fmt.Errorf("目标 Ingress 已变更或同名占用")
	}
	if !switchRoutes {
		if oldErr != nil || targetErr == nil {
			return fmt.Errorf("入口已经变更，请检查源和目标 Ingress")
		}
		desired := original.DeepCopy()
		desired.ObjectMeta = moveMetadata(original.ObjectMeta, move.TargetNamespace, marker, fmt.Sprintf("project-%d", move.TargetProjectID), EnvironmentLabelValue(move.TargetEnvironmentID))
		return c.validateNamespaceMoveIngress(desired, endpoints)
	}
	if oldErr == nil {
		uid, version := old.UID, old.ResourceVersion
		if err := source.Delete(ctx, original.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &version}}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	if apierrors.IsNotFound(targetErr) {
		original.ObjectMeta = moveMetadata(original.ObjectMeta, move.TargetNamespace, marker, fmt.Sprintf("project-%d", move.TargetProjectID), EnvironmentLabelValue(move.TargetEnvironmentID))
		if _, err := target.Create(ctx, &original, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("创建目标 Ingress 失败，请重试切流: %w", err)
		}
	}
	return c.CheckApplicationNamespaceRoutes(ctx, move, endpoints)
}

func sameNamespaceMoveIngressAnnotations(current, expected map[string]string) bool {
	current = maps.Clone(current)
	delete(current, applicationMoveAnnotation)
	delete(current, "kubectl.kubernetes.io/last-applied-configuration")
	expected = maps.Clone(expected)
	delete(expected, applicationMoveAnnotation)
	return maps.Equal(current, expected)
}

// Kubernetes may fill in the default IngressClass when creating a route that
// was originally created without one. Check every explicitly configured field.
func sameNamespaceMoveTargetIngressSpec(current, original networkingv1.IngressSpec) bool {
	if original.IngressClassName == nil {
		current.IngressClassName = nil
	}
	return equality.Semantic.DeepEqual(current, original)
}
