package application

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"
	"path"
	"strings"
)

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
	issues = append(issues, validateContainerArguments("command", spec.Command)...)
	issues = append(issues, validateContainerArguments("args", spec.Args)...)
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
	issues = append(issues, validateFileMounts(spec.FileMounts, enabledStringMap(spec.Config, spec.ConfigDisabled), enabledStringMap(spec.Secrets, spec.SecretsDisabled))...)
	issues = append(issues, validateManagedKeys("config_managed_keys", spec.ConfigManagedKeys, spec.Config, spec.ConfigDisabled)...)
	issues = append(issues, validateManagedKeys("secret_managed_keys", spec.SecretManagedKeys, spec.Secrets, spec.SecretsDisabled)...)
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
	servicePorts, serviceIssues := validateServicePorts(spec.Service)
	issues = append(issues, serviceIssues...)
	serviceType := normalizeServiceType(spec.Service.Type)
	if serviceType == "" {
		issues = append(issues, ValidationIssue{Field: "service.type", Message: "Service 类型必须为 ClusterIP、NodePort 或 LoadBalancer"})
	}
	if serviceType == ServiceTypeClusterIP {
		for index, servicePort := range servicePorts {
			if servicePort.NodePort == 0 {
				continue
			}
			issues = append(issues, ValidationIssue{Field: serviceNodePortField(spec.Service, index), Message: "ClusterIP Service 不能指定 NodePort"})
		}
		if strings.TrimSpace(spec.Service.ExternalTrafficPolicy) != "" {
			issues = append(issues, ValidationIssue{Field: "service.external_traffic_policy", Message: "ClusterIP Service 不能指定外部流量策略"})
		}
	} else {
		if policy := strings.TrimSpace(spec.Service.ExternalTrafficPolicy); policy != "" && policy != string(corev1.ServiceExternalTrafficPolicyCluster) && policy != string(corev1.ServiceExternalTrafficPolicyLocal) {
			issues = append(issues, ValidationIssue{Field: "service.external_traffic_policy", Message: "外部流量策略必须为 Cluster 或 Local"})
		}
	}
	if _, hasTCPPort := spec.Service.PrimaryTCPPort(); !hasTCPPort && (spec.Health.ReadinessEnabled || spec.Health.LivenessEnabled) {
		issues = append(issues, ValidationIssue{Field: "health", Message: "UDP Service 不支持 HTTP 或 TCP 健康检查"})
	}
	switch spec.Endpoint.Exposure {
	case ExposureCluster, ExposureTailnet:
		if spec.Endpoint.TLSEnabled {
			issues = append(issues, ValidationIssue{Field: "endpoint.tls_enabled", Message: "仅公网入口支持 cert-manager TLS"})
		}
	case ExposurePublic:
		if _, hasTCPPort := spec.Service.PrimaryTCPPort(); !hasTCPPort {
			issues = append(issues, ValidationIssue{Field: "endpoint.exposure", Message: "UDP Service 不支持 HTTP Ingress 公网入口"})
		}
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

func validateManagedKeys(field string, managedKeys []string, values map[string]string, disabled []string) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	disabledSet := make(map[string]struct{}, len(disabled))
	for _, key := range disabled {
		disabledSet[strings.TrimSpace(key)] = struct{}{}
	}
	seen := make(map[string]struct{}, len(managedKeys))
	for index, rawKey := range managedKeys {
		key := strings.TrimSpace(rawKey)
		itemField := fmt.Sprintf("%s[%d]", field, index)
		if len(validation.IsConfigMapKey(key)) > 0 {
			issues = append(issues, ValidationIssue{Field: itemField, Message: "受管配置键名无效"})
			continue
		}
		if _, exists := seen[key]; exists {
			issues = append(issues, ValidationIssue{Field: itemField, Message: "受管配置键名不能重复"})
			continue
		}
		seen[key] = struct{}{}
		if _, exists := values[key]; !exists {
			issues = append(issues, ValidationIssue{Field: itemField, Message: fmt.Sprintf("受管配置键 %q 不存在", key)})
		}
		if _, disabledKey := disabledSet[key]; disabledKey {
			issues = append(issues, ValidationIssue{Field: itemField, Message: fmt.Sprintf("受管配置键 %q 已被禁用", key)})
		}
	}
	return issues
}

// NormalizeManagedKeys migrates the old per-file managed flag into the key-level
// fields. New fields take precedence once present; legacy flags are only consulted
// when the corresponding key list is absent.

func validateServicePorts(service ServiceSpec) ([]ServicePortSpec, []ValidationIssue) {
	ports := service.PortSpecs()
	issues := make([]ValidationIssue, 0)
	names := make(map[string]struct{}, len(ports))
	servicePorts := make(map[int32]struct{}, len(ports))
	for index, port := range ports {
		field := "service"
		if len(service.Ports) > 0 {
			field = fmt.Sprintf("service.ports[%d]", index)
			name := strings.TrimSpace(port.Name)
			if len(validation.IsValidPortName(name)) > 0 {
				issues = append(issues, ValidationIssue{Field: field + ".name", Message: "Service 端口名称无效"})
			} else if _, exists := names[name]; exists {
				issues = append(issues, ValidationIssue{Field: field + ".name", Message: "Service 端口名称不能重复"})
			} else {
				names[name] = struct{}{}
			}
		}
		if port.Port < 1 || port.Port > 65535 || port.TargetPort < 1 || port.TargetPort > 65535 {
			issues = append(issues, ValidationIssue{Field: field, Message: "Service 端口必须在 1 到 65535 之间"})
		}
		if _, exists := servicePorts[port.Port]; exists {
			issues = append(issues, ValidationIssue{Field: field + ".port", Message: "Service 端口不能重复"})
		}
		servicePorts[port.Port] = struct{}{}
		if normalizeServiceProtocol(port.Protocol) == "" {
			issues = append(issues, ValidationIssue{Field: field + ".protocol", Message: "Service 协议必须为 TCP 或 UDP"})
		}
		if port.NodePort < 0 || port.NodePort > 65535 {
			issues = append(issues, ValidationIssue{Field: serviceNodePortField(service, index), Message: "NodePort 必须在 1 到 65535 之间，或留空由集群分配"})
		}
	}
	return ports, issues
}

func serviceNodePortField(service ServiceSpec, index int) string {
	if len(service.Ports) == 0 {
		return "service.node_port"
	}
	return fmt.Sprintf("service.ports[%d].node_port", index)
}

func validateFileMounts(fileMounts []FileMountSpec, applicationConfig, applicationSecrets map[string]string) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	paths := make(map[string]struct{}, len(fileMounts))
	directories := make(map[string]string, len(fileMounts))
	for index, fileMount := range fileMounts {
		field := fmt.Sprintf("file_mounts[%d]", index)
		if fileMount.SourceType != FileMountSourceConfigMap && fileMount.SourceType != FileMountSourceSecret && fileMount.SourceType != FileMountSourceApplicationConfig && fileMount.SourceType != FileMountSourceApplicationSecret {
			issues = append(issues, ValidationIssue{Field: field + ".source_type", Message: "文件来源必须为当前应用 ConfigMap、当前应用 Secret、ConfigMap 或 Secret"})
		}
		if fileMount.Managed && fileMount.SourceType != FileMountSourceApplicationConfig && fileMount.SourceType != FileMountSourceApplicationSecret {
			issues = append(issues, ValidationIssue{Field: field + ".managed", Message: "仅当前应用 ConfigMap 或 Secret 文件可启用完整管理"})
		}
		if fileMount.SourceType != FileMountSourceApplicationConfig && fileMount.SourceType != FileMountSourceApplicationSecret && len(validation.IsDNS1123Subdomain(fileMount.SourceName)) > 0 {
			issues = append(issues, ValidationIssue{Field: field + ".source_name", Message: "文件来源名称无效"})
		}
		if len(validation.IsConfigMapKey(fileMount.Key)) > 0 {
			issues = append(issues, ValidationIssue{Field: field + ".key", Message: "文件来源键名无效"})
		} else if fileMount.SourceType == FileMountSourceApplicationConfig {
			if _, exists := applicationConfig[fileMount.Key]; !exists {
				issues = append(issues, ValidationIssue{Field: field + ".key", Message: "当前应用 ConfigMap 不包含该键"})
			}
		} else if fileMount.SourceType == FileMountSourceApplicationSecret {
			if _, exists := applicationSecrets[fileMount.Key]; !exists {
				issues = append(issues, ValidationIssue{Field: field + ".key", Message: "当前应用 Secret 不包含该键"})
			}
		}
		mountPath := strings.TrimSpace(fileMount.MountPath)
		if !path.IsAbs(mountPath) || path.Clean(mountPath) != mountPath || path.Base(mountPath) == "." || path.Base(mountPath) == "/" {
			issues = append(issues, ValidationIssue{Field: field + ".mount_path", Message: "文件挂载路径必须是规范的绝对文件路径"})
			continue
		}
		if _, exists := paths[mountPath]; exists {
			issues = append(issues, ValidationIssue{Field: field + ".mount_path", Message: "同一文件挂载路径不能重复使用"})
		}
		paths[mountPath] = struct{}{}
		directory := path.Dir(mountPath)
		source := fileMount.SourceType + ":" + fileMount.SourceName
		if existing, exists := directories[directory]; exists && existing != source {
			issues = append(issues, ValidationIssue{Field: field + ".mount_path", Message: "同一目录只能投影同一个 ConfigMap 或 Secret"})
		}
		directories[directory] = source
	}
	return issues
}

func validateContainerArguments(field string, values []string) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	if len(values) > 64 {
		return append(issues, ValidationIssue{Field: field, Message: "启动命令和参数最多各 64 项"})
	}
	for index, value := range values {
		if value == "" {
			issues = append(issues, ValidationIssue{Field: fmt.Sprintf("%s[%d]", field, index), Message: "启动命令和参数不能包含空项"})
			continue
		}
		if len(value) > 4096 || strings.ContainsAny(value, "\x00\r\n") {
			issues = append(issues, ValidationIssue{Field: fmt.Sprintf("%s[%d]", field, index), Message: "启动命令和参数格式无效"})
		}
	}
	return issues
}

func ValidateApplicationName(name string) error {
	if validationErrors := validation.IsDNS1123Label(name); len(validationErrors) > 0 {
		return fmt.Errorf("应用名称必须为 1-63 位小写字母、数字或连字符，且以字母或数字开头和结尾")
	}
	return nil
}
