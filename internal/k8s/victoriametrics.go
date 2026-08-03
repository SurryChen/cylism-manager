package k8s

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	VictoriaMetricsStateNotInstalled = "not_installed"
	VictoriaMetricsStateInstalling   = "installing"
	VictoriaMetricsStateReady        = "ready"
	VictoriaMetricsStateDegraded     = "degraded"
	VictoriaMetricsStateUnavailable  = "unavailable"

	victoriaMetricsNamespace = "monitoring"
	victoriaMetricsName      = "cylism-victoria-metrics"
	victoriaMetricsImage     = "victoriametrics/victoria-metrics:v1.115.0"
	nodeExporterName         = "cylism-node-exporter"
	nodeExporterImage        = "quay.io/prometheus/node-exporter:v1.8.2"
)

// VictoriaMetricsConfig controls the single-node, hostPath-backed metrics store.
type VictoriaMetricsConfig struct {
	NodeName      string `json:"node_name"`
	DataPath      string `json:"data_path"`
	RetentionDays int    `json:"retention_days"`
}

// VictoriaMetricsStatus describes the managed instance and its current Kubernetes state.
type VictoriaMetricsStatus struct {
	State               string `json:"state"`
	Message             string `json:"message"`
	NodeName            string `json:"node_name,omitempty"`
	DataPath            string `json:"data_path,omitempty"`
	RetentionDays       int    `json:"retention_days,omitempty"`
	ReadyReplicas       int32  `json:"ready_replicas"`
	NodeExporterReady   int32  `json:"node_exporter_ready"`
	NodeExporterDesired int32  `json:"node_exporter_desired"`
	Endpoint            string `json:"endpoint,omitempty"`
}

func (c *Client) VictoriaMetricsStatus() *VictoriaMetricsStatus {
	status := &VictoriaMetricsStatus{State: VictoriaMetricsStateUnavailable, Message: "Kubernetes 客户端未初始化"}
	if c == nil || c.Clientset == nil {
		return status
	}

	deployment, err := c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Get(c.Ctx(), victoriaMetricsName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		status.State = VictoriaMetricsStateNotInstalled
		status.Message = "尚未安装 VictoriaMetrics"
		return status
	}
	if err != nil {
		status.State = VictoriaMetricsStateDegraded
		status.Message = fmt.Sprintf("读取 VictoriaMetrics 状态失败: %v", err)
		return status
	}

	config, _ := victoriaMetricsConfigFromDeployment(deployment)
	status.NodeName = config.NodeName
	status.DataPath = config.DataPath
	status.RetentionDays = config.RetentionDays
	status.ReadyReplicas = deployment.Status.AvailableReplicas
	status.Endpoint = VictoriaMetricsServiceURL()
	if deployment.Status.AvailableReplicas == 0 {
		status.State = VictoriaMetricsStateInstalling
		status.Message = deploymentStatusMessage(deployment)
		for _, condition := range deployment.Status.Conditions {
			if condition.Type == appsv1.DeploymentReplicaFailure && condition.Status == corev1.ConditionTrue {
				status.State = VictoriaMetricsStateDegraded
				status.Message = condition.Message
				break
			}
		}
		return status
	}

	nodeExporter, err := c.Clientset.AppsV1().DaemonSets(victoriaMetricsNamespace).Get(c.Ctx(), nodeExporterName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		status.State = VictoriaMetricsStateDegraded
		status.Message = "VictoriaMetrics 已启动，但 node-exporter 未安装"
		return status
	}
	if err != nil {
		status.State = VictoriaMetricsStateDegraded
		status.Message = fmt.Sprintf("读取 node-exporter 状态失败: %v", err)
		return status
	}
	status.NodeExporterReady = nodeExporter.Status.NumberAvailable
	status.NodeExporterDesired = nodeExporter.Status.DesiredNumberScheduled
	if status.NodeExporterDesired == 0 || status.NodeExporterReady < status.NodeExporterDesired {
		status.State = VictoriaMetricsStateInstalling
		status.Message = "VictoriaMetrics 已启动，等待 node-exporter 在全部节点就绪"
		return status
	}
	status.State = VictoriaMetricsStateReady
	status.Message = "VictoriaMetrics 已就绪，正在采集节点、磁盘和容器指标"
	return status
}

// InstallVictoriaMetrics creates or updates the managed metrics store. Data location
// is immutable after the first install because hostPath is local to one Kubernetes node.
func (c *Client) InstallVictoriaMetrics(config VictoriaMetricsConfig) (*VictoriaMetricsStatus, error) {
	if c == nil || c.Clientset == nil {
		return c.VictoriaMetricsStatus(), fmt.Errorf("Kubernetes 客户端未初始化")
	}
	if err := validateVictoriaMetricsConfig(config); err != nil {
		return c.VictoriaMetricsStatus(), err
	}
	node, err := c.Clientset.CoreV1().Nodes().Get(c.Ctx(), config.NodeName, metav1.GetOptions{})
	if err != nil {
		return c.VictoriaMetricsStatus(), fmt.Errorf("读取数据节点失败: %w", err)
	}
	if !nodeReady(node) {
		return c.VictoriaMetricsStatus(), fmt.Errorf("数据节点 %s 未就绪", config.NodeName)
	}

	existing, err := c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Get(c.Ctx(), victoriaMetricsName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return c.VictoriaMetricsStatus(), fmt.Errorf("读取现有 VictoriaMetrics 配置失败: %w", err)
	}
	if err == nil {
		old, _ := victoriaMetricsConfigFromDeployment(existing)
		if old.NodeName != "" && (old.NodeName != config.NodeName || old.DataPath != config.DataPath) {
			return c.VictoriaMetricsStatus(), fmt.Errorf("已安装实例的数据节点或目录不能修改；hostPath 数据仍保留在 %s:%s", old.NodeName, old.DataPath)
		}
	}

	if err := ensureMonitoringNamespace(c); err != nil {
		return c.VictoriaMetricsStatus(), err
	}
	if err := ensureVictoriaMetricsAccess(c); err != nil {
		return c.VictoriaMetricsStatus(), err
	}
	if err := upsertVictoriaMetricsConfig(c, config); err != nil {
		return c.VictoriaMetricsStatus(), err
	}
	if err := upsertVictoriaMetricsService(c); err != nil {
		return c.VictoriaMetricsStatus(), err
	}
	if err := upsertVictoriaMetricsDeployment(c, config); err != nil {
		return c.VictoriaMetricsStatus(), err
	}
	if err := upsertNodeExporter(c); err != nil {
		return c.VictoriaMetricsStatus(), err
	}

	status := c.VictoriaMetricsStatus()
	if status.State == VictoriaMetricsStateNotInstalled {
		return status, fmt.Errorf("创建 VictoriaMetrics Deployment 失败")
	}
	return status, nil
}

// UninstallVictoriaMetrics removes only platform-managed Kubernetes resources. The
// hostPath directory is intentionally retained so metrics data is not deleted by mistake.
func (c *Client) UninstallVictoriaMetrics() error {
	if c == nil || c.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	for _, remove := range []func() error{
		func() error {
			return c.Clientset.AppsV1().DaemonSets(victoriaMetricsNamespace).Delete(c.Ctx(), nodeExporterName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Delete(c.Ctx(), victoriaMetricsName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().Services(victoriaMetricsNamespace).Delete(c.Ctx(), victoriaMetricsName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().ConfigMaps(victoriaMetricsNamespace).Delete(c.Ctx(), victoriaMetricsName+"-scrape", metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.RbacV1().ClusterRoleBindings().Delete(c.Ctx(), victoriaMetricsName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.RbacV1().ClusterRoles().Delete(c.Ctx(), victoriaMetricsName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().ServiceAccounts(victoriaMetricsNamespace).Delete(c.Ctx(), victoriaMetricsName, metav1.DeleteOptions{})
		},
	} {
		if err := remove(); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("卸载 VictoriaMetrics 失败: %w", err)
		}
	}
	return nil
}

func validateVictoriaMetricsConfig(config VictoriaMetricsConfig) error {
	config.NodeName = strings.TrimSpace(config.NodeName)
	if config.NodeName == "" {
		return fmt.Errorf("请选择 VictoriaMetrics 数据节点")
	}
	if config.DataPath == "" || !filepath.IsAbs(config.DataPath) || filepath.Clean(config.DataPath) != config.DataPath {
		return fmt.Errorf("数据目录必须是规范的绝对路径")
	}
	if config.DataPath == "/" || !strings.HasPrefix(config.DataPath, "/data/") {
		return fmt.Errorf("数据目录仅允许使用 /data/ 下的独立目录")
	}
	if config.RetentionDays < 1 || config.RetentionDays > 365 {
		return fmt.Errorf("指标保留天数应在 1 到 365 天之间")
	}
	return nil
}

func ensureMonitoringNamespace(c *Client) error {
	_, err := c.Clientset.CoreV1().Namespaces().Get(c.Ctx(), victoriaMetricsNamespace, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("读取 monitoring 命名空间失败: %w", err)
	}
	_, err = c.Clientset.CoreV1().Namespaces().Create(c.Ctx(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsNamespace, Labels: victoriaMetricsLabels()}}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("创建 monitoring 命名空间失败: %w", err)
	}
	return nil
}

func ensureVictoriaMetricsAccess(c *Client) error {
	serviceAccount := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsName, Namespace: victoriaMetricsNamespace, Labels: victoriaMetricsLabels()}}
	if _, err := c.Clientset.CoreV1().ServiceAccounts(victoriaMetricsNamespace).Create(c.Ctx(), serviceAccount, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("创建 VictoriaMetrics 服务账号失败: %w", err)
	}
	role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsName, Labels: victoriaMetricsLabels()}, Rules: []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"nodes", "pods"}, Verbs: []string{"get", "list", "watch"}},
		{APIGroups: []string{""}, Resources: []string{"nodes/proxy"}, Verbs: []string{"get"}},
	}}
	if err := createOrUpdateClusterRole(c, role); err != nil {
		return err
	}
	binding := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsName, Labels: victoriaMetricsLabels()}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: victoriaMetricsName, Namespace: victoriaMetricsNamespace}}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: victoriaMetricsName}}
	if err := createOrUpdateClusterRoleBinding(c, binding); err != nil {
		return err
	}
	return nil
}

func upsertVictoriaMetricsConfig(c *Client, config VictoriaMetricsConfig) error {
	resource := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsName + "-scrape", Namespace: victoriaMetricsNamespace, Labels: victoriaMetricsLabels()}, Data: map[string]string{"scrape.yml": victoriaMetricsScrapeConfig()}}
	return createOrUpdateConfigMap(c, resource)
}

func upsertVictoriaMetricsService(c *Client) error {
	resource := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsName, Namespace: victoriaMetricsNamespace, Labels: victoriaMetricsLabels()}, Spec: corev1.ServiceSpec{Selector: victoriaMetricsLabels(), Ports: []corev1.ServicePort{{Name: "http", Port: 8428, TargetPort: intstr.FromInt(8428)}}}}
	current, err := c.Clientset.CoreV1().Services(victoriaMetricsNamespace).Get(c.Ctx(), victoriaMetricsName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.Clientset.CoreV1().Services(victoriaMetricsNamespace).Create(c.Ctx(), resource, metav1.CreateOptions{})
	} else if err == nil {
		resource.ResourceVersion = current.ResourceVersion
		resource.Spec.ClusterIP = current.Spec.ClusterIP
		resource.Spec.ClusterIPs = current.Spec.ClusterIPs
		resource.Spec.IPFamilies = current.Spec.IPFamilies
		resource.Spec.IPFamilyPolicy = current.Spec.IPFamilyPolicy
		_, err = c.Clientset.CoreV1().Services(victoriaMetricsNamespace).Update(c.Ctx(), resource, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("创建 VictoriaMetrics Service 失败: %w", err)
	}
	return nil
}

func upsertVictoriaMetricsDeployment(c *Client, config VictoriaMetricsConfig) error {
	replicas := int32(1)
	hostPathType := corev1.HostPathDirectoryOrCreate
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsName, Namespace: victoriaMetricsNamespace, Labels: victoriaMetricsLabels()}, Spec: appsv1.DeploymentSpec{
		Replicas: &replicas,
		Selector: &metav1.LabelSelector{MatchLabels: victoriaMetricsLabels()},
		Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: victoriaMetricsLabels(), Annotations: map[string]string{"cylism.io/scrape-config-hash": victoriaMetricsScrapeConfigHash()}}, Spec: corev1.PodSpec{
			NodeSelector:       map[string]string{corev1.LabelHostname: config.NodeName},
			ServiceAccountName: victoriaMetricsName,
			Volumes: []corev1.Volume{
				{Name: "storage", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: config.DataPath, Type: &hostPathType}}},
				{Name: "scrape-config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: victoriaMetricsName + "-scrape"}}}},
			},
			Containers: []corev1.Container{{
				Name:           "victoria-metrics",
				Image:          victoriaMetricsImage,
				Args:           []string{"-storageDataPath=/storage", "-retentionPeriod=" + strconv.Itoa(config.RetentionDays) + "d", "-promscrape.config=/etc/victoriametrics/scrape.yml"},
				Ports:          []corev1.ContainerPort{{Name: "http", ContainerPort: 8428}},
				Resources:      corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("256Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("1Gi")}},
				VolumeMounts:   []corev1.VolumeMount{{Name: "storage", MountPath: "/storage"}, {Name: "scrape-config", MountPath: "/etc/victoriametrics", ReadOnly: true}},
				ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt(8428)}}, InitialDelaySeconds: 5, PeriodSeconds: 10},
				LivenessProbe:  &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt(8428)}}, InitialDelaySeconds: 15, PeriodSeconds: 15},
			}},
		}},
	}}
	current, err := c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Get(c.Ctx(), victoriaMetricsName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Create(c.Ctx(), deployment, metav1.CreateOptions{})
	} else if err == nil {
		deployment.ResourceVersion = current.ResourceVersion
		_, err = c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Update(c.Ctx(), deployment, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("创建 VictoriaMetrics Deployment 失败: %w", err)
	}
	return nil
}

func upsertNodeExporter(c *Client) error {
	hostRootType := corev1.HostPathDirectory
	daemonSet := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: nodeExporterName, Namespace: victoriaMetricsNamespace, Labels: nodeExporterLabels()}, Spec: appsv1.DaemonSetSpec{
		Selector: &metav1.LabelSelector{MatchLabels: nodeExporterLabels()},
		Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: nodeExporterLabels()}, Spec: corev1.PodSpec{
			HostNetwork: true,
			Tolerations: []corev1.Toleration{{Operator: corev1.TolerationOpExists}},
			Volumes:     []corev1.Volume{{Name: "host-root", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/", Type: &hostRootType}}}},
			Containers: []corev1.Container{{
				Name:         "node-exporter",
				Image:        nodeExporterImage,
				Args:         []string{"--path.rootfs=/host"},
				Ports:        []corev1.ContainerPort{{Name: "metrics", ContainerPort: 9100, HostPort: 9100}},
				Resources:    corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("25m"), corev1.ResourceMemory: resource.MustParse("64Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi")}},
				VolumeMounts: []corev1.VolumeMount{{Name: "host-root", MountPath: "/host", ReadOnly: true}},
			}},
		}},
	}}
	current, err := c.Clientset.AppsV1().DaemonSets(victoriaMetricsNamespace).Get(c.Ctx(), nodeExporterName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.Clientset.AppsV1().DaemonSets(victoriaMetricsNamespace).Create(c.Ctx(), daemonSet, metav1.CreateOptions{})
	} else if err == nil {
		daemonSet.ResourceVersion = current.ResourceVersion
		_, err = c.Clientset.AppsV1().DaemonSets(victoriaMetricsNamespace).Update(c.Ctx(), daemonSet, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("创建 node-exporter DaemonSet 失败: %w", err)
	}
	return nil
}

func createOrUpdateConfigMap(c *Client, resource *corev1.ConfigMap) error {
	current, err := c.Clientset.CoreV1().ConfigMaps(victoriaMetricsNamespace).Get(c.Ctx(), resource.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.Clientset.CoreV1().ConfigMaps(victoriaMetricsNamespace).Create(c.Ctx(), resource, metav1.CreateOptions{})
	} else if err == nil {
		resource.ResourceVersion = current.ResourceVersion
		_, err = c.Clientset.CoreV1().ConfigMaps(victoriaMetricsNamespace).Update(c.Ctx(), resource, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("创建 VictoriaMetrics 采集配置失败: %w", err)
	}
	return nil
}

func createOrUpdateClusterRole(c *Client, resource *rbacv1.ClusterRole) error {
	current, err := c.Clientset.RbacV1().ClusterRoles().Get(c.Ctx(), resource.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.Clientset.RbacV1().ClusterRoles().Create(c.Ctx(), resource, metav1.CreateOptions{})
	} else if err == nil {
		resource.ResourceVersion = current.ResourceVersion
		_, err = c.Clientset.RbacV1().ClusterRoles().Update(c.Ctx(), resource, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("创建 VictoriaMetrics 采集权限失败: %w", err)
	}
	return nil
}

func createOrUpdateClusterRoleBinding(c *Client, resource *rbacv1.ClusterRoleBinding) error {
	current, err := c.Clientset.RbacV1().ClusterRoleBindings().Get(c.Ctx(), resource.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.Clientset.RbacV1().ClusterRoleBindings().Create(c.Ctx(), resource, metav1.CreateOptions{})
	} else if err == nil {
		resource.ResourceVersion = current.ResourceVersion
		_, err = c.Clientset.RbacV1().ClusterRoleBindings().Update(c.Ctx(), resource, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("绑定 VictoriaMetrics 采集权限失败: %w", err)
	}
	return nil
}

func victoriaMetricsConfigFromDeployment(deployment *appsv1.Deployment) (VictoriaMetricsConfig, bool) {
	config := VictoriaMetricsConfig{NodeName: deployment.Spec.Template.Spec.NodeSelector[corev1.LabelHostname]}
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name == "storage" && volume.HostPath != nil {
			config.DataPath = volume.HostPath.Path
			break
		}
	}
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name != "victoria-metrics" {
			continue
		}
		for _, arg := range container.Args {
			if raw, ok := strings.CutPrefix(arg, "-retentionPeriod="); ok {
				value := strings.TrimSuffix(raw, "d")
				config.RetentionDays, _ = strconv.Atoi(value)
			}
		}
	}
	return config, config.NodeName != "" && config.DataPath != ""
}

func victoriaMetricsLabels() map[string]string {
	return map[string]string{"app.kubernetes.io/name": "victoria-metrics", "app.kubernetes.io/managed-by": "cylism-manager", "cylism.io/component": "monitoring"}
}

func nodeExporterLabels() map[string]string {
	return map[string]string{"app.kubernetes.io/name": "node-exporter", "app.kubernetes.io/managed-by": "cylism-manager", "cylism.io/component": "monitoring"}
}

// VictoriaMetricsServiceURL returns the in-cluster endpoint used by the platform API.
func VictoriaMetricsServiceURL() string {
	return "http://" + victoriaMetricsName + "." + victoriaMetricsNamespace + ".svc:8428"
}

func victoriaMetricsScrapeConfig() string {
	return `global:
  scrape_interval: 30s
  scrape_timeout: 10s
scrape_configs:
  - job_name: victoria-metrics
    static_configs:
      - targets: [localhost:8428]
  - job_name: kubernetes-nodes
    scheme: https
    kubernetes_sd_configs:
      - role: node
    bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
    tls_config:
      ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
    relabel_configs:
      - target_label: __address__
        replacement: kubernetes.default.svc:443
      - source_labels: [__meta_kubernetes_node_name]
        target_label: __metrics_path__
        replacement: /api/v1/nodes/${1}/proxy/metrics
      - source_labels: [__meta_kubernetes_node_name]
        target_label: node
  - job_name: kubernetes-cadvisor
    scheme: https
    kubernetes_sd_configs:
      - role: node
    bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
    tls_config:
      ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
    relabel_configs:
      - target_label: __address__
        replacement: kubernetes.default.svc:443
      - source_labels: [__meta_kubernetes_node_name]
        target_label: __metrics_path__
        replacement: /api/v1/nodes/${1}/proxy/metrics/cadvisor
      - source_labels: [__meta_kubernetes_node_name]
        target_label: node
  - job_name: node-exporter
    kubernetes_sd_configs:
      - role: pod
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_label_app_kubernetes_io_name]
        regex: node-exporter
        action: keep
      - source_labels: [__meta_kubernetes_pod_host_ip]
        target_label: __address__
        replacement: ${1}:9100
      - source_labels: [__meta_kubernetes_pod_node_name]
        target_label: node
`
}

func victoriaMetricsScrapeConfigHash() string {
	checksum := sha256.Sum256([]byte(victoriaMetricsScrapeConfig()))
	return fmt.Sprintf("%x", checksum[:])
}

func deploymentStatusMessage(deployment *appsv1.Deployment) string {
	for _, condition := range deployment.Status.Conditions {
		if condition.Message != "" {
			return condition.Message
		}
	}
	return "VictoriaMetrics 正在启动，等待工作负载就绪"
}

func nodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
