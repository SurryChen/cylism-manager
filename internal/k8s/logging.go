package k8s

import (
	"fmt"
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
	LoggingStateNotInstalled = "not_installed"
	LoggingStateInstalling   = "installing"
	LoggingStateReady        = "ready"
	LoggingStateDegraded     = "degraded"

	lokiName                   = "cylism-loki"
	lokiPVCName                = "cylism-loki-data"
	alloyName                  = "cylism-alloy"
	loggingConfigName          = "cylism-logging-config"
	loggingRetentionAnnotation = "cylism.io/log-retention-days"
	lokiImage                  = "grafana/loki:3.5.3"
	alloyImage                 = "grafana/alloy:v1.9.1"
)

// LoggingConfig controls the platform-owned Loki data store and container log collector.
type LoggingConfig struct {
	NodeName         string `json:"node_name"`
	Storage          string `json:"storage"`
	StorageClassName string `json:"storage_class_name,omitempty"`
	RetentionDays    int    `json:"retention_days"`
}

// LoggingStatus describes the managed logging resources without exposing Loki externally.
type LoggingStatus struct {
	State         string `json:"state"`
	Message       string `json:"message"`
	NodeName      string `json:"node_name,omitempty"`
	LokiReady     int32  `json:"loki_ready"`
	AlloyDesired  int32  `json:"alloy_desired"`
	AlloyReady    int32  `json:"alloy_ready"`
	PVCName       string `json:"pvc_name,omitempty"`
	Storage       string `json:"storage,omitempty"`
	StorageClass  string `json:"storage_class_name,omitempty"`
	RetentionDays int    `json:"retention_days"`
}

// LoggingStatus returns the current Loki and Alloy readiness state.
func (c *Client) LoggingStatus() *LoggingStatus {
	status := &LoggingStatus{State: LoggingStateNotInstalled, Message: "尚未启用容器日志采集", RetentionDays: 14}
	if c == nil || c.Clientset == nil {
		status.State = LoggingStateDegraded
		status.Message = "Kubernetes 客户端未初始化"
		return status
	}

	loki, err := c.Clientset.AppsV1().StatefulSets(victoriaMetricsNamespace).Get(c.Ctx(), lokiName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return status
	}
	if err != nil {
		status.State = LoggingStateDegraded
		status.Message = fmt.Sprintf("读取 Loki 状态失败: %v", err)
		return status
	}
	status.State = LoggingStateInstalling
	status.Message = "Loki 正在启动，等待日志采集器就绪"
	status.NodeName = loki.Spec.Template.Spec.NodeSelector[corev1.LabelHostname]
	status.LokiReady = loki.Status.ReadyReplicas
	if raw := strings.TrimSpace(loki.Annotations[loggingRetentionAnnotation]); raw != "" {
		status.RetentionDays, _ = strconv.Atoi(raw)
	}
	status.PVCName = lokiPVCName
	if claim, claimErr := c.Clientset.CoreV1().PersistentVolumeClaims(victoriaMetricsNamespace).Get(c.Ctx(), lokiPVCName, metav1.GetOptions{}); claimErr == nil {
		status.Storage = claim.Spec.Resources.Requests.Storage().String()
		if claim.Spec.StorageClassName != nil {
			status.StorageClass = *claim.Spec.StorageClassName
		}
	}

	alloy, alloyErr := c.Clientset.AppsV1().DaemonSets(victoriaMetricsNamespace).Get(c.Ctx(), alloyName, metav1.GetOptions{})
	if apierrors.IsNotFound(alloyErr) {
		status.State = LoggingStateDegraded
		status.Message = "Loki 已创建，但 Alloy 日志采集器未安装"
		return status
	}
	if alloyErr != nil {
		status.State = LoggingStateDegraded
		status.Message = fmt.Sprintf("读取 Alloy 状态失败: %v", alloyErr)
		return status
	}
	status.AlloyDesired = alloy.Status.DesiredNumberScheduled
	status.AlloyReady = alloy.Status.NumberAvailable
	if status.LokiReady > 0 && status.AlloyDesired > 0 && status.AlloyReady == status.AlloyDesired {
		status.State = LoggingStateReady
		status.Message = "Loki 与 Alloy 已就绪，正在采集容器标准输出日志"
	}
	return status
}

// InstallLogging creates or updates the platform-owned Loki and Alloy resources.
func (c *Client) InstallLogging(config LoggingConfig) (*LoggingStatus, error) {
	if c == nil || c.Clientset == nil {
		return c.LoggingStatus(), fmt.Errorf("Kubernetes 客户端未初始化")
	}
	if err := normalizeLoggingConfig(&config); err != nil {
		return c.LoggingStatus(), err
	}
	existing, err := c.Clientset.AppsV1().StatefulSets(victoriaMetricsNamespace).Get(c.Ctx(), lokiName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return c.LoggingStatus(), fmt.Errorf("读取 Loki 配置失败: %w", err)
	}
	if err == nil {
		oldNode := existing.Spec.Template.Spec.NodeSelector[corev1.LabelHostname]
		if oldNode != "" && oldNode != config.NodeName {
			return c.LoggingStatus(), fmt.Errorf("已安装日志存储的数据节点不能直接修改；当前数据仍绑定在 %s", oldNode)
		}
		config.NodeName = oldNode
		config.Storage = ""
		config.StorageClassName = ""
	}
	node, err := c.Clientset.CoreV1().Nodes().Get(c.Ctx(), config.NodeName, metav1.GetOptions{})
	if err != nil {
		return c.LoggingStatus(), fmt.Errorf("读取日志数据节点失败: %w", err)
	}
	if !nodeReady(node) {
		return c.LoggingStatus(), fmt.Errorf("日志数据节点 %s 未就绪", config.NodeName)
	}
	if err := ensureMonitoringNamespace(c); err != nil {
		return c.LoggingStatus(), err
	}
	if err := ensureAlloyAccess(c); err != nil {
		return c.LoggingStatus(), err
	}
	if err := upsertLokiPVC(c, config); err != nil {
		return c.LoggingStatus(), err
	}
	if err := upsertLoggingConfig(c, config); err != nil {
		return c.LoggingStatus(), err
	}
	if err := upsertLokiService(c); err != nil {
		return c.LoggingStatus(), err
	}
	if err := upsertLokiStatefulSet(c, config); err != nil {
		return c.LoggingStatus(), err
	}
	if err := upsertAlloyDaemonSet(c); err != nil {
		return c.LoggingStatus(), err
	}
	return c.LoggingStatus(), nil
}

// UninstallLogging removes compute and configuration while retaining the Loki PVC.
func (c *Client) UninstallLogging() error {
	if c == nil || c.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	resources := []func() error{
		func() error {
			return c.Clientset.AppsV1().DaemonSets(victoriaMetricsNamespace).Delete(c.Ctx(), alloyName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.AppsV1().StatefulSets(victoriaMetricsNamespace).Delete(c.Ctx(), lokiName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().Services(victoriaMetricsNamespace).Delete(c.Ctx(), lokiName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().ConfigMaps(victoriaMetricsNamespace).Delete(c.Ctx(), loggingConfigName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.RbacV1().ClusterRoleBindings().Delete(c.Ctx(), alloyName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.RbacV1().ClusterRoles().Delete(c.Ctx(), alloyName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().ServiceAccounts(victoriaMetricsNamespace).Delete(c.Ctx(), alloyName, metav1.DeleteOptions{})
		},
	}
	for _, remove := range resources {
		if err := remove(); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("卸载日志采集资源失败: %w", err)
		}
	}
	return nil
}

func normalizeLoggingConfig(config *LoggingConfig) error {
	config.NodeName = strings.TrimSpace(config.NodeName)
	config.Storage = strings.TrimSpace(config.Storage)
	config.StorageClassName = strings.TrimSpace(config.StorageClassName)
	if config.NodeName == "" {
		return fmt.Errorf("请选择日志数据节点")
	}
	if config.RetentionDays < 1 || config.RetentionDays > 365 {
		return fmt.Errorf("日志保留天数必须在 1 到 365 天之间")
	}
	if config.Storage == "" {
		return nil
	}
	quantity, err := resource.ParseQuantity(config.Storage)
	if err != nil || quantity.Sign() <= 0 {
		return fmt.Errorf("日志存储容量格式无效")
	}
	return nil
}

func ensureAlloyAccess(c *Client) error {
	serviceAccount := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: alloyName, Namespace: victoriaMetricsNamespace, Labels: alloyLabels()}}
	if _, err := c.Clientset.CoreV1().ServiceAccounts(victoriaMetricsNamespace).Create(c.Ctx(), serviceAccount, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("创建 Alloy 服务账号失败: %w", err)
	}
	role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: alloyName, Labels: alloyLabels()}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods", "namespaces"}, Verbs: []string{"get", "list", "watch"}}}}
	if err := createOrUpdateClusterRole(c, role); err != nil {
		return err
	}
	binding := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: alloyName, Labels: alloyLabels()}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: alloyName, Namespace: victoriaMetricsNamespace}}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: alloyName}}
	if err := createOrUpdateClusterRoleBinding(c, binding); err != nil {
		return err
	}
	return nil
}

func upsertLokiPVC(c *Client, config LoggingConfig) error {
	claims := c.Clientset.CoreV1().PersistentVolumeClaims(victoriaMetricsNamespace)
	existing, err := claims.Get(c.Ctx(), lokiPVCName, metav1.GetOptions{})
	if err == nil {
		labels := existing.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		changed := labels[ManagedByLabel] != ManagedByValue || labels[InfrastructureLabel] != InfrastructureLoki
		labels[ManagedByLabel] = ManagedByValue
		labels[InfrastructureLabel] = InfrastructureLoki
		if !changed {
			return nil
		}
		existing.Labels = labels
		if _, err := claims.Update(c.Ctx(), existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("更新 Loki 存储卷归属失败: %w", err)
		}
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("读取 Loki 存储卷失败: %w", err)
	}
	storage, parseErr := resource.ParseQuantity(config.Storage)
	if parseErr != nil || storage.Sign() <= 0 {
		return fmt.Errorf("日志存储容量格式无效")
	}
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: lokiPVCName, Namespace: victoriaMetricsNamespace, Labels: infrastructurePVCLabels(InfrastructureLoki)}, Spec: corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: storage}}}}
	if config.StorageClassName != "" {
		claim.Spec.StorageClassName = &config.StorageClassName
	}
	if _, err := claims.Create(c.Ctx(), claim, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("创建 Loki 存储卷失败: %w", err)
	}
	return nil
}

func upsertLoggingConfig(c *Client, config LoggingConfig) error {
	return upsertNamedConfigMap(c, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: loggingConfigName, Namespace: victoriaMetricsNamespace, Labels: loggingLabels()}, Data: map[string]string{
		"loki.yaml":    renderLokiConfig(config.RetentionDays),
		"config.alloy": renderAlloyConfig(),
	}})
}

func upsertLokiService(c *Client) error {
	services := c.Clientset.CoreV1().Services(victoriaMetricsNamespace)
	current, err := services.Get(c.Ctx(), lokiName, metav1.GetOptions{})
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: lokiName, Namespace: victoriaMetricsNamespace, Labels: lokiLabels()}, Spec: corev1.ServiceSpec{ClusterIP: corev1.ClusterIPNone, Selector: lokiLabels(), Ports: []corev1.ServicePort{{Name: "http", Port: 3100, TargetPort: intstr.FromInt(3100)}}}}
	if apierrors.IsNotFound(err) {
		_, err = services.Create(c.Ctx(), service, metav1.CreateOptions{})
	} else if err == nil {
		service.ResourceVersion = current.ResourceVersion
		service.Spec.ClusterIP = current.Spec.ClusterIP
		service.Spec.ClusterIPs = current.Spec.ClusterIPs
		service.Spec.IPFamilies = current.Spec.IPFamilies
		service.Spec.IPFamilyPolicy = current.Spec.IPFamilyPolicy
		_, err = services.Update(c.Ctx(), service, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("创建 Loki Service 失败: %w", err)
	}
	return nil
}

func upsertLokiStatefulSet(c *Client, config LoggingConfig) error {
	replicas := int32(1)
	statefulSet := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: lokiName, Namespace: victoriaMetricsNamespace, Labels: lokiLabels(), Annotations: map[string]string{loggingRetentionAnnotation: strconv.Itoa(config.RetentionDays)}}, Spec: appsv1.StatefulSetSpec{
		ServiceName: lokiName,
		Replicas:    &replicas,
		Selector:    &metav1.LabelSelector{MatchLabels: lokiLabels()},
		Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: lokiLabels()}, Spec: corev1.PodSpec{
			NodeSelector: map[string]string{corev1.LabelHostname: config.NodeName},
			Volumes: []corev1.Volume{
				{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: lokiPVCName}}},
				{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: loggingConfigName}}}},
			},
			Containers: []corev1.Container{{
				Name:           "loki",
				Image:          lokiImage,
				Args:           []string{"-config.file=/etc/loki/loki.yaml"},
				Ports:          []corev1.ContainerPort{{Name: "http", ContainerPort: 3100}},
				Resources:      loggingResources("100m", "256Mi", "500m", "512Mi"),
				VolumeMounts:   []corev1.VolumeMount{{Name: "data", MountPath: "/loki"}, {Name: "config", MountPath: "/etc/loki", ReadOnly: true}},
				ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/ready", Port: intstr.FromInt(3100)}}, InitialDelaySeconds: 5, PeriodSeconds: 10},
				LivenessProbe:  &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/ready", Port: intstr.FromInt(3100)}}, InitialDelaySeconds: 15, PeriodSeconds: 15},
			}},
		}},
	}}
	current, err := c.Clientset.AppsV1().StatefulSets(victoriaMetricsNamespace).Get(c.Ctx(), lokiName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.Clientset.AppsV1().StatefulSets(victoriaMetricsNamespace).Create(c.Ctx(), statefulSet, metav1.CreateOptions{})
	} else if err == nil {
		statefulSet.ResourceVersion = current.ResourceVersion
		_, err = c.Clientset.AppsV1().StatefulSets(victoriaMetricsNamespace).Update(c.Ctx(), statefulSet, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("创建 Loki StatefulSet 失败: %w", err)
	}
	return nil
}

func upsertAlloyDaemonSet(c *Client) error {
	hostDirectory := corev1.HostPathDirectory
	daemonSet := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: alloyName, Namespace: victoriaMetricsNamespace, Labels: alloyLabels()}, Spec: appsv1.DaemonSetSpec{
		Selector: &metav1.LabelSelector{MatchLabels: alloyLabels()},
		Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: alloyLabels()}, Spec: corev1.PodSpec{
			ServiceAccountName: alloyName,
			Tolerations:        []corev1.Toleration{{Operator: corev1.TolerationOpExists}},
			Volumes: []corev1.Volume{
				{Name: "pod-logs", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/log/pods", Type: &hostDirectory}}},
				{Name: "container-logs", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/log/containers", Type: &hostDirectory}}},
				{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: loggingConfigName}}}},
				{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
			Containers: []corev1.Container{{
				Name:           "alloy",
				Image:          alloyImage,
				Args:           []string{"run", "/etc/alloy/config.alloy", "--server.http.listen-addr=0.0.0.0:12345", "--storage.path=/var/lib/alloy"},
				Resources:      loggingResources("25m", "64Mi", "100m", "128Mi"),
				VolumeMounts:   []corev1.VolumeMount{{Name: "pod-logs", MountPath: "/var/log/pods", ReadOnly: true}, {Name: "container-logs", MountPath: "/var/log/containers", ReadOnly: true}, {Name: "config", MountPath: "/etc/alloy", ReadOnly: true}, {Name: "data", MountPath: "/var/lib/alloy"}},
				ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/-/ready", Port: intstr.FromInt(12345)}}, InitialDelaySeconds: 5, PeriodSeconds: 10},
			}},
		}},
	}}
	current, err := c.Clientset.AppsV1().DaemonSets(victoriaMetricsNamespace).Get(c.Ctx(), alloyName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.Clientset.AppsV1().DaemonSets(victoriaMetricsNamespace).Create(c.Ctx(), daemonSet, metav1.CreateOptions{})
	} else if err == nil {
		daemonSet.ResourceVersion = current.ResourceVersion
		_, err = c.Clientset.AppsV1().DaemonSets(victoriaMetricsNamespace).Update(c.Ctx(), daemonSet, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("创建 Alloy DaemonSet 失败: %w", err)
	}
	return nil
}

func loggingResources(requestCPU, requestMemory, limitCPU, limitMemory string) corev1.ResourceRequirements {
	return corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(requestCPU), corev1.ResourceMemory: resource.MustParse(requestMemory)}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(limitCPU), corev1.ResourceMemory: resource.MustParse(limitMemory)}}
}

func loggingLabels() map[string]string {
	return map[string]string{ManagedByLabel: ManagedByValue, "cylism.io/component": "monitoring-logs"}
}

func lokiLabels() map[string]string {
	labels := loggingLabels()
	labels["app.kubernetes.io/name"] = "loki"
	return labels
}

func alloyLabels() map[string]string {
	labels := loggingLabels()
	labels["app.kubernetes.io/name"] = "alloy"
	return labels
}

// LokiServiceURL returns Loki's cluster-internal query and push endpoint.
func LokiServiceURL() string {
	return "http://" + lokiName + "." + victoriaMetricsNamespace + ".svc:3100"
}

func renderLokiConfig(retentionDays int) string {
	return fmt.Sprintf(`auth_enabled: false
server:
  http_listen_port: 3100
common:
  path_prefix: /loki
  storage:
    filesystem:
      chunks_directory: /loki/chunks
      rules_directory: /loki/rules
  replication_factor: 1
  ring:
    kvstore:
      store: inmemory
schema_config:
  configs:
    - from: 2024-01-01
      store: tsdb
      object_store: filesystem
      schema: v13
      index:
        prefix: index_
        period: 24h
limits_config:
  retention_period: %dh
  allow_structured_metadata: false
compactor:
  working_directory: /loki/compactor
  retention_enabled: true
  delete_request_store: filesystem
`, retentionDays*24)
}

func renderAlloyConfig() string {
	return `discovery.kubernetes "pods" {
  role = "pod"
}

discovery.relabel "container_logs" {
  targets = discovery.kubernetes.pods.targets

  rule {
    source_labels = ["__meta_kubernetes_pod_node_name"]
    target_label  = "node"
  }
  rule {
    source_labels = ["__meta_kubernetes_namespace"]
    target_label  = "namespace"
  }
  rule {
    source_labels = ["__meta_kubernetes_pod_name"]
    target_label  = "pod"
  }
  rule {
    source_labels = ["__meta_kubernetes_pod_container_name"]
    target_label  = "container"
  }
  rule {
    source_labels = ["__meta_kubernetes_pod_label_app_kubernetes_io_name"]
    target_label  = "workload"
  }
  rule {
    source_labels = ["__meta_kubernetes_pod_label_app_kubernetes_io_part_of"]
    target_label  = "project"
  }
  rule {
    source_labels = ["__meta_kubernetes_pod_label_cylism_io_environment"]
    target_label  = "environment"
  }
  rule {
    source_labels = ["__meta_kubernetes_pod_label_cylism_io_release"]
    target_label  = "release"
  }
  rule {
    source_labels = ["__meta_kubernetes_pod_uid", "__meta_kubernetes_pod_container_name"]
    separator     = "/"
    target_label  = "__path__"
    replacement   = "/var/log/pods/*$1/*.log"
  }
}

loki.source.file "container_logs" {
  targets    = discovery.relabel.container_logs.output
  forward_to = [loki.process.container_logs.receiver]
}

loki.process "container_logs" {
  stage.cri {}
  forward_to = [loki.write.default.receiver]
}

loki.write "default" {
  endpoint {
    url = "` + LokiServiceURL() + `/loki/api/v1/push"
  }
}
`
}
