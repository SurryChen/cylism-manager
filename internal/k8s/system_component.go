package k8s

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var helmChartConfigGVR = schema.GroupVersionResource{
	Group:    "helm.cattle.io",
	Version:  "v1",
	Resource: "helmchartconfigs",
}

// ControllerMode identifies the workload control plane detected from resources
// currently present in the cluster. It must not be inferred from the component
// name alone because K3s installations can replace or disable built-ins.
type ControllerMode string

const (
	HelmChartMode        ControllerMode = "helm_chart"
	StaticDeploymentMode ControllerMode = "static_deployment"
	EmbeddedMode         ControllerMode = "embedded"
	UnknownMode          ControllerMode = "unknown"
)

// SystemComponentDetection records the evidence that selected an adapter.
type SystemComponentDetection struct {
	Mode        ControllerMode
	Evidence    []string
	Deployment  *appsv1.Deployment
	Workload    *ComponentWorkload
	ChartReady  bool
	ChartFailed bool
	ChartStatus string
}

// ComponentWorkload is the common status shape used for Helm Deployments and
// DaemonSets. Helm charts are allowed to choose either workload kind.
type ComponentWorkload struct {
	Kind      string
	Name      string
	Desired   int32
	Ready     int32
	Available int32
}

const (
	traefikWebReadTimeoutArgument       = "--entryPoints.web.transport.respondingTimeouts.readTimeout="
	traefikWebSecureReadTimeoutArgument = "--entryPoints.websecure.transport.respondingTimeouts.readTimeout="
)

// TraefikReadTimeout reports an effective timeout only when both HTTP and HTTPS
// entrypoints are configured with the same expected value.
func TraefikReadTimeout(deployment *appsv1.Deployment, expected string) (string, bool) {
	if deployment == nil || expected == "" {
		return "", false
	}
	var args []string
	for index := range deployment.Spec.Template.Spec.Containers {
		container := &deployment.Spec.Template.Spec.Containers[index]
		if container.Name == "traefik" {
			args = container.Args
			break
		}
	}
	if args == nil && len(deployment.Spec.Template.Spec.Containers) > 0 {
		args = deployment.Spec.Template.Spec.Containers[0].Args
	}
	web, webSecure := "", ""
	for _, argument := range args {
		if strings.HasPrefix(argument, traefikWebReadTimeoutArgument) {
			web = strings.TrimPrefix(argument, traefikWebReadTimeoutArgument)
		}
		if strings.HasPrefix(argument, traefikWebSecureReadTimeoutArgument) {
			webSecure = strings.TrimPrefix(argument, traefikWebSecureReadTimeoutArgument)
		}
	}
	if web == "" || web != webSecure {
		return "", false
	}
	return web, web == expected
}

// StaticDeploymentConfig is the narrow set of workload fields owned by the
// platform for a static K3s Deployment.
type StaticDeploymentConfig struct {
	Replicas       int32
	MaxUnavailable intstr.IntOrString
	MaxSurge       intstr.IntOrString
	NodeName       string
}

// CoreDNSConfig is the narrow, platform-managed portion of the K3s CoreDNS
// Deployment. K3s currently ships CoreDNS as a static Deployment manifest, so
// HelmChartConfig values do not affect its rollout or placement.
type CoreDNSConfig = StaticDeploymentConfig

// DetectSystemComponent chooses an adapter from resources actually present in
// the cluster. HelmChart takes precedence over a same-name Deployment because
// helm-controller owns the rendered Deployment and would overwrite direct edits.
func (c *Client) DetectSystemComponent(ctx context.Context, namespace, name string) (SystemComponentDetection, error) {
	if c == nil || c.Clientset == nil {
		return SystemComponentDetection{}, fmt.Errorf("Kubernetes 客户端未初始化")
	}
	result := SystemComponentDetection{Mode: UnknownMode}
	if c.DynamicClient != nil {
		chart, err := c.GetHelmChart(ctx, namespace, name)
		if err != nil {
			return result, err
		}
		if chart != nil {
			result.Mode = HelmChartMode
			result.Evidence = []string{"发现匹配的 HelmChart " + namespace + "/" + name}
			result.ChartReady, result.ChartFailed, result.ChartStatus = helmChartCondition(chart)
			workloadNamespace, _, targetErr := unstructured.NestedString(chart.Object, "spec", "targetNamespace")
			if targetErr != nil || workloadNamespace == "" {
				workloadNamespace = namespace
			}
			workload, workloadErr := c.findHelmWorkload(ctx, workloadNamespace, name)
			if workloadErr != nil {
				return result, workloadErr
			}
			if workload != nil {
				result.Workload = workload
				result.Evidence = append(result.Evidence, "发现 Helm 工作负载 "+workloadNamespace+"/"+workload.Kind+"/"+workload.Name)
				if workload.Kind == "Deployment" {
					deployment, getErr := c.Clientset.AppsV1().Deployments(workloadNamespace).Get(ctx, workload.Name, metav1.GetOptions{})
					if getErr == nil {
						result.Deployment = deployment
					}
				}
			} else if result.ChartReady {
				result.Evidence = append(result.Evidence, "HelmChart 已就绪但未匹配到 Deployment 或 DaemonSet")
			} else if result.ChartFailed {
				result.Evidence = append(result.Evidence, "HelmChart 报告安装失败")
			}
			return result, nil
		}
		result.Evidence = append(result.Evidence, "未发现匹配的 HelmChart")
	} else {
		result.Evidence = append(result.Evidence, "HelmChart API 客户端不可用")
		return result, nil
	}

	deployment, err := c.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		result.Mode = StaticDeploymentMode
		result.Deployment = deployment
		result.Evidence = append(result.Evidence, "发现同名 Deployment "+namespace+"/"+name)
		return result, nil
	}
	if !apierrors.IsNotFound(err) {
		return result, fmt.Errorf("读取 Deployment %s/%s: %w", namespace, name, err)
	}
	result.Evidence = append(result.Evidence, "未发现同名 Deployment")

	if name == "servicelb" {
		daemonsets, listErr := c.Clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
		if listErr != nil {
			return result, fmt.Errorf("读取 ServiceLB DaemonSet: %w", listErr)
		}
		for _, daemonset := range daemonsets.Items {
			if len(daemonset.Name) >= len("svclb-") && daemonset.Name[:len("svclb-")] == "svclb-" {
				result.Mode = EmbeddedMode
				result.Evidence = append(result.Evidence, "发现 ServiceLB 创建的 DaemonSet "+daemonset.Name)
				return result, nil
			}
		}
	}
	result.Evidence = append(result.Evidence, "无法确认组件控制源")
	return result, nil
}

func (c *Client) findHelmWorkload(ctx context.Context, namespace, name string) (*ComponentWorkload, error) {
	deployments, err := c.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("读取 Helm Deployment: %w", err)
	}
	for _, deployment := range deployments.Items {
		if !matchesHelmWorkload(&deployment, name, namespace) {
			continue
		}
		return &ComponentWorkload{
			Kind:      "Deployment",
			Name:      deployment.Name,
			Desired:   desiredReplicas(deployment.Spec.Replicas),
			Ready:     deployment.Status.ReadyReplicas,
			Available: deployment.Status.AvailableReplicas,
		}, nil
	}
	daemonsets, err := c.Clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("读取 Helm DaemonSet: %w", err)
	}
	for _, daemonset := range daemonsets.Items {
		if !matchesHelmWorkload(&daemonset, name, namespace) {
			continue
		}
		return &ComponentWorkload{
			Kind:      "DaemonSet",
			Name:      daemonset.Name,
			Desired:   daemonset.Status.DesiredNumberScheduled,
			Ready:     daemonset.Status.NumberReady,
			Available: daemonset.Status.NumberAvailable,
		}, nil
	}
	return nil, nil
}

func matchesHelmWorkload(object metav1.Object, name, namespace string) bool {
	if object.GetName() == name || strings.HasPrefix(object.GetName(), name+"-") {
		return true
	}
	labels := object.GetLabels()
	for _, key := range []string{"app.kubernetes.io/instance", "app.kubernetes.io/name", "k8s-app"} {
		value := labels[key]
		if value == name || value == name+"-"+namespace {
			return true
		}
	}
	return false
}

func desiredReplicas(replicas *int32) int32 {
	if replicas == nil {
		return 0
	}
	return *replicas
}

func helmChartCondition(chart *unstructured.Unstructured) (ready, failed bool, status string) {
	conditions, found, err := unstructured.NestedSlice(chart.Object, "status", "conditions")
	if !found || err != nil {
		return false, false, ""
	}
	for _, raw := range conditions {
		condition, ok := raw.(map[string]interface{})
		if !ok || condition["type"] != "Ready" {
			continue
		}
		conditionStatus := fmt.Sprint(condition["status"])
		status = fmt.Sprint(condition["message"])
		if conditionStatus == "True" {
			return true, false, status
		}
		if conditionStatus == "False" {
			return false, true, status
		}
	}
	return false, false, status
}

// ApplyCoreDNSConfig updates only the CoreDNS settings managed by the
// platform. Changing the hostname selector updates the Pod template and lets
// the Deployment controller roll the replicas onto the selected node.
func (c *Client) ApplyCoreDNSConfig(ctx context.Context, config CoreDNSConfig) error {
	return c.ApplyStaticDeploymentConfig(ctx, "kube-system", "coredns", config)
}

// ApplyStaticDeploymentConfig updates only fields explicitly owned by the
// platform, preserving the static manifest's remaining pod template fields.
func (c *Client) ApplyStaticDeploymentConfig(ctx context.Context, namespace, name string, config StaticDeploymentConfig) error {
	if c == nil || c.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	if config.Replicas < 1 {
		return fmt.Errorf("%s 副本数必须至少为 1", name)
	}
	deployment, err := c.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("读取 Deployment %s/%s: %w", namespace, name, err)
	}
	deployment.Spec.Replicas = &config.Replicas
	deployment.Spec.Strategy = appsv1.DeploymentStrategy{
		Type: appsv1.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDeployment{
			MaxUnavailable: &config.MaxUnavailable,
			MaxSurge:       &config.MaxSurge,
		},
	}
	if deployment.Spec.Template.Spec.NodeSelector == nil {
		deployment.Spec.Template.Spec.NodeSelector = make(map[string]string)
	}
	if config.NodeName == "" {
		delete(deployment.Spec.Template.Spec.NodeSelector, corev1.LabelHostname)
	} else {
		deployment.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] = config.NodeName
	}
	if _, err := c.Clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("更新 Deployment %s/%s: %w", namespace, name, err)
	}
	return nil
}

// RestoreCoreDNSDefaults restores the fields modified by ApplyCoreDNSConfig to
// the values in the K3s bundled CoreDNS manifest.
func (c *Client) RestoreCoreDNSDefaults(ctx context.Context) error {
	return c.RestoreStaticDeploymentDefaults(ctx, "kube-system", "coredns")
}

// RestoreStaticDeploymentDefaults resets only the platform-owned fields. CoreDNS
// retains the defaults in the K3s bundled manifest; other Deployments use the
// Kubernetes Deployment defaults because their bundled defaults are not safely
// discoverable through the Kubernetes API.
func (c *Client) RestoreStaticDeploymentDefaults(ctx context.Context, namespace, name string) error {
	config := StaticDeploymentConfig{
		Replicas:       1,
		MaxUnavailable: intstr.FromString("25%"),
		MaxSurge:       intstr.FromString("25%"),
	}
	if name == "coredns" {
		config.MaxUnavailable = intstr.FromInt(1)
	}
	return c.ApplyStaticDeploymentConfig(ctx, namespace, name, config)
}

// GetHelmChart returns a matching HelmChart, or nil when K3s does not expose
// one for the component.
func (c *Client) GetHelmChart(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	if c == nil || c.DynamicClient == nil {
		return nil, fmt.Errorf("Kubernetes 动态客户端未初始化")
	}
	obj, err := c.DynamicClient.Resource(helmChartGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 HelmChart %s/%s: %w", namespace, name, err)
	}
	return obj, nil
}

// GetHelmChartConfig 返回 HelmChartConfig CRD，不存在时返回 nil。
func (c *Client) GetHelmChartConfig(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	if c == nil || c.DynamicClient == nil {
		return nil, fmt.Errorf("Kubernetes 动态客户端未初始化")
	}
	obj, err := c.DynamicClient.Resource(helmChartConfigGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 HelmChartConfig %s/%s: %w", namespace, name, err)
	}
	return obj, nil
}

// ApplyHelmChartConfig 创建或更新 HelmChartConfig CRD；K3s helm-controller
// 会按 CRD 重新渲染内置 chart，因此配置在升级后依然保留。
func (c *Client) ApplyHelmChartConfig(ctx context.Context, namespace, name, valuesContent string) error {
	if c == nil || c.DynamicClient == nil {
		return fmt.Errorf("Kubernetes 动态客户端未初始化")
	}
	desired := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "helm.cattle.io/v1",
		"kind":       "HelmChartConfig",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels":    map[string]interface{}{ManagedByLabel: ManagedByValue},
		},
		"spec": map[string]any{"valuesContent": valuesContent},
	}}
	current, err := c.GetHelmChartConfig(ctx, namespace, name)
	if err != nil {
		return err
	}
	resource := c.DynamicClient.Resource(helmChartConfigGVR).Namespace(namespace)
	if current == nil {
		if _, err := resource.Create(ctx, desired, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("创建 HelmChartConfig %s/%s: %w", namespace, name, err)
		}
		return nil
	}
	desired.SetResourceVersion(current.GetResourceVersion())
	if _, err := resource.Update(ctx, desired, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("更新 HelmChartConfig %s/%s: %w", namespace, name, err)
	}
	return nil
}

// DeleteHelmChartConfig 删除 CRD，K3s 恢复内置 chart 默认值。
func (c *Client) DeleteHelmChartConfig(ctx context.Context, namespace, name string) error {
	if c == nil || c.DynamicClient == nil {
		return fmt.Errorf("Kubernetes 动态客户端未初始化")
	}
	err := c.DynamicClient.Resource(helmChartConfigGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("删除 HelmChartConfig %s/%s: %w", namespace, name, err)
	}
	return nil
}

// SystemComponentKubernetesAdapter exposes only the Kubernetes operations
// required by the system-component Service. It keeps client wiring out of
// HTTP handlers and makes the boundary replaceable in tests.
type SystemComponentKubernetesAdapter struct{ Client *Client }

func (a SystemComponentKubernetesAdapter) Available() bool {
	return a.Client != nil && a.Client.Clientset != nil
}

func (a SystemComponentKubernetesAdapter) GetDeployment(ctx context.Context, ns, name string) (*appsv1.Deployment, error) {
	return a.Client.Clientset.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
}

func (a SystemComponentKubernetesAdapter) ListNodes(ctx context.Context) ([]corev1.Node, error) {
	list, err := a.Client.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (a SystemComponentKubernetesAdapter) ListPods(ctx context.Context, ns, selector string) ([]corev1.Pod, error) {
	list, err := a.Client.Clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (a SystemComponentKubernetesAdapter) HasDynamicClient() bool {
	return a.Client != nil && a.Client.DynamicClient != nil
}

func (a SystemComponentKubernetesAdapter) DetectSystemComponent(ctx context.Context, ns, name string) (SystemComponentDetection, error) {
	return a.Client.DetectSystemComponent(ctx, ns, name)
}

func (a SystemComponentKubernetesAdapter) GetNodeInfo(ctx context.Context, name string) (*NodeInfo, error) {
	return a.Client.GetNodeInfoContext(ctx, name)
}

func (a SystemComponentKubernetesAdapter) ApplyHelmChartConfig(ctx context.Context, ns, name, values string) error {
	return a.Client.ApplyHelmChartConfig(ctx, ns, name, values)
}

func (a SystemComponentKubernetesAdapter) DeleteHelmChartConfig(ctx context.Context, ns, name string) error {
	return a.Client.DeleteHelmChartConfig(ctx, ns, name)
}

func (a SystemComponentKubernetesAdapter) ApplyStaticDeploymentConfig(ctx context.Context, ns, name string, cfg StaticDeploymentConfig) error {
	return a.Client.ApplyStaticDeploymentConfig(ctx, ns, name, cfg)
}

func (a SystemComponentKubernetesAdapter) RestoreStaticDeploymentDefaults(ctx context.Context, ns, name string) error {
	return a.Client.RestoreStaticDeploymentDefaults(ctx, ns, name)
}
