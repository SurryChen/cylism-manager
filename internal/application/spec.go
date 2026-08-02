package application

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	ManagedByLabel       = "app.kubernetes.io/managed-by"
	ManagedByValue       = "cylism-manager"
	ProjectLabel         = "app.kubernetes.io/part-of"
	ApplicationNameLabel = "app.kubernetes.io/name"
	EnvironmentLabel     = "cylism.io/environment"
	ReleaseLabel         = "cylism.io/release"

	ExposureCluster = "cluster"
	ExposureTailnet = "tailnet"
	ExposurePublic  = "public"
)

type ReleaseSpec struct {
	Image      string   `json:"image"`
	Version    string   `json:"version,omitempty"`
	Command    []string `json:"command,omitempty"`
	Args       []string `json:"args,omitempty"`
	RegistryID uint     `json:"registry_id,omitempty"`
	// 以下字段只在发布执行期存在，禁止写入 API 响应或发布快照。
	RegistryEndpoint                    string            `json:"-"`
	RegistryAuthType                    string            `json:"-"`
	RegistryUsername                    string            `json:"-"`
	RegistryCredential                  string            `json:"-"`
	ImageVerificationEndpoint           string            `json:"-"`
	ImageVerificationUsername           string            `json:"-"`
	ImageVerificationCredential         string            `json:"-"`
	ImageVerificationInsecureSkipVerify bool              `json:"-"`
	ContainerPort                       int32             `json:"container_port"`
	Replicas                            int32             `json:"replicas"`
	Resources                           ResourceSpec      `json:"resources"`
	Health                              HealthSpec        `json:"health"`
	Config                              map[string]string `json:"config,omitempty"`
	Secrets                             map[string]string `json:"secrets,omitempty"`
	NodeName                            string            `json:"node_name,omitempty"`
	Volumes                             []VolumeMountSpec `json:"volumes,omitempty"`
	Service                             ServiceSpec       `json:"service"`
	Endpoint                            EndpointSpec      `json:"endpoint"`
}

// VolumeMountSpec describes a platform-managed PVC mounted into the main
// application container. PVC lifecycle remains independent from releases.
type VolumeMountSpec struct {
	ClaimName string `json:"claim_name"`
	MountPath string `json:"mount_path"`
	ReadOnly  bool   `json:"read_only"`
}

type ResourceSpec struct {
	RequestsCPU    string `json:"requests_cpu"`
	RequestsMemory string `json:"requests_memory"`
	LimitsCPU      string `json:"limits_cpu"`
	LimitsMemory   string `json:"limits_memory"`
}

type HealthSpec struct {
	ReadinessEnabled bool   `json:"readiness_enabled"`
	ReadinessType    string `json:"readiness_type,omitempty"`
	ReadinessPath    string `json:"readiness_path,omitempty"`
	LivenessEnabled  bool   `json:"liveness_enabled"`
	LivenessType     string `json:"liveness_type,omitempty"`
	LivenessPath     string `json:"liveness_path,omitempty"`
}

type ServiceSpec struct {
	Port       int32 `json:"port"`
	TargetPort int32 `json:"target_port"`
}

type EndpointSpec struct {
	Exposure               string `json:"exposure"`
	DomainID               uint   `json:"domain_id,omitempty"`
	Domain                 string `json:"domain,omitempty"`
	Path                   string `json:"path,omitempty"`
	TLSEnabled             bool   `json:"tls_enabled"`
	IssuerRef              string `json:"issuer_ref,omitempty"`
	IssuerKind             string `json:"issuer_kind,omitempty"`
	ManagedCertificateName string `json:"managed_certificate_name,omitempty"`
	ManagedTLSSecretName   string `json:"managed_tls_secret_name,omitempty"`
}

type ValidationIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ApplicationContext struct {
	ProjectID       uint
	EnvironmentID   uint
	ProjectName     string
	EnvironmentName string
	ApplicationName string
	Namespace       string
	ReleaseSequence uint
}

type RenderedResources struct {
	ConfigMap       *corev1.ConfigMap
	Secret          *corev1.Secret
	ImagePullSecret *corev1.Secret
	Deployment      *appsv1.Deployment
	Service         *corev1.Service
	Certificate     *unstructured.Unstructured
	Ingress         *networkingv1.Ingress
	SanitizedSpec   ReleaseSpec
}

func ValidateReleaseSpec(spec ReleaseSpec) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	if strings.TrimSpace(spec.Image) == "" {
		issues = append(issues, ValidationIssue{Field: "image", Message: "镜像不能为空"})
	}
	if spec.ContainerPort < 1 || spec.ContainerPort > 65535 {
		issues = append(issues, ValidationIssue{Field: "container_port", Message: "容器端口必须在 1 到 65535 之间"})
	}
	if spec.Replicas < 1 {
		issues = append(issues, ValidationIssue{Field: "replicas", Message: "副本数至少为 1"})
	}
	if strings.TrimSpace(spec.NodeName) != "" && len(validation.IsDNS1123Subdomain(spec.NodeName)) > 0 {
		issues = append(issues, ValidationIssue{Field: "node_name", Message: "部署节点名称无效"})
	}
	claimNames := make(map[string]struct{}, len(spec.Volumes))
	mountPaths := make(map[string]struct{}, len(spec.Volumes))
	for index, volume := range spec.Volumes {
		field := fmt.Sprintf("volumes[%d]", index)
		claimName := strings.TrimSpace(volume.ClaimName)
		mountPath := strings.TrimSpace(volume.MountPath)
		if len(validation.IsDNS1123Label(claimName)) > 0 {
			issues = append(issues, ValidationIssue{Field: field + ".claim_name", Message: "PVC 名称无效"})
		}
		if !strings.HasPrefix(mountPath, "/") || path.Clean(mountPath) != mountPath {
			issues = append(issues, ValidationIssue{Field: field + ".mount_path", Message: "挂载路径必须是规范的绝对路径"})
		}
		if _, exists := claimNames[claimName]; exists {
			issues = append(issues, ValidationIssue{Field: field + ".claim_name", Message: "同一 PVC 不能重复挂载"})
		}
		if _, exists := mountPaths[mountPath]; exists {
			issues = append(issues, ValidationIssue{Field: field + ".mount_path", Message: "同一挂载路径不能重复使用"})
		}
		claimNames[claimName] = struct{}{}
		mountPaths[mountPath] = struct{}{}
	}
	if len(spec.Volumes) > 0 && spec.Replicas > 1 {
		issues = append(issues, ValidationIssue{Field: "volumes", Message: "ReadWriteOnce PVC 仅支持单副本应用"})
	}
	parseQuantity := func(field, value string) *resource.Quantity {
		quantity, err := resource.ParseQuantity(value)
		if err != nil || strings.TrimSpace(value) == "" {
			issues = append(issues, ValidationIssue{Field: field, Message: "资源数量格式无效"})
			return nil
		}
		return &quantity
	}
	requestCPU := parseQuantity("resources.requests_cpu", spec.Resources.RequestsCPU)
	requestMemory := parseQuantity("resources.requests_memory", spec.Resources.RequestsMemory)
	limitCPU := parseQuantity("resources.limits_cpu", spec.Resources.LimitsCPU)
	limitMemory := parseQuantity("resources.limits_memory", spec.Resources.LimitsMemory)
	if requestCPU != nil && limitCPU != nil && requestCPU.Cmp(*limitCPU) > 0 {
		issues = append(issues, ValidationIssue{Field: "resources.requests_cpu", Message: "CPU 请求不能大于限制"})
	}
	if requestMemory != nil && limitMemory != nil && requestMemory.Cmp(*limitMemory) > 0 {
		issues = append(issues, ValidationIssue{Field: "resources.requests_memory", Message: "内存请求不能大于限制"})
	}
	if enabled, probeType := healthProbeEnabled(spec.Health.ReadinessEnabled, spec.Health.ReadinessType, spec.Health.ReadinessPath); enabled && probeType == "http" && !strings.HasPrefix(spec.Health.ReadinessPath, "/") {
		issues = append(issues, ValidationIssue{Field: "health.readiness_path", Message: "HTTP 就绪检查路径必须以 / 开头"})
	}
	if enabled, probeType := healthProbeEnabled(spec.Health.LivenessEnabled, spec.Health.LivenessType, spec.Health.LivenessPath); enabled && probeType == "http" && !strings.HasPrefix(spec.Health.LivenessPath, "/") {
		issues = append(issues, ValidationIssue{Field: "health.liveness_path", Message: "HTTP 存活检查路径必须以 / 开头"})
	}
	if _, probeType := healthProbeEnabled(spec.Health.ReadinessEnabled, spec.Health.ReadinessType, spec.Health.ReadinessPath); probeType != "http" && probeType != "tcp" {
		issues = append(issues, ValidationIssue{Field: "health.readiness_type", Message: "就绪检查仅支持 HTTP 或 TCP"})
	}
	if _, probeType := healthProbeEnabled(spec.Health.LivenessEnabled, spec.Health.LivenessType, spec.Health.LivenessPath); probeType != "http" && probeType != "tcp" {
		issues = append(issues, ValidationIssue{Field: "health.liveness_type", Message: "存活检查仅支持 HTTP 或 TCP"})
	}
	if spec.Service.Port < 1 || spec.Service.Port > 65535 || spec.Service.TargetPort < 1 || spec.Service.TargetPort > 65535 {
		issues = append(issues, ValidationIssue{Field: "service", Message: "Service 端口必须在 1 到 65535 之间"})
	}
	switch spec.Endpoint.Exposure {
	case ExposureCluster, ExposureTailnet:
		if spec.Endpoint.TLSEnabled {
			issues = append(issues, ValidationIssue{Field: "endpoint.tls_enabled", Message: "仅公网入口支持 cert-manager TLS"})
		}
	case ExposurePublic:
		if strings.TrimSpace(spec.Endpoint.Domain) == "" {
			issues = append(issues, ValidationIssue{Field: "endpoint.domain", Message: "公网入口必须提供域名"})
		}
		if spec.Endpoint.TLSEnabled && strings.TrimSpace(spec.Endpoint.IssuerRef) == "" && strings.TrimSpace(spec.Endpoint.ManagedCertificateName) == "" {
			issues = append(issues, ValidationIssue{Field: "endpoint.issuer_ref", Message: "启用 TLS 时必须指定 Issuer"})
		}
		if spec.Endpoint.TLSEnabled && (strings.TrimSpace(spec.Endpoint.ManagedCertificateName) == "") != (strings.TrimSpace(spec.Endpoint.ManagedTLSSecretName) == "") {
			issues = append(issues, ValidationIssue{Field: "endpoint", Message: "受管域名证书信息不完整"})
		}
		if spec.Endpoint.TLSEnabled && spec.Endpoint.IssuerKind != "" && spec.Endpoint.IssuerKind != "Issuer" && spec.Endpoint.IssuerKind != "ClusterIssuer" {
			issues = append(issues, ValidationIssue{Field: "endpoint.issuer_kind", Message: "Issuer 类型必须为 Issuer 或 ClusterIssuer"})
		}
	default:
		issues = append(issues, ValidationIssue{Field: "endpoint.exposure", Message: "暴露模式必须为 cluster、tailnet 或 public"})
	}
	return issues
}

func RenderResources(context ApplicationContext, spec ReleaseSpec) (*RenderedResources, error) {
	if issues := ValidateReleaseSpec(spec); len(issues) > 0 {
		return nil, fmt.Errorf("发布定义无效: %s", issues[0].Message)
	}
	if context.ApplicationName == "" || context.Namespace == "" || context.ProjectName == "" || context.EnvironmentName == "" {
		return nil, fmt.Errorf("应用上下文不完整")
	}
	if err := ValidateApplicationName(context.ApplicationName); err != nil {
		return nil, err
	}
	labels := managedLabels(context)
	result := &RenderedResources{SanitizedSpec: SanitizeReleaseSpec(spec)}
	configName := context.ApplicationName + "-config"
	secretName := context.ApplicationName + "-secret"

	if len(spec.Config) > 0 {
		result.ConfigMap = &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: configName, Namespace: context.Namespace, Labels: labels}, Data: cloneStringMap(spec.Config)}
	}
	if len(spec.Secrets) > 0 {
		if hasSecretValues(spec.Secrets) {
			result.Secret = &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: context.Namespace, Labels: labels}, Type: corev1.SecretTypeOpaque, StringData: cloneStringMap(spec.Secrets)}
		}
	}
	if spec.RegistryID != 0 && spec.RegistryAuthType != "" && spec.RegistryAuthType != "anonymous" {
		pullSecret, err := imagePullSecret(context, labels, spec)
		if err != nil {
			return nil, err
		}
		result.ImagePullSecret = pullSecret
	}

	container := corev1.Container{
		Name:    context.ApplicationName,
		Image:   spec.Image,
		Command: append([]string(nil), spec.Command...),
		Args:    append([]string(nil), spec.Args...),
		Ports:   []corev1.ContainerPort{{Name: "http", ContainerPort: spec.ContainerPort}},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(spec.Resources.RequestsCPU), corev1.ResourceMemory: resource.MustParse(spec.Resources.RequestsMemory)},
			Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(spec.Resources.LimitsCPU), corev1.ResourceMemory: resource.MustParse(spec.Resources.LimitsMemory)},
		},
	}
	if enabled, probeType := healthProbeEnabled(spec.Health.ReadinessEnabled, spec.Health.ReadinessType, spec.Health.ReadinessPath); enabled {
		container.ReadinessProbe = healthProbe(probeType, spec.Health.ReadinessPath, spec.ContainerPort)
	}
	if enabled, probeType := healthProbeEnabled(spec.Health.LivenessEnabled, spec.Health.LivenessType, spec.Health.LivenessPath); enabled {
		container.LivenessProbe = healthProbe(probeType, spec.Health.LivenessPath, spec.ContainerPort)
	}
	if result.ConfigMap != nil {
		container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: configName}}})
	}
	if len(spec.Secrets) > 0 {
		container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: secretName}}})
	}
	volumes := make([]corev1.Volume, 0, len(spec.Volumes))
	for index, volume := range spec.Volumes {
		name := fmt.Sprintf("pvc-%d", index)
		volumes = append(volumes, corev1.Volume{Name: name, VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: volume.ClaimName, ReadOnly: volume.ReadOnly}}})
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: name, MountPath: volume.MountPath, ReadOnly: volume.ReadOnly})
	}
	replicas := spec.Replicas
	podSpec := corev1.PodSpec{Containers: []corev1.Container{container}, Volumes: volumes}
	if spec.NodeName != "" {
		podSpec.NodeSelector = map[string]string{corev1.LabelHostname: spec.NodeName}
	}
	if result.ImagePullSecret != nil {
		podSpec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: result.ImagePullSecret.Name}}
	}
	result.Deployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: context.ApplicationName, Namespace: context.Namespace, Labels: labels, Annotations: map[string]string{ReleaseLabel: fmt.Sprintf("%d", context.ReleaseSequence)}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: workloadSelector(context.ApplicationName)},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: mergeLabels(labels, workloadSelector(context.ApplicationName))}, Spec: podSpec},
		},
	}
	if len(spec.Volumes) > 0 {
		result.Deployment.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}
	}
	result.Service = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: context.ApplicationName, Namespace: context.Namespace, Labels: labels},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, Selector: workloadSelector(context.ApplicationName), Ports: []corev1.ServicePort{{Name: "http", Port: spec.Service.Port, TargetPort: intstr.FromInt32(spec.Service.TargetPort)}}},
	}

	if spec.Endpoint.Exposure == ExposurePublic {
		result.Ingress = endpointIngress(context, spec.Endpoint, spec.Service.Port)
	}
	return result, nil
}

func endpointIngress(context ApplicationContext, endpoint EndpointSpec, servicePort int32) *networkingv1.Ingress {
	pathType := networkingv1.PathTypePrefix
	path := endpoint.Path
	if path == "" {
		path = "/"
	}
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: context.ApplicationName, Namespace: context.Namespace, Labels: managedLabels(context)},
		Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{
			Host: endpoint.Domain,
			IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{
				Path: path, PathType: &pathType,
				Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: context.ApplicationName, Port: networkingv1.ServiceBackendPort{Number: servicePort}}},
			}}}},
		}}},
	}
	if endpoint.TLSEnabled {
		tlsName := context.ApplicationName + "-tls"
		if endpoint.ManagedTLSSecretName != "" {
			tlsName = endpoint.ManagedTLSSecretName
		}
		ingress.Spec.TLS = []networkingv1.IngressTLS{{Hosts: []string{endpoint.Domain}, SecretName: tlsName}}
	}
	return ingress
}

func imagePullSecret(context ApplicationContext, labels map[string]string, spec ReleaseSpec) (*corev1.Secret, error) {
	if strings.TrimSpace(spec.RegistryEndpoint) == "" || strings.TrimSpace(spec.RegistryUsername) == "" || strings.TrimSpace(spec.RegistryCredential) == "" {
		return nil, fmt.Errorf("镜像仓库凭据不完整")
	}
	config, err := json.Marshal(map[string]map[string]map[string]string{"auths": {
		spec.RegistryEndpoint: {"username": spec.RegistryUsername, "password": spec.RegistryCredential, "auth": base64.StdEncoding.EncodeToString([]byte(spec.RegistryUsername + ":" + spec.RegistryCredential))},
	}})
	if err != nil {
		return nil, fmt.Errorf("生成镜像仓库凭据: %w", err)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("cylism-regcred-%d", spec.RegistryID), Namespace: context.Namespace, Labels: labels},
		Type:       corev1.SecretTypeDockerConfigJson,
		Data:       map[string][]byte{corev1.DockerConfigJsonKey: config},
	}, nil
}

func managedLabels(context ApplicationContext) map[string]string {
	return map[string]string{
		ManagedByLabel: ManagedByValue, ProjectLabel: fmt.Sprintf("project-%d", context.ProjectID), ApplicationNameLabel: context.ApplicationName,
		EnvironmentLabel: fmt.Sprintf("environment-%d", context.EnvironmentID),
	}
}

// ValidateApplicationName ensures the application can be used as a Kubernetes resource name and label value.
func ValidateApplicationName(name string) error {
	if validationErrors := validation.IsDNS1123Label(name); len(validationErrors) > 0 {
		return fmt.Errorf("应用名称必须为 1-63 位小写字母、数字或连字符，且以字母或数字开头和结尾")
	}
	return nil
}

func workloadSelector(applicationName string) map[string]string {
	return map[string]string{ApplicationNameLabel: applicationName}
}

func httpProbe(path string, port int32) *corev1.Probe {
	return &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: path, Port: intstr.FromInt32(port)}}, InitialDelaySeconds: 5, PeriodSeconds: 10}
}

func healthProbeEnabled(enabled bool, probeType, path string) (bool, string) {
	if probeType == "" {
		probeType = "http"
	}
	return enabled, probeType
}

func healthProbe(probeType, path string, port int32) *corev1.Probe {
	if probeType == "tcp" {
		return &corev1.Probe{ProbeHandler: corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(port)}}, InitialDelaySeconds: 5, PeriodSeconds: 10}
	}
	return httpProbe(path, port)
}

func certificateResource(context ApplicationContext, domain, secretName, issuerRef, issuerKind string, labels map[string]string) *unstructured.Unstructured {
	if issuerKind == "" {
		issuerKind = "ClusterIssuer"
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cert-manager.io/v1", "kind": "Certificate",
		"metadata": map[string]interface{}{"name": context.ApplicationName + "-tls", "namespace": context.Namespace, "labels": labels},
		"spec":     map[string]interface{}{"secretName": secretName, "dnsNames": []interface{}{domain}, "issuerRef": map[string]interface{}{"name": issuerRef, "kind": issuerKind}},
	}}
}

func SanitizeReleaseSpec(spec ReleaseSpec) ReleaseSpec {
	spec.RegistryEndpoint = ""
	spec.RegistryAuthType = ""
	spec.RegistryUsername = ""
	spec.RegistryCredential = ""
	spec.ImageVerificationEndpoint = ""
	spec.ImageVerificationUsername = ""
	spec.ImageVerificationCredential = ""
	spec.ImageVerificationInsecureSkipVerify = false
	sanitized := spec
	sanitized.Config = cloneStringMap(spec.Config)
	sanitized.Secrets = make(map[string]string, len(spec.Secrets))
	for key := range spec.Secrets {
		sanitized.Secrets[key] = ""
	}
	return sanitized
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func hasSecretValues(values map[string]string) bool {
	for _, value := range values {
		if value != "" {
			return true
		}
	}
	return false
}

func mergeLabels(maps ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, labels := range maps {
		for key, value := range labels {
			merged[key] = value
		}
	}
	return merged
}

func SortedSecretKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
